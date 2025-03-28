package gem

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MigrationTool represents the type of migration tool to be used for database schema migrations.
type MigrationTool int

const (
	// RawSQL generates plain SQL migration files without any specific migration tool format.
	RawSQL MigrationTool = iota
	// Goose generates migration files in the format compatible with the Goose migration tool.
	// See: https://github.com/pressly/goose
	Goose
	// GolangMigrate generates migration files in the format compatible with the Golang-Migrate tool.
	// See: https://github.com/golang-migrate/migrate
	GolangMigrate
)

// Config defines the configuration options for the database schema migration generator.
type Config struct {
	// Tool specifies which migration tool format to use for generating migration files.
	// Available options are:
	// - RawSQL: Plain SQL files
	// - Goose: Goose-compatible format
	// - GolangMigrate: Golang-Migrate compatible format
	//
	// Default: RawSQL
	Tool MigrationTool

	// QuoteChar specifies the quote character to use for SQL identifiers.
	// For MySQL use backtick (`), for PostgreSQL use double quote (")
	//
	// Default: ` (backtick for MySQL)
	QuoteChar rune

	// OutputPath specifies the directory path where migration files will be stored.
	// The path can be either absolute or relative to the current working directory.
	//
	// Default: ./migrations
	OutputPath string

	// KeepDroppedColumn determines whether to preserve dropped columns in down migrations.
	// When set to true, dropped columns will be restored in down migrations.
	// When set to false, dropped columns will be permanently removed.
	//
	// Default: false
	KeepDroppedColumn bool

	// RawSQLAggregation determines whether to aggregate multiple models into a single migration file.
	// When set to true, all models will be aggregated into a single migration file.
	// When set to false, each model will have its own migration file.
	//
	//	- Note: This option is only applicable when using the RawSQL tool.
	//
	// Default: false
	RawSQLAggregation bool
}

func (c *Config) getExportDir() string {
	if len(c.OutputPath) == 0 {
		return "." + string(os.PathSeparator) + "migrations"
	}

	return c.OutputPath
}

type migrator struct {
	conf      *Config
	quote     *quote
	models    []interface{}
	snapshots []*modelSnapshot
}

// New creates a new migrator instance with the given configuration.
// If config is nil, default configuration values will be used.
func New(config *Config) *migrator {
	quote := newQuote(config.QuoteChar)
	return &migrator{
		conf:      config,
		quote:     quote,
		models:    make([]interface{}, 0),
		snapshots: make([]*modelSnapshot, 0),
	}
}

// AddModels adds one or more models to the migrator for schema migration generation.
// The models should be struct types that represent database tables.
// Returns the migrator instance for method chaining.
func (m *migrator) AddModels(models ...interface{}) *migrator {
	m.models = append(m.models, models...)

	// sort by model name
	sort.Slice(m.models, func(i, j int) bool {
		return getTableName(m.models[i]) < getTableName(m.models[j])
	})

	return m
}

// Generate executes the migration generation process for all added models.
// It performs the following steps:
// 1. Creates necessary directories for migration files
// 2. Loads existing snapshots if any
// 3. For each model:
//   - Generates SQL schema and indexes
//   - Compares with previous snapshot if exists
//   - Creates migration files for schema changes
//   - Updates snapshots
//
// 4. Saves updated snapshots
//
// Returns an error if any step fails during the process.
func (m *migrator) Generate() error {
	if len(m.models) == 0 {
		return nil
	}

	if err := os.MkdirAll(m.conf.getExportDir(), 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(m.snapshotsDir(), 0755); err != nil {
		return err
	}

	if err := m.loadSnapshots(); err != nil {
		return err
	}

	now := time.Now()
	timestamp, err := strconv.ParseInt(now.Format("20060102150405"), 10, 64)
	if err != nil {
		return fmt.Errorf("parse timestamp, err: %w", err)
	}

	timestamp -= int64(len(m.models))

	doNotEditSignFilename := filepath.Join(m.conf.getExportDir(), _doNotEditFolderFilename)
	_ = os.Truncate(doNotEditSignFilename, 0)
	if err := os.WriteFile(doNotEditSignFilename, []byte(_doNotEditFolderContent), 0644); err != nil {
		return fmt.Errorf("generate do not edit sign file, err: %w", err)
	}

	log.Println("...\tStart generating migration.")

	infos := make([]migrationFileInfo, 0, len(m.models))

	for _, model := range m.models {
		timestamp++

		schema, indexes, err := parseModelToSQLWithIndexes(model, m.quote)
		if err != nil {
			return fmt.Errorf("parse model, err: %w", err)
		}

		tableName := getTableName(model)
		newHash := m.generateHash(schema, indexes)
		snapshot := m.findSnapshot(tableName)

		if snapshot == nil {
			// New table
			info := m.generateMigrationFileInfo(timestamp, tableName, schema, indexes, true)
			infos = append(infos, info)
			m.snapshots = append(m.snapshots, &modelSnapshot{
				Name:    tableName,
				Hash:    newHash,
				Schema:  schema,
				Indexes: indexes,
			})
		} else if snapshot.Hash != newHash {
			// Check if there are actual changes
			upStatements, _ := m.generateAlterStatements(tableName, schema, indexes)
			if len(upStatements) > 0 {
				// Only generate migration file when there are actual changes
				info := m.generateMigrationFileInfo(timestamp, tableName, schema, indexes, false)
				infos = append(infos, info)
				snapshot.Hash = newHash
				snapshot.Schema = schema
				snapshot.Indexes = indexes
			}
		}
	}

	aggregateContent := &strings.Builder{}

	for _, info := range infos {
		if m.conf.RawSQLAggregation {
			aggregateContent.WriteByte('\n')
			aggregateContent.WriteString(info.upContent)
			aggregateContent.WriteByte('\n')
			aggregateContent.WriteByte('\n')
		} else {
			if err := os.WriteFile(filepath.Join(m.conf.getExportDir(), info.upFilename), []byte(info.wrapDoNotEditUpContent()), 0644); err != nil {
				return fmt.Errorf("write (%s), err: %w", info.upFilename, err)
			}

			if len(info.downFilename) != 0 {
				if err := os.WriteFile(filepath.Join(m.conf.getExportDir(), info.downFilename), []byte(info.wrapDoNotEditDownContent()), 0644); err != nil {
					return fmt.Errorf("write (%s), err: %w", info.downFilename, err)
				}
			}

			log.Default().Printf("OK\t%s", info.upFilename)
		}
	}

	if m.conf.RawSQLAggregation && aggregateContent.Len() != 0 {
		filename := filepath.Join(m.conf.getExportDir(), "aggregation.sql")
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open (%s), err: %w", filename, err)
		}
		defer f.Close()

		header := fmt.Sprintf("\n\n-- %s\n-- Generate by https://github.com/yanun0323/gem\n", now.Format("2006-01-02 15:04:05"))
		content := header + aggregateContent.String()

		if _, err := f.WriteString(content); err != nil {
			return fmt.Errorf("write (%s), err: %w", filename, err)
		}

		log.Default().Printf("OK\t%s", filename)
	}

	log.Println("\tGenerate migration done.")

	return m.saveSnapshots()
}

const (
	_snapshotName = "snapshots.json"
)

type modelSnapshot struct {
	Name    string   `json:"name"`
	Hash    string   `json:"hash"`
	Schema  string   `json:"schema"`
	Indexes []string `json:"indexes"`
}

type columnDef struct {
	Name        string
	Type        string
	Constraints []string
	Position    int
}

type tableDef struct {
	Name    string
	Columns []columnDef
	Indexes []string
}

// alterOperation defines a change operation
type alterOperation struct {
	Up   string
	Down string
}

type indexDef struct {
	Name      string
	Columns   []string
	IsUnique  bool
	TableName string
}

func (idx *indexDef) ToSQL() string {
	// Ensure no duplicate columns
	idx.Columns = removeDuplicates(idx.Columns)

	if idx.IsUnique {
		return fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s);",
			idx.Name, idx.TableName, strings.Join(idx.Columns, ", "))
	}

	return fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
		idx.Name, idx.TableName, strings.Join(idx.Columns, ", "))
}

func (m *migrator) snapshotsDir() string {
	return filepath.Join(m.conf.getExportDir(), ".gem")
}

func (m *migrator) loadSnapshots() error {
	snapshotFile := filepath.Join(m.snapshotsDir(), _snapshotName)
	data, err := os.ReadFile(snapshotFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read snapshots, err: %w", err)
	}

	return json.Unmarshal(data, &m.snapshots)
}

func (m *migrator) saveSnapshots() error {
	snapshotFile := filepath.Join(m.snapshotsDir(), _snapshotName)
	data, err := json.MarshalIndent(m.snapshots, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshots, err: %w", err)
	}

	return os.WriteFile(snapshotFile, data, 0644)
}

func (m *migrator) findSnapshot(name string) *modelSnapshot {
	for _, s := range m.snapshots {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (m *migrator) generateHash(schema string, indexes []string) string {
	h := md5.New()
	h.Write([]byte(normalizeWhitespace(schema)))
	for _, idx := range indexes {
		h.Write([]byte(normalizeWhitespace(idx)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

const (
	_textDoNotEdit   = "-- DO NOT EDIT THIS FILE!!!"
	_textGeneratedBy = "-- Generate by https://github.com/yanun0323/gem"
)

func wrapDoNotEdit(s string) string {
	return _textDoNotEdit + "\n--\n" + _textGeneratedBy + "\n\n" + s + "\n\n" + _textDoNotEdit
}

type migrationFileInfo struct {
	upFilename   string
	downFilename string
	upContent    string
	downContent  string
}

func (m *migrationFileInfo) wrapDoNotEditUpContent() string {
	return wrapDoNotEdit(m.upContent)
}

func (m *migrationFileInfo) wrapDoNotEditDownContent() string {
	return wrapDoNotEdit(m.downContent)
}

func (m *migrator) generateMigrationFileInfo(timestamp int64, tableName string, schema string, indexes []string, isNew bool) migrationFileInfo {
	var (
		upFilename string
		upContent  string

		downFilename string
		downContent  string
	)

	if isNew {
		// Case of new table
		switch m.conf.Tool {
		case RawSQL:
			upFilename = fmt.Sprintf("%d_create_%s.sql", timestamp, tableName)
			if len(indexes) == 0 {
				upContent = schema
			} else {
				upContent = schema + "\n" + joinStrings(indexes, "\n")
			}
		case Goose:
			upFilename = fmt.Sprintf("%d_create_%s.sql", timestamp, tableName)
			if len(indexes) == 0 {
				upContent = fmt.Sprintf("-- +goose Up\n%s\n\n-- +goose Down\nDROP TABLE IF EXISTS %s;\n",
					schema, m.quote.Wrap(tableName))
			} else {
				upContent = fmt.Sprintf("-- +goose Up\n%s\n\n%s\n\n-- +goose Down\nDROP TABLE IF EXISTS %s;\n",
					schema, joinStrings(indexes, "\n"), m.quote.Wrap(tableName))
			}
		case GolangMigrate:
			upFilename = fmt.Sprintf("%d_create_%s.up.sql", timestamp, tableName)
			if len(indexes) == 0 {
				upContent = schema
			} else {
				upContent = schema + "\n\n" + joinStrings(indexes, "\n")
			}

			downFilename = fmt.Sprintf("%d_create_%s.down.sql", timestamp, tableName)
			downContent = fmt.Sprintf("DROP TABLE IF EXISTS %s;", m.quote.Wrap(tableName))
		}
	} else {
		// Case of table modification
		upStatements, downStatements := m.generateAlterStatements(tableName, schema, indexes)
		switch m.conf.Tool {
		case RawSQL:
			upFilename = fmt.Sprintf("%d_alter_%s.sql", timestamp, tableName)
			upContent = joinStrings(upStatements, "\n")
		case Goose:
			upFilename = fmt.Sprintf("%d_alter_%s.sql", timestamp, tableName)
			upContent = fmt.Sprintf("-- +goose Up\n%s\n\n-- +goose Down\n%s\n",
				joinStrings(upStatements, "\n"),
				joinStrings(downStatements, "\n"))
		case GolangMigrate:
			upFilename = fmt.Sprintf("%d_alter_%s.up.sql", timestamp, tableName)
			upContent = joinStrings(upStatements, "\n")

			downFilename = fmt.Sprintf("%d_alter_%s.down.sql", timestamp, tableName)
			downContent = joinStrings(downStatements, "\n")
		}
	}

	info := migrationFileInfo{
		upFilename:   upFilename,
		upContent:    upContent,
		downFilename: downFilename,
		downContent:  downContent,
	}

	return info
}

func (m *migrator) generateAlterStatements(tableName string, newSchema string, newIndexes []string) (upStatements []string, downStatements []string) {
	// Parse new schema
	newDef, err := parseCreateTable(newSchema, m.quote)
	if err != nil {
		return []string{fmt.Sprintf("-- Error parsing new schema: %v", err)}, nil
	}

	// Get old schema from snapshot
	snapshot := m.findSnapshot(tableName)
	if snapshot == nil {
		return []string{fmt.Sprintf("-- Unable to find snapshot for table %s", tableName)}, nil
	}

	oldDef, err := parseCreateTable(snapshot.Schema, m.quote)
	if err != nil {
		return []string{fmt.Sprintf("-- Error parsing old schema: %v", err)}, nil
	}

	// Compare column differences
	columnOps := m.compareColumns(tableName, oldDef.Columns, newDef.Columns)
	for _, op := range columnOps {
		if op.Up != "" {
			upStatements = append(upStatements, op.Up)
		}
		if op.Down != "" {
			downStatements = append(downStatements, op.Down)
		}
	}

	// Compare index differences
	indexOps := compareIndexes(snapshot.Indexes, newIndexes)
	for _, op := range indexOps {
		if op.Up != "" {
			upStatements = append(upStatements, op.Up)
		}
		if op.Down != "" {
			downStatements = append(downStatements, op.Down)
		}
	}

	return upStatements, downStatements
}

func normalizeWhitespace(s string) string {
	ss := s
	ss = strings.ReplaceAll(strings.ReplaceAll(ss, "\t", " "), "\n", " ")

	count := len(ss)
	for {
		ss = strings.ReplaceAll(ss, "  ", " ")
		if len(ss) == count {
			break
		}
		count = len(ss)
	}

	return ss
}

func joinStrings(str []string, sep string) string {
	if len(str) == 0 {
		return ""
	}
	result := str[0]
	for _, s := range str[1:] {
		result += sep + s
	}
	return result
}

// parseCreateTable parses CREATE TABLE statement
func parseCreateTable(sql string, quote *quote) (*tableDef, error) {
	// Remove extra whitespace and newlines
	sql = strings.TrimSpace(sql)

	// Parse table name
	tableNameRegex := regexp.MustCompile(`CREATE TABLE IF NOT EXISTS ` + quote.RegexPattern() + ` \(([\s\S]+)\);`)
	matches := tableNameRegex.FindStringSubmatch(sql)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid CREATE TABLE syntax")
	}

	tableName := matches[1]
	columnsStr := matches[2]

	// Split column definitions
	var columns []columnDef
	var currentColumn string
	var inParentheses int
	position := 0 // 增加位置計數器

	// Split by lines and process each line
	lines := strings.Split(columnsStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Calculate brackets
		for _, char := range line {
			if char == '(' {
				inParentheses++
			} else if char == ')' {
				inParentheses--
			}
		}

		// If current line is not a complete definition, continue accumulating
		if currentColumn != "" {
			currentColumn += " " + line
		} else {
			currentColumn = line
		}

		// If brackets are paired and current line ends with comma or is the last line
		if inParentheses == 0 && (strings.HasSuffix(line, ",") || !strings.Contains(columnsStr[len(currentColumn):], ",")) {
			// Remove trailing comma
			currentColumn = strings.TrimSuffix(currentColumn, ",")

			// Parse column definition
			parts := strings.Fields(currentColumn)
			if len(parts) < 2 {
				continue
			}

			// Remove backticks from column name
			columnName := quote.Unwrap(parts[0])

			// Special handling for PRIMARY KEY definition
			if strings.ToUpper(parts[0]) == "PRIMARY" && strings.ToUpper(parts[1]) == "KEY" {
				currentColumn = ""
				continue
			}

			col := columnDef{
				Name:        columnName,
				Type:        parts[1],
				Constraints: parts[2:],
				Position:    position, // 設置欄位位置
			}
			columns = append(columns, col)
			position++ // 增加位置計數
			currentColumn = ""
		}
	}

	return &tableDef{
		Name:    tableName,
		Columns: columns,
	}, nil
}

// parseIndexes parses index definitions
func parseIndexes(indexes []string) map[string]*indexDef {
	result := make(map[string]*indexDef)
	for _, idx := range indexes {
		// Parse CREATE INDEX statement
		parts := strings.Fields(idx)
		if len(parts) < 6 { // CREATE [UNIQUE] INDEX name ON table (columns)
			continue
		}

		isUnique := strings.ToUpper(parts[1]) == "UNIQUE"
		startIdx := 2
		if isUnique {
			startIdx = 3
		}

		name := parts[startIdx]
		tableName := parts[startIdx+2]

		// Extract column names, handle content inside brackets
		columnsStr := idx[strings.Index(idx, "(")+1 : strings.LastIndex(idx, ")")]
		columns := strings.Split(columnsStr, ",")
		for i := range columns {
			columns[i] = strings.TrimSpace(columns[i])
		}

		// Remove possible extra semicolons
		for i := range columns {
			columns[i] = strings.TrimSuffix(columns[i], ";")
			columns[i] = strings.TrimSuffix(columns[i], ");")
		}

		// If index name starts with "idx_" and is duplicate, it's part of a composite index
		if existingIdx, ok := result[name]; ok {
			// Merge columns into existing index
			existingIdx.Columns = append(existingIdx.Columns, columns...)
		} else {
			result[name] = &indexDef{
				Name:      name,
				Columns:   columns,
				IsUnique:  isUnique,
				TableName: tableName,
			}
		}
	}

	// Clean up duplicate columns
	for _, idx := range result {
		idx.Columns = removeDuplicates(idx.Columns)
	}

	return result
}

// removeDuplicates removes duplicate column names
func removeDuplicates(elements []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)

	for _, element := range elements {
		if !seen[element] {
			seen[element] = true
			result = append(result, element)
		}
	}
	return result
}

// compareColumns compares differences between two column definitions
func (m *migrator) compareColumns(tableName string, oldCols, newCols []columnDef) []alterOperation {
	var operations []alterOperation
	oldColMap := make(map[string]columnDef)
	newColMap := make(map[string]columnDef)

	// Build column mapping
	for _, col := range oldCols {
		oldColMap[col.Name] = col
	}
	for _, col := range newCols {
		newColMap[col.Name] = col
	}

	// Sort new columns by position
	sortedNewCols := make([]columnDef, len(newCols))
	copy(sortedNewCols, newCols)
	sort.Slice(sortedNewCols, func(i, j int) bool {
		return sortedNewCols[i].Position < sortedNewCols[j].Position
	})

	// Build column name to position mapping
	colPositionMap := make(map[string]int)
	for i, col := range sortedNewCols {
		colPositionMap[col.Name] = i
	}

	// 跟蹤當前操作中已添加的新欄位
	addedCols := make(map[string]bool)

	// 首先收集所有要增加的欄位及其位置信息
	addedColumns := []struct {
		col      columnDef
		position int
	}{}

	for name, newCol := range newColMap {
		if _, exists := oldColMap[name]; !exists {
			position := colPositionMap[name]
			addedColumns = append(addedColumns, struct {
				col      columnDef
				position int
			}{
				col:      newCol,
				position: position,
			})
		}
	}

	// 按位置排序要添加的欄位
	sort.Slice(addedColumns, func(i, j int) bool {
		return addedColumns[i].position < addedColumns[j].position
	})

	// 處理添加欄位
	for _, item := range addedColumns {
		newCol := item.col
		position := item.position

		// 尋找前一個欄位 - 可能是原有欄位或剛剛添加的欄位
		positionClause := "FIRST"
		for i := position - 1; i >= 0; i-- {
			if i < len(sortedNewCols) {
				prevCol := sortedNewCols[i]
				// 檢查前一個欄位是原有的或已在此操作中添加的
				if _, prevExistsInOld := oldColMap[prevCol.Name]; prevExistsInOld || addedCols[prevCol.Name] {
					positionClause = fmt.Sprintf("AFTER %s", m.quote.Wrap(prevCol.Name))
					break
				}
			}
		}

		operations = append(operations, alterOperation{
			Up: fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s %s %s;",
				m.quote.Wrap(tableName), m.quote.Wrap(newCol.Name), newCol.Type, strings.Join(newCol.Constraints, " "), positionClause),
			Down: fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;",
				m.quote.Wrap(tableName), m.quote.Wrap(newCol.Name)),
		})

		// 記錄此欄位已添加
		addedCols[newCol.Name] = true
	}

	// 處理修改欄位
	for name, newCol := range newColMap {
		oldCol, exists := oldColMap[name]
		if exists && !compareColumnDef(oldCol, newCol) {
			operations = append(operations, alterOperation{
				Up: fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s %s;",
					m.quote.Wrap(tableName), m.quote.Wrap(newCol.Name), newCol.Type, strings.Join(newCol.Constraints, " ")),
				Down: fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s %s;",
					m.quote.Wrap(tableName), m.quote.Wrap(oldCol.Name), oldCol.Type, strings.Join(oldCol.Constraints, " ")),
			})
		}
	}

	// 處理刪除欄位
	if !m.conf.KeepDroppedColumn {
		sortedOldCols := make([]columnDef, len(oldCols))
		copy(sortedOldCols, oldCols)
		sort.Slice(sortedOldCols, func(i, j int) bool {
			return sortedOldCols[i].Position < sortedOldCols[j].Position
		})

		oldColPositionMap := make(map[string]int)
		for i, col := range sortedOldCols {
			oldColPositionMap[col.Name] = i
		}

		// 跟蹤當前操作中已添加的新欄位(用於DOWN操作)
		addedColsInDown := make(map[string]bool)

		// 首先收集所有要恢復的欄位及其位置信息
		deletedColumns := []struct {
			col      columnDef
			position int
		}{}

		for name, oldCol := range oldColMap {
			if _, exists := newColMap[name]; !exists {
				position := oldColPositionMap[name]
				deletedColumns = append(deletedColumns, struct {
					col      columnDef
					position int
				}{
					col:      oldCol,
					position: position,
				})
			}
		}

		// 按位置排序要恢復的欄位
		sort.Slice(deletedColumns, func(i, j int) bool {
			return deletedColumns[i].position < deletedColumns[j].position
		})

		// 為每個刪除的欄位生成操作
		for _, item := range deletedColumns {
			oldCol := item.col
			position := item.position

			// 生成 DOWN 語句，根據位置添加 AFTER 子句
			positionClause := "FIRST"
			for i := position - 1; i >= 0; i-- {
				prevColName := sortedOldCols[i].Name
				// 檢查前一個欄位是否存在於新表或已在此 DOWN 操作中添加
				if _, prevExistsInNew := newColMap[prevColName]; prevExistsInNew || addedColsInDown[prevColName] {
					positionClause = fmt.Sprintf("AFTER %s", m.quote.Wrap(prevColName))
					break
				}
			}

			operations = append(operations, alterOperation{
				Up: fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;",
					m.quote.Wrap(tableName), m.quote.Wrap(oldCol.Name)),
				Down: fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s %s %s;",
					m.quote.Wrap(tableName), m.quote.Wrap(oldCol.Name), oldCol.Type, strings.Join(oldCol.Constraints, " "), positionClause),
			})

			// 記錄此欄位已在 DOWN 操作中添加
			addedColsInDown[oldCol.Name] = true
		}
	}

	return operations
}

// compareColumnDef compares if two column definitions are the same
func compareColumnDef(old, new columnDef) bool {
	if old.Type != new.Type {
		return false
	}
	if len(old.Constraints) != len(new.Constraints) {
		return false
	}
	for i, constraint := range old.Constraints {
		if constraint != new.Constraints[i] {
			return false
		}
	}
	return true
}

// compareIndexes compares index differences
func compareIndexes(oldIndexes, newIndexes []string) []alterOperation {
	var operations []alterOperation
	oldIndexMap := parseIndexes(oldIndexes)
	newIndexMap := parseIndexes(newIndexes)

	// Check added and modified indexes
	for name, newIdx := range newIndexMap {
		oldIdx, exists := oldIndexMap[name]
		if !exists {
			// New indexes
			operations = append(operations, alterOperation{
				Up:   newIdx.ToSQL(),
				Down: fmt.Sprintf("DROP INDEX %s ON %s;", name, newIdx.TableName),
			})
		} else {
			// Compare if index definition has changes
			if !compareIndexDef(oldIdx, newIdx) {
				operations = append(operations, alterOperation{
					Up: fmt.Sprintf("DROP INDEX %s ON %s;\n%s",
						name, newIdx.TableName, newIdx.ToSQL()),
					Down: fmt.Sprintf("DROP INDEX %s ON %s;\n%s",
						name, oldIdx.TableName, oldIdx.ToSQL()),
				})
			}
		}
	}

	// Check deleted indexes
	for name, oldIdx := range oldIndexMap {
		if _, exists := newIndexMap[name]; !exists {
			operations = append(operations, alterOperation{
				Up:   fmt.Sprintf("DROP INDEX %s ON %s;", name, oldIdx.TableName),
				Down: oldIdx.ToSQL(),
			})
		}
	}

	return operations
}

// compareIndexDef compares if two index definitions are the same
func compareIndexDef(old, new *indexDef) bool {
	if old.IsUnique != new.IsUnique {
		return false
	}
	if len(old.Columns) != len(new.Columns) {
		return false
	}

	// Sort columns before comparison to ensure order doesn't affect comparison
	oldCols := make([]string, len(old.Columns))
	newCols := make([]string, len(new.Columns))
	copy(oldCols, old.Columns)
	copy(newCols, new.Columns)

	sort.Strings(oldCols)
	sort.Strings(newCols)

	for i, col := range oldCols {
		if col != newCols[i] {
			return false
		}
	}
	return true
}
