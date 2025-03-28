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
	Quote      *quote
	Name       string
	Columns    []string
	IsUnique   bool
	Priorities map[string]int
	TableName  string
}

func getTableName(model interface{}) string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	tableName := toSnakeCase(t.Name())
	if nameable, ok := model.(nameable); ok {
		tableName = nameable.TableName()
	} else {
		tableName = toPlural(tableName)
	}

	return tableName
}

func toPlural(s string) string {
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "z") || strings.HasSuffix(s, "ch") ||
		strings.HasSuffix(s, "sh") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") {
		if len(s) > 1 {
			lastChar := rune(s[len(s)-2])
			if lastChar != 'a' && lastChar != 'e' && lastChar != 'i' &&
				lastChar != 'o' && lastChar != 'u' {
				return s[:len(s)-1] + "ies"
			}
		}
	}
	return s + "s"
}

// parseModel parses GORM model struct
// Get the reflection type of the struct
func parseModel(model interface{}, quote *quote) (tableName string, columns []string, indexes map[string]*indexInfo) {
	// Get the reflection type of the struct
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	tableName = getTableName(model)

	// 創建一個新的結構來存儲欄位定義，包括位置資訊
	type fieldInfo struct {
		name     string
		def      string
		position int
	}

	fieldInfos := []fieldInfo{}
	indexes = make(map[string]*indexInfo)

	// 收集所有有效欄位的資訊
	validFieldCount := 0
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 忽略未導出欄位
		if !field.IsExported() {
			continue
		}

		// 處理嵌入欄位
		if field.Anonymous || hasTag(field, "embedded") {
			embeddedPrefix := getTagValue(field, "embeddedPrefix")
			// 這裡需要修改以支持嵌入欄位的位置追蹤
			embeddedColumns := parseEmbeddedField(field.Type, embeddedPrefix, quote)
			for _, col := range embeddedColumns {
				fieldInfos = append(fieldInfos, fieldInfo{
					name:     "", // 需要從col提取名稱
					def:      col,
					position: validFieldCount,
				})
				validFieldCount++
			}
			continue
		}

		column := parseField(field, quote)
		if column != "" {
			columnName := getColumnName(field)
			fieldInfos = append(fieldInfos, fieldInfo{
				name:     columnName,
				def:      column,
				position: validFieldCount,
			})
			validFieldCount++
		}

		// Handle indexes
		if hasTag(field, "index") {
			indexName := getTagValue(field, "index")
			columnName := getColumnName(field)
			priority := 0

			// 檢查是否有 priority 後綴
			if strings.Contains(indexName, ",priority:") {
				parts := strings.Split(indexName, ",priority:")
				indexName = parts[0]
				if len(parts) > 1 {
					fmt.Sscanf(parts[1], "%d", &priority)
				}
			}

			if indexName == "" {
				// If there's only index tag without value, create a single-column index
				indexName = fmt.Sprintf("idx_%s", columnName)
				indexes[indexName] = &indexInfo{
					Quote:      quote,
					Name:       indexName,
					Columns:    []string{columnName},
					IsUnique:   false,
					Priorities: map[string]int{columnName: priority},
					TableName:  tableName,
				}
			} else {
				// If there's a specified index name, it might be part of a composite index
				if idx, exists := indexes[indexName]; exists {
					idx.Columns = append(idx.Columns, columnName)
					idx.Priorities[columnName] = priority
				} else {
					indexes[indexName] = &indexInfo{
						Quote:      quote,
						Name:       indexName,
						Columns:    []string{columnName},
						IsUnique:   false,
						Priorities: map[string]int{columnName: priority},
						TableName:  tableName,
					}
				}
			}
		}

		// Handle unique indexes
		if hasTag(field, "uniqueIndex") {
			indexName := getTagValue(field, "uniqueIndex")
			columnName := getColumnName(field)
			priority := 0

			// 檢查是否有 priority 後綴
			if strings.Contains(indexName, ",priority:") {
				parts := strings.Split(indexName, ",priority:")
				indexName = parts[0]
				if len(parts) > 1 {
					fmt.Sscanf(parts[1], "%d", &priority)
				}
			}

			if indexName == "" {
				// If there's only uniqueIndex tag without value, create a single-column unique index
				indexName = fmt.Sprintf("udx_%s", columnName)
				indexes[indexName] = &indexInfo{
					Quote:      quote,
					Name:       indexName,
					Columns:    []string{columnName},
					IsUnique:   true,
					Priorities: map[string]int{columnName: priority},
					TableName:  tableName,
				}
			} else {
				// If there's a specified index name, it might be part of a composite index
				if idx, exists := indexes[indexName]; exists {
					idx.Columns = append(idx.Columns, columnName)
					idx.Priorities[columnName] = priority
				} else {
					indexes[indexName] = &indexInfo{
						Quote:      quote,
						Name:       indexName,
						Columns:    []string{columnName},
						IsUnique:   true,
						Priorities: map[string]int{columnName: priority},
						TableName:  tableName,
					}
				}
			}
		}
	}

	// 將欄位資訊按照位置排序並轉換為欄位定義列表
	sort.Slice(fieldInfos, func(i, j int) bool {
		return fieldInfos[i].position < fieldInfos[j].position
	})

	for _, info := range fieldInfos {
		columns = append(columns, info.def)
	}

	return
}

// parseModelToSQLWithIndexes parses model and returns CREATE TABLE statement and index definitions
// Check if there's a primary key field
// If there's a primary key, add PRIMARY KEY constraint
// Generate CREATE TABLE statement
// Generate index statements
func parseModelToSQLWithIndexes(model interface{}, quote *quote) (string, []string, error) {
	tableName, columns, indexes := parseModel(model, quote)

	// Check if there's a primary key field
	primaryKeyName := ""
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if hasTag(field, "primaryKey") {
			primaryKeyName = getColumnName(field)
			break
		}
	}

	if len(primaryKeyName) != 0 {
		columns = append(columns, fmt.Sprintf("PRIMARY KEY (%s)", quote.Wrap(primaryKeyName)))
	}

	// Generate CREATE TABLE statement
	createTable := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n);",
		quote.Wrap(tableName),
		strings.Join(columns, ",\n  "))

	// Generate index statements
	var indexStatements []string
	for _, idx := range indexes {
		indexStatements = append(indexStatements, idx.ToSQL())
	}

	sort.Strings(indexStatements)

	return createTable, indexStatements, nil
}

// parseField parses a single field
// If marked as "-", ignore this field
// Add constraints in fixed order
// Handle check constraint
// Add NOT NULL constraint only for non-pointer types or explicitly marked as not null
// Handle default value
// Handle comment, use single quotes, no need for extra escaping
// Remove leading and trailing quotes (if any)
func parseField(field reflect.StructField, quote *quote) string {
	// If marked as "-", ignore this field
	if ignore := getTagValue(field, "-"); ignore == "all" || ignore == "migration" {
		return ""
	}

	columnName := getColumnName(field)
	sqlType := getSQLType(field)

	var constraints []string

	// Add constraints in fixed order
	if hasTag(field, "autoIncrement") {
		constraints = append(constraints, "AUTO_INCREMENT")
	}

	// Handle check constraint
	if check := getTagValue(field, "check"); check != "" {
		constraints = append(constraints, fmt.Sprintf("CHECK (%s)", check))
	}

	if hasTag(field, "unique") {
		constraints = append(constraints, "UNIQUE")
	}

	// Add NOT NULL constraint only for non-pointer types or explicitly marked as not null
	if hasTag(field, "not null") || (field.Type.Kind() != reflect.Ptr && !hasTag(field, "default")) {
		constraints = append(constraints, "NOT NULL")
	}

	// Handle default value
	if defaultValue := getTagValue(field, "default"); defaultValue != "" {
		constraints = append(constraints, fmt.Sprintf("DEFAULT %s", defaultValue))
	}

	// Handle comment, use single quotes, no need for extra escaping
	if comment := getTagValue(field, "comment"); comment != "" {
		// Remove leading and trailing quotes (if any)
		comment = strings.Trim(comment, "'")
		constraints = append(constraints, fmt.Sprintf("COMMENT '%s'", comment))
	}

	if len(constraints) > 0 {
		return fmt.Sprintf("%s %s %s", quote.Wrap(columnName), sqlType, strings.Join(constraints, " "))
	}
	return fmt.Sprintf("%s %s", quote.Wrap(columnName), sqlType)
}

// parseEmbeddedField parses embedded fields
// Add prefix to column name and ensure correct backtick placement
// Remove original backticks
// Add prefix and re-add backticks
func parseEmbeddedField(t reflect.Type, prefix string, quote *quote) []string {
	var columns []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		column := parseField(field, quote)
		if column != "" {
			if prefix != "" {
				// Add prefix to column name and ensure correct backtick placement
				parts := strings.SplitN(column, " ", 2)
				// Remove original backticks
				columnName := quote.Unwrap(parts[0])
				// Add prefix and re-add backticks
				column = fmt.Sprintf("%s %s", quote.Wrap(prefix+columnName), parts[1])
			}
			columns = append(columns, column)
		}
	}

	return columns
}

// getSQLType gets corresponding SQL type based on Go type
// Check if type is explicitly specified
// If it's a pointer type and not primary key, add NULL constraint
// Handle precision
// Get size tag
// Get base type
// Handle special types
// If it's a pointer type and not primary key, add NULL constraint
func getSQLType(field reflect.StructField) string {
	// Check if type is explicitly specified
	if sqlType := getTagValue(field, "type"); sqlType != "" {
		sqlType = strings.ToUpper(sqlType)
		// If it's a pointer type and not primary key, add NULL constraint
		if field.Type.Kind() == reflect.Ptr && !hasTag(field, "primaryKey") {
			return sqlType + " NULL"
		}
		return sqlType
	}

	// Handle precision
	precision := getTagValue(field, "precision")
	scale := getTagValue(field, "scale")
	if precision != "" {
		if scale != "" {
			return fmt.Sprintf("DECIMAL(%s,%s)", precision, scale)
		}
		return fmt.Sprintf("DECIMAL(%s)", precision)
	}

	// Get size tag
	size := getTagValue(field, "size")

	// Get base type
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
		sqlType = "INTEGER UNSIGNED"
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
		// Handle special types
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

	// If it's a pointer type and not primary key, add NULL constraint
	if isPtr && !hasTag(field, "primaryKey") {
		return sqlType + " NULL"
	}

	return sqlType
}

// Utility functions
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		// Special handling for consecutive uppercase letters
		if i > 0 && r >= 'A' && r <= 'Z' {
			// Check if previous character is lowercase or next character is lowercase
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
		if strings.EqualFold(option, key) || strings.HasPrefix(option, key+":") {
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

func (idx *indexInfo) ToSQL() string {
	// Ensure no duplicate columns
	idx.Columns = removeDuplicates(idx.Columns)

	// Sort columns by priority if priorities are set
	if len(idx.Priorities) > 0 {
		type columnPriority struct {
			name     string
			priority int
		}

		sortedColumns := make([]columnPriority, 0, len(idx.Columns))
		for _, col := range idx.Columns {
			priority := idx.Priorities[col]
			sortedColumns = append(sortedColumns, columnPriority{col, priority})
		}

		sort.SliceStable(sortedColumns, func(i, j int) bool {
			return sortedColumns[i].priority < sortedColumns[j].priority
		})

		// Update columns array with sorted columns
		for i, col := range sortedColumns {
			idx.Columns[i] = col.name
		}
	}

	// Quote each column name
	quotedColumns := make([]string, len(idx.Columns))
	for i, col := range idx.Columns {
		quotedColumns[i] = idx.Quote.Wrap(col)
	}

	if idx.IsUnique {
		return fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s);",
			idx.Name, idx.Quote.Wrap(idx.TableName), strings.Join(quotedColumns, ", "))
	}

	return fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
		idx.Name, idx.Quote.Wrap(idx.TableName), strings.Join(quotedColumns, ", "))
}
