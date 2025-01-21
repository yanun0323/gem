package gem

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"
)

type nameable interface {
	TableName() string
}

type indexInfo struct {
	Name     string
	Columns  []string
	IsUnique bool
}

// parseModel 解析 GORM model 結構體
func parseModel(model interface{}) (tableName string, columns []string, indexes map[string]*indexInfo) {
	// 獲取結構體的反射類型
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	tableName = toSnakeCase(t.Name())
	if nameable, ok := model.(nameable); ok {
		tableName = nameable.TableName()
	}

	columns = make([]string, 0)
	indexes = make(map[string]*indexInfo)

	// 遍歷所有字段
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 忽略未導出的字段
		if !field.IsExported() {
			continue
		}

		// 處理嵌入字段
		if field.Anonymous || hasTag(field, "embedded") {
			embeddedPrefix := getTagValue(field, "embeddedPrefix")
			columns = append(columns, parseEmbeddedField(field.Type, embeddedPrefix)...)
			continue
		}

		column := parseField(field)
		if column != "" {
			columns = append(columns, column)
		}

		// 處理索引
		if hasTag(field, "index") {
			indexName := getTagValue(field, "index")
			if indexName == "" {
				// 如果只有 index 標記但沒有值，創建單欄位索引
				indexName = fmt.Sprintf("idx_%s", toSnakeCase(field.Name))
				indexes[indexName] = &indexInfo{
					Name:     indexName,
					Columns:  []string{toSnakeCase(field.Name)},
					IsUnique: false,
				}
			} else {
				// 如果有指定索引名稱，可能是複合索引的一部分
				if idx, exists := indexes[indexName]; exists {
					idx.Columns = append(idx.Columns, toSnakeCase(field.Name))
				} else {
					indexes[indexName] = &indexInfo{
						Name:     indexName,
						Columns:  []string{toSnakeCase(field.Name)},
						IsUnique: false,
					}
				}
			}
		}

		// 處理唯一索引
		if hasTag(field, "uniqueIndex") {
			indexName := getTagValue(field, "uniqueIndex")
			if indexName == "" {
				// 如果只有 uniqueIndex 標記但沒有值，創建單欄位唯一索引
				indexName = fmt.Sprintf("udx_%s", toSnakeCase(field.Name))
				indexes[indexName] = &indexInfo{
					Name:     indexName,
					Columns:  []string{toSnakeCase(field.Name)},
					IsUnique: true,
				}
			} else {
				// 如果有指定索引名稱，可能是複合索引的一部分
				if idx, exists := indexes[indexName]; exists {
					idx.Columns = append(idx.Columns, toSnakeCase(field.Name))
				} else {
					indexes[indexName] = &indexInfo{
						Name:     indexName,
						Columns:  []string{toSnakeCase(field.Name)},
						IsUnique: true,
					}
				}
			}
		}
	}

	return
}

// parseModelToSQL 將 GORM model 結構體解析為 CREATE TABLE SQL 語句
func parseModelToSQL(model interface{}) string {
	tableName, columns, _ := parseModel(model)
	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n);",
		tableName,
		strings.Join(columns, ",\n  "))
}

// parseModelToSQLWithIndexes 解析模型並返回 CREATE TABLE 語句和索引定義
func parseModelToSQLWithIndexes(model interface{}) (string, []string) {
	tableName, columns, indexes := parseModel(model)

	// 生成 CREATE TABLE 語句
	createTable := fmt.Sprintf("CREATE TABLE %s (\n  %s\n);",
		tableName,
		strings.Join(columns, ",\n  "))

	// 生成索引語句
	var indexStatements []string
	for _, idx := range indexes {
		if idx.IsUnique {
			indexStatements = append(indexStatements,
				fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s);",
					idx.Name, tableName, strings.Join(idx.Columns, ", ")))
		} else {
			indexStatements = append(indexStatements,
				fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
					idx.Name, tableName, strings.Join(idx.Columns, ", ")))
		}
	}

	sort.Strings(indexStatements)

	return createTable, indexStatements
}

// parseField 解析單個字段
func parseField(field reflect.StructField) string {
	// 如果標記為 "-" 則忽略該字段
	if getTagValue(field, "-") == "all" {
		return ""
	}

	columnName := getColumnName(field)
	sqlType := getSQLType(field)

	var constraints []string

	// 按照固定順序添加約束
	if hasTag(field, "primaryKey") {
		constraints = append(constraints, "PRIMARY KEY")
	}
	if hasTag(field, "autoIncrement") {
		constraints = append(constraints, "AUTO_INCREMENT")
	}

	// 處理 check 約束
	if check := getTagValue(field, "check"); check != "" {
		constraints = append(constraints, fmt.Sprintf("CHECK (%s)", check))
	}

	if hasTag(field, "unique") {
		constraints = append(constraints, "UNIQUE")
	}

	// 只有非指標類型或明確標記為 not null 時才添加 NOT NULL 約束
	if hasTag(field, "not null") || (field.Type.Kind() != reflect.Ptr && !hasTag(field, "default")) {
		constraints = append(constraints, "NOT NULL")
	}

	// 處理默認值
	if defaultValue := getTagValue(field, "default"); defaultValue != "" {
		constraints = append(constraints, fmt.Sprintf("DEFAULT %s", defaultValue))
	}

	// 處理 comment，使用單引號，不需要額外轉義
	if comment := getTagValue(field, "comment"); comment != "" {
		// 移除首尾的引號（如果有的話）
		comment = strings.Trim(comment, "'")
		constraints = append(constraints, fmt.Sprintf("COMMENT '%s'", comment))
	}

	if len(constraints) > 0 {
		return fmt.Sprintf("%s %s %s", columnName, sqlType, strings.Join(constraints, " "))
	}
	return fmt.Sprintf("%s %s", columnName, sqlType)
}

// parseEmbeddedField 解析嵌入字段
func parseEmbeddedField(t reflect.Type, prefix string) []string {
	var columns []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		column := parseField(field)
		if column != "" {
			if prefix != "" {
				// 添加前綴到列名
				parts := strings.SplitN(column, " ", 2)
				column = prefix + parts[0] + " " + parts[1]
			}
			columns = append(columns, column)
		}
	}

	return columns
}

// getSQLType 根據 Go 類型獲取對應的 SQL 類型
func getSQLType(field reflect.StructField) string {
	// 檢查是否有明確指定類型
	if sqlType := getTagValue(field, "type"); sqlType != "" {
		sqlType = strings.ToUpper(sqlType)
		// 如果是指標類型且不是主鍵，添加 NULL 約束
		if field.Type.Kind() == reflect.Ptr && !hasTag(field, "primaryKey") {
			return sqlType + " NULL"
		}
		return sqlType
	}

	// 處理精度
	precision := getTagValue(field, "precision")
	scale := getTagValue(field, "scale")
	if precision != "" {
		if scale != "" {
			return fmt.Sprintf("DECIMAL(%s,%s)", precision, scale)
		}
		return fmt.Sprintf("DECIMAL(%s)", precision)
	}

	// 獲取 size 標籤
	size := getTagValue(field, "size")

	// 獲取基礎類型
	fieldType := field.Type
	isPtr := fieldType.Kind() == reflect.Ptr
	if isPtr {
		fieldType = fieldType.Elem()
	}

	var sqlType string
	switch fieldType.Kind() {
	case reflect.Bool:
		sqlType = "BOOLEAN"
	case reflect.Int, reflect.Int32:
		sqlType = "INTEGER"
	case reflect.Int8:
		sqlType = "TINYINT"
	case reflect.Int16:
		sqlType = "SMALLINT"
	case reflect.Int64:
		sqlType = "BIGINT"
	case reflect.Uint:
		if field.Name == "ID" {
			sqlType = "INTEGER"
		} else {
			sqlType = "INTEGER UNSIGNED"
		}
	case reflect.Uint8:
		sqlType = "TINYINT UNSIGNED"
	case reflect.Uint16:
		sqlType = "SMALLINT UNSIGNED"
	case reflect.Uint32:
		sqlType = "INTEGER UNSIGNED"
	case reflect.Uint64:
		sqlType = "BIGINT UNSIGNED"
	case reflect.Float32:
		sqlType = "FLOAT"
	case reflect.Float64:
		sqlType = "DOUBLE"
	case reflect.String:
		if size != "" {
			sqlType = fmt.Sprintf("VARCHAR(%s)", size)
		} else {
			sqlType = "VARCHAR(255)"
		}
	default:
		// 處理特殊類型
		typeName := fieldType.String()
		switch typeName {
		case "time.Time":
			sqlType = "DATETIME"
		case "[]byte":
			sqlType = "BLOB"
		default:
			sqlType = "VARCHAR(255)"
		}
	}

	// 如果是指標類型且不是主鍵，添加 NULL 約束
	if isPtr && !hasTag(field, "primaryKey") {
		return sqlType + " NULL"
	}

	return sqlType
}

// 工具函數

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		// 特殊處理連續的大寫字母
		if i > 0 && r >= 'A' && r <= 'Z' {
			// 檢查前一個字符是否為小寫或後一個字符是否為小寫
			prev := s[i-1]
			if prev >= 'a' && prev <= 'z' {
				result.WriteByte('_')
			} else if i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z' {
				if i > 1 {
					result.WriteByte('_')
				}
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

func getTagValue(field reflect.StructField, key string) string {
	tag := field.Tag.Get("gorm")
	for _, option := range strings.Split(tag, ";") {
		kv := strings.SplitN(option, ":", 2)
		if kv[0] == key {
			if len(kv) == 2 {
				return kv[1]
			}
			return ""
		}
	}
	return ""
}

func hasTag(field reflect.StructField, key string) bool {
	tag := field.Tag.Get("gorm")
	for _, option := range strings.Split(tag, ";") {
		if option == key || strings.HasPrefix(option, key+":") {
			return true
		}
	}
	return false
}

func getColumnName(field reflect.StructField) string {
	if columnName := getTagValue(field, "column"); columnName != "" {
		return columnName
	}
	return toSnakeCase(field.Name)
}

// 添加新的輔助函數來檢查是否有指定值的標籤
func hasTagValue(field reflect.StructField, key string) bool {
	tag := field.Tag.Get("gorm")
	for _, option := range strings.Split(tag, ";") {
		kv := strings.SplitN(option, ":", 2)
		if kv[0] == key && len(kv) == 2 && kv[1] != "" {
			return true
		}
	}
	return false
}
