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
	"strings"
	"time"
)

type MigrationTool int

const (
	RawSQL MigrationTool = iota + 1
	Goose
	GoMigrate
)

type modelSnapshot struct {
	Name    string   `json:"name"`
	Hash    string   `json:"hash"`
	Schema  string   `json:"schema"`
	Indexes []string `json:"indexes"`
}

type migrator struct {
	conf      *MigratorConfig
	models    []interface{}
	snapshots []*modelSnapshot
}

type MigratorConfig struct {
	Format            MigrationTool
	ExportDir         string
	KeepDroppedColumn bool
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

// alterOperation 定義一個變更操作
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
	// 確保沒有重複的欄位
	idx.Columns = removeDuplicates(idx.Columns)

	if idx.IsUnique {
		return fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s);",
			idx.Name, idx.TableName, strings.Join(idx.Columns, ", "))
	}
	return fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
		idx.Name, idx.TableName, strings.Join(idx.Columns, ", "))
}

func New(config *MigratorConfig) *migrator {
	return &migrator{
		conf:      config,
		models:    make([]interface{}, 0),
		snapshots: make([]*modelSnapshot, 0),
	}
}

func (m *migrator) Model(models ...interface{}) *migrator {
	m.models = append(m.models, models...)
	return m
}

func (m *migrator) snapshotsDir() string {
	return filepath.Join(m.conf.ExportDir, ".gem")
}

func (m *migrator) loadSnapshots() error {
	snapshotFile := filepath.Join(m.snapshotsDir(), "snapshots.json")
	data, err := os.ReadFile(snapshotFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &m.snapshots)
}

func (m *migrator) saveSnapshots() error {
	snapshotFile := filepath.Join(m.snapshotsDir(), "snapshots.json")
	data, err := json.MarshalIndent(m.snapshots, "", "  ")
	if err != nil {
		return err
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

func (m *migrator) generateMigrationFile(modelName string, schema string, indexes []string, isNew bool) error {
	timestamp := time.Now().Format("20060102150405")
	var filename string
	var content string

	if isNew {
		// 新表的情況
		switch m.conf.Format {
		case RawSQL:
			filename = fmt.Sprintf("%s_create_%s.sql", timestamp, modelName)
			content = schema + "\n" + joinStrings(indexes, "\n")
		case Goose:
			filename = fmt.Sprintf("%s_create_%s.sql", timestamp, modelName)
			content = fmt.Sprintf("-- +goose Up\n%s\n%s\n\n-- +goose Down\nDROP TABLE IF EXISTS %s;\n",
				schema, joinStrings(indexes, "\n"), modelName)
		case GoMigrate:
			filename = fmt.Sprintf("%s_create_%s.up.sql", timestamp, modelName)
			content = schema + "\n" + joinStrings(indexes, "\n")

			downContent := fmt.Sprintf("DROP TABLE IF EXISTS %s;", modelName)
			downFile := filepath.Join(m.conf.ExportDir, fmt.Sprintf("%s_create_%s.down.sql", timestamp, modelName))
			if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
				return err
			}
		}
	} else {
		// 修改表的情況
		upStatements, downStatements := m.generateAlterStatements(modelName, schema, indexes)
		switch m.conf.Format {
		case RawSQL:
			filename = fmt.Sprintf("%s_alter_%s.sql", timestamp, modelName)
			content = joinStrings(upStatements, "\n")
		case Goose:
			filename = fmt.Sprintf("%s_alter_%s.sql", timestamp, modelName)
			content = fmt.Sprintf("-- +goose Up\n%s\n\n-- +goose Down\n%s\n",
				joinStrings(upStatements, "\n"),
				joinStrings(downStatements, "\n"))
		case GoMigrate:
			filename = fmt.Sprintf("%s_alter_%s.up.sql", timestamp, modelName)
			content = joinStrings(upStatements, "\n")

			downFile := filepath.Join(m.conf.ExportDir, fmt.Sprintf("%s_alter_%s.down.sql", timestamp, modelName))
			if err := os.WriteFile(downFile, []byte(joinStrings(downStatements, "\n")), 0644); err != nil {
				return err
			}
		}
	}

	return os.WriteFile(filepath.Join(m.conf.ExportDir, filename), []byte(content), 0644)
}

func (m *migrator) generateAlterStatements(tableName string, newSchema string, newIndexes []string) (upStatements []string, downStatements []string) {
	// 解析新的 schema
	newDef, err := parseCreateTable(newSchema)
	if err != nil {
		return []string{fmt.Sprintf("-- Error parsing new schema: %v", err)}, nil
	}

	// 從快照中獲取舊的 schema
	snapshot := m.findSnapshot(tableName)
	if snapshot == nil {
		return []string{fmt.Sprintf("-- 無法找到表 %s 的快照", tableName)}, nil
	}

	oldDef, err := parseCreateTable(snapshot.Schema)
	if err != nil {
		return []string{fmt.Sprintf("-- Error parsing old schema: %v", err)}, nil
	}

	// 比較欄位差異
	columnOps := m.compareColumns(oldDef.Columns, newDef.Columns)
	for _, op := range columnOps {
		if op.Up != "" {
			upStatements = append(upStatements, op.Up)
		}
		if op.Down != "" {
			downStatements = append(downStatements, op.Down)
		}
	}

	// 比較索引差異
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

func (m *migrator) Run() error {
	if err := os.MkdirAll(m.conf.ExportDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(m.snapshotsDir(), 0755); err != nil {
		return err
	}

	if err := m.loadSnapshots(); err != nil {
		return err
	}

	for _, model := range m.models {
		schema, indexes := parseModelToSQLWithIndexes(model)

		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		modelName := toSnakeCase(t.Name())

		// 如果是可命名的接口，使用指定的表名
		if nameable, ok := model.(nameable); ok {
			modelName = nameable.TableName()
		}

		newHash := m.generateHash(schema, indexes)
		snapshot := m.findSnapshot(modelName)

		if snapshot == nil {
			// 新表
			if err := m.generateMigrationFile(modelName, schema, indexes, true); err != nil {
				return err
			}
			m.snapshots = append(m.snapshots, &modelSnapshot{
				Name:    modelName,
				Hash:    newHash,
				Schema:  schema,
				Indexes: indexes,
			})
		} else if snapshot.Hash != newHash {
			// 檢查是否有實際變動
			upStatements, _ := m.generateAlterStatements(modelName, schema, indexes)
			if len(upStatements) > 0 {
				// 只有在有實際變動時才生成遷移文件
				if err := m.generateMigrationFile(modelName, schema, indexes, false); err != nil {
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

// parseCreateTable 解析 CREATE TABLE 語句
func parseCreateTable(sql string) (*tableDef, error) {
	// 移除多餘的空白和換行
	sql = strings.TrimSpace(sql)

	// 解析表名
	tableNameRegex := regexp.MustCompile(`CREATE TABLE ` + "`" + `(\w+)` + "`" + ` \(([\s\S]+)\);`)
	matches := tableNameRegex.FindStringSubmatch(sql)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid CREATE TABLE syntax")
	}

	tableName := matches[1]
	columnsStr := matches[2]

	// 分割欄位定義
	var columns []columnDef
	var currentColumn string
	var inParentheses int

	// 按行分割並處理每一行
	lines := strings.Split(columnsStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 計算括號
		for _, char := range line {
			if char == '(' {
				inParentheses++
			} else if char == ')' {
				inParentheses--
			}
		}

		// 如果當前行不是完整的定義，則繼續累積
		if currentColumn != "" {
			currentColumn += " " + line
		} else {
			currentColumn = line
		}

		// 如果括號已經配對完成，且當前行以逗號結尾或是最後一行
		if inParentheses == 0 && (strings.HasSuffix(line, ",") || !strings.Contains(columnsStr[len(currentColumn):], ",")) {
			// 移除尾部的逗號
			currentColumn = strings.TrimSuffix(currentColumn, ",")

			// 解析欄位定義
			parts := strings.Fields(currentColumn)
			if len(parts) < 2 {
				continue
			}

			// 移除欄位名稱的反引號
			columnName := strings.Trim(parts[0], "`")

			// 特殊處理 PRIMARY KEY 定義
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

// parseIndexes 解析索引定義
func parseIndexes(indexes []string) map[string]*indexDef {
	result := make(map[string]*indexDef)
	for _, idx := range indexes {
		// 解析 CREATE INDEX 語句
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

		// 提取列名，處理括號內的內容
		columnsStr := idx[strings.Index(idx, "(")+1 : strings.LastIndex(idx, ")")]
		columns := strings.Split(columnsStr, ",")
		for i := range columns {
			columns[i] = strings.TrimSpace(columns[i])
		}

		// 移除可能存在的多餘分號
		for i := range columns {
			columns[i] = strings.TrimSuffix(columns[i], ";")
			columns[i] = strings.TrimSuffix(columns[i], ");")
		}

		// 如果索引名稱以 "idx_" 開頭且重複，則為複合索引的一部分
		if existingIdx, ok := result[name]; ok {
			// 合併欄位到現有索引
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

	// 清理重複的欄位
	for _, idx := range result {
		idx.Columns = removeDuplicates(idx.Columns)
	}

	return result
}

// removeDuplicates 移除重複的欄位名稱
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

// compareColumns 比較兩個欄位定義的差異
func (m *migrator) compareColumns(oldCols, newCols []columnDef) []alterOperation {
	var operations []alterOperation
	oldColMap := make(map[string]columnDef)
	newColMap := make(map[string]columnDef)

	// 建立欄位映射
	for _, col := range oldCols {
		oldColMap[col.Name] = col
	}
	for _, col := range newCols {
		newColMap[col.Name] = col
	}

	// 檢查新增和修改的欄位
	for name, newCol := range newColMap {
		oldCol, exists := oldColMap[name]
		if !exists {
			// 新增欄位
			operations = append(operations, alterOperation{
				Up: fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s %s;",
					name, newCol.Name, newCol.Type, strings.Join(newCol.Constraints, " ")),
				Down: fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `%s`;",
					name, newCol.Name),
			})
		} else {
			// 比較欄位定義是否有變更
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

	// 檢查刪除的欄位
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

// compareColumnDef 比較兩個欄位的定義是否相同
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

// compareIndexes 比較索引的差異
func compareIndexes(oldIndexes, newIndexes []string) []alterOperation {
	var operations []alterOperation
	oldIndexMap := parseIndexes(oldIndexes)
	newIndexMap := parseIndexes(newIndexes)

	// 檢查新增和修改的索引
	for name, newIdx := range newIndexMap {
		oldIdx, exists := oldIndexMap[name]
		if !exists {
			// 新增索引
			operations = append(operations, alterOperation{
				Up:   newIdx.ToSQL(),
				Down: fmt.Sprintf("DROP INDEX %s;", name),
			})
		} else {
			// 比較索引定義是否有變更
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

	// 檢查刪除的索引
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

// compareIndexDef 比較兩個索引的定義是否相同
func compareIndexDef(old, new *indexDef) bool {
	if old.IsUnique != new.IsUnique {
		return false
	}
	if len(old.Columns) != len(new.Columns) {
		return false
	}

	// 將欄位排序後比較，確保順序不影響比較結果
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

// normalizeIndex 正規化索引定義
func normalizeIndex(idx string) string {
	// 移除多餘的空白和換行
	return strings.Join(strings.Fields(idx), " ")
}

// extractIndexName 從索引定義中提取索引名稱
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
