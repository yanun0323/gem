package gem

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
}

func (c *Config) getExportDir() string {
	if len(c.OutputPath) == 0 {
		return "." + string(os.PathSeparator) + "migrations"
	}

	return c.OutputPath
}

type migrator struct {
	conf      *Config
	models    []interface{}
	snapshots []*modelSnapshot
}

// New creates a new migrator instance with the given configuration.
// If config is nil, default configuration values will be used.
func New(config *Config) *migrator {
	return &migrator{
		conf:      config,
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

	timestamp, err := strconv.ParseInt(time.Now().Format("20060102150405"), 10, 64)
	if err != nil {
		return fmt.Errorf("parse timestamp, err: %w", err)
	}

	timestamp -= int64(len(m.models))

	doNotEditSignFilename := filepath.Join(m.conf.getExportDir(), _doNotEditFolderFilename)
	_ = os.Truncate(doNotEditSignFilename, 0)
	if err := os.WriteFile(doNotEditSignFilename, []byte(_doNotEditFolderContent), 0644); err != nil {
		return fmt.Errorf("generate do not edit sign file, err: %w", err)
	}

	for _, model := range m.models {
		timestamp++

		schema, indexes, err := parseModelToSQLWithIndexes(model)
		if err != nil {
			return fmt.Errorf("parse model, err: %w", err)
		}

		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		modelName := toSnakeCase(t.Name())

		// If it's a nameable interface, use the specified table name
		if nameable, ok := model.(nameable); ok {
			modelName = nameable.TableName()
		}

		newHash := m.generateHash(schema, indexes)
		snapshot := m.findSnapshot(modelName)

		if snapshot == nil {
			// New table
			if err := m.generateMigrationFile(timestamp, modelName, schema, indexes, true); err != nil {
				return err
			}
			m.snapshots = append(m.snapshots, &modelSnapshot{
				Name:    modelName,
				Hash:    newHash,
				Schema:  schema,
				Indexes: indexes,
			})
		} else if snapshot.Hash != newHash {
			// Check if there are actual changes
			upStatements, _ := m.generateAlterStatements(modelName, schema, indexes)
			if len(upStatements) > 0 {
				// Only generate migration file when there are actual changes
				if err := m.generateMigrationFile(timestamp, modelName, schema, indexes, false); err != nil {
					return err
				}
				snapshot.Hash = newHash
				snapshot.Schema = schema
				snapshot.Indexes = indexes
			}
		}
	}

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

func (m *migrator) generateMigrationFile(timestamp int64, modelName string, schema string, indexes []string, isNew bool) error {
	var filename string
	var content string

	if isNew {
		// Case of new table
		switch m.conf.Tool {
		case RawSQL:
			filename = fmt.Sprintf("%d_create_%s.sql", timestamp, modelName)
			if len(indexes) == 0 {
				content = schema
			} else {
				content = schema + "\n" + joinStrings(indexes, "\n")
			}
		case Goose:
			filename = fmt.Sprintf("%d_create_%s.sql", timestamp, modelName)
			if len(indexes) == 0 {
				content = fmt.Sprintf("-- +goose Up\n%s\n\n-- +goose Down\nDROP TABLE IF EXISTS `%s`;\n",
					schema, modelName)
			} else {
				content = fmt.Sprintf("-- +goose Up\n%s\n\n%s\n\n-- +goose Down\nDROP TABLE IF EXISTS `%s`;\n",
					schema, joinStrings(indexes, "\n"), modelName)
			}
		case GolangMigrate:
			filename = fmt.Sprintf("%d_create_%s.up.sql", timestamp, modelName)
			if len(indexes) == 0 {
				content = schema
			} else {
				content = schema + "\n\n" + joinStrings(indexes, "\n")
			}

			downContent := fmt.Sprintf("DROP TABLE IF EXISTS `%s`;", modelName)
			downFile := filepath.Join(m.conf.getExportDir(), fmt.Sprintf("%d_create_%s.down.sql", timestamp, modelName))
			if err := os.WriteFile(
				downFile,
				[]byte(wrapDoNotEdit(downContent)),
				0644,
			); err != nil {
				return err
			}
		}
	} else {
		// Case of table modification
		upStatements, downStatements := m.generateAlterStatements(modelName, schema, indexes)
		switch m.conf.Tool {
		case RawSQL:
			filename = fmt.Sprintf("%d_alter_%s.sql", timestamp, modelName)
			content = joinStrings(upStatements, "\n")
		case Goose:
			filename = fmt.Sprintf("%d_alter_%s.sql", timestamp, modelName)
			content = fmt.Sprintf("-- +goose Up\n%s\n\n-- +goose Down\n%s\n",
				joinStrings(upStatements, "\n"),
				joinStrings(downStatements, "\n"))
		case GolangMigrate:
			filename = fmt.Sprintf("%d_alter_%s.up.sql", timestamp, modelName)
			content = joinStrings(upStatements, "\n")

			downFile := filepath.Join(m.conf.getExportDir(), fmt.Sprintf("%d_alter_%s.down.sql", timestamp, modelName))
			if err := os.WriteFile(
				downFile,
				[]byte(wrapDoNotEdit(joinStrings(downStatements, "\n"))),
				0644,
			); err != nil {
				return err
			}
		}
	}

	content = wrapDoNotEdit(content)

	return os.WriteFile(filepath.Join(m.conf.getExportDir(), filename), []byte(content), 0644)
}

func (m *migrator) generateAlterStatements(tableName string, newSchema string, newIndexes []string) (upStatements []string, downStatements []string) {
	// Parse new schema
	newDef, err := parseCreateTable(newSchema)
	if err != nil {
		return []string{fmt.Sprintf("-- Error parsing new schema: %v", err)}, nil
	}

	// Get old schema from snapshot
	snapshot := m.findSnapshot(tableName)
	if snapshot == nil {
		return []string{fmt.Sprintf("-- Unable to find snapshot for table %s", tableName)}, nil
	}

	oldDef, err := parseCreateTable(snapshot.Schema)
	if err != nil {
		return []string{fmt.Sprintf("-- Error parsing old schema: %v", err)}, nil
	}

	// Compare column differences
	columnOps := m.compareColumns(oldDef.Columns, newDef.Columns)
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
func parseCreateTable(sql string) (*tableDef, error) {
	// Remove extra whitespace and newlines
	sql = strings.TrimSpace(sql)

	// Parse table name
	tableNameRegex := regexp.MustCompile(`CREATE TABLE ` + "`" + `(\w+)` + "`" + ` \(([\s\S]+)\);`)
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
			columnName := strings.Trim(parts[0], "`")

			// Special handling for PRIMARY KEY definition
			if strings.ToUpper(parts[0]) == "PRIMARY" && strings.ToUpper(parts[1]) == "KEY" {
				currentColumn = ""
				continue
			}

			col := columnDef{
				Name:        columnName,
				Type:        parts[1],
				Constraints: parts[2:],
			}
			columns = append(columns, col)
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
func (m *migrator) compareColumns(oldCols, newCols []columnDef) []alterOperation {
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

	// Check added and modified columns
	for name, newCol := range newColMap {
		oldCol, exists := oldColMap[name]
		if !exists {
			// New columns
			operations = append(operations, alterOperation{
				Up: fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s %s;",
					name, newCol.Name, newCol.Type, strings.Join(newCol.Constraints, " ")),
				Down: fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `%s`;",
					name, newCol.Name),
			})
		} else {
			// Compare if column definition has changes
			if !compareColumnDef(oldCol, newCol) {
				operations = append(operations, alterOperation{
					Up: fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN `%s` %s %s;",
						name, newCol.Name, newCol.Type, strings.Join(newCol.Constraints, " ")),
					Down: fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN `%s` %s %s;",
						name, oldCol.Name, oldCol.Type, strings.Join(oldCol.Constraints, " ")),
				})
			}
		}
	}

	// Check deleted columns
	if !m.conf.KeepDroppedColumn {
		for name, oldCol := range oldColMap {
			if _, exists := newColMap[name]; !exists {
				operations = append(operations, alterOperation{
					Up: fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `%s`;",
						name, oldCol.Name),
					Down: fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s %s;",
						name, oldCol.Name, oldCol.Type, strings.Join(oldCol.Constraints, " ")),
				})
			}
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
				Down: fmt.Sprintf("DROP INDEX %s;", name),
			})
		} else {
			// Compare if index definition has changes
			if !compareIndexDef(oldIdx, newIdx) {
				operations = append(operations, alterOperation{
					Up: fmt.Sprintf("DROP INDEX %s;\n%s",
						name, newIdx.ToSQL()),
					Down: fmt.Sprintf("DROP INDEX %s;\n%s",
						name, oldIdx.ToSQL()),
				})
			}
		}
	}

	// Check deleted indexes
	for name, oldIdx := range oldIndexMap {
		if _, exists := newIndexMap[name]; !exists {
			operations = append(operations, alterOperation{
				Up:   fmt.Sprintf("DROP INDEX %s;", name),
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

// normalizeIndex normalizes index definition
func normalizeIndex(idx string) string {
	// Remove extra whitespace and newlines
	return strings.Join(strings.Fields(idx), " ")
}

// extractIndexName extracts index name from index definition
func extractIndexName(idx string) string {
	parts := strings.Fields(idx)
	startIdx := 2
	if len(parts) >= 3 {
		if strings.ToUpper(parts[1]) == "UNIQUE" {
			startIdx = 3
		}
		if len(parts) > startIdx && strings.ToUpper(parts[0]) == "CREATE" {
			return parts[startIdx]
		}
	}
	return ""
}
