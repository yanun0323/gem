package gem

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// 測試用的模型結構
type User struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"size:100;not null;index:idx_name"`
	Email     string    `gorm:"size:150;uniqueIndex:udx_email"`
	Age       *int      `gorm:"default:18"`
	CreatedAt time.Time `gorm:"type:DATETIME;not null"`
	UpdatedAt time.Time `gorm:"type:DATETIME;not null"`
}

func (User) TableName() string {
	return "users"
}

// 測試嵌入結構
type Address struct {
	Street  string `gorm:"size:200;not null"`
	City    string `gorm:"size:100"`
	Country string `gorm:"size:100"`
}

type Customer struct {
	ID      uint    `gorm:"primaryKey"`
	Name    string  `gorm:"size:100"`
	Address Address `gorm:"embedded;embeddedPrefix:address_"`
}

func TestParseModel(t *testing.T) {
	conf := &MigratorConfig{
		IndexPrefix:       "idx_",
		UniqueIndexPrefix: "udx_",
	}

	tests := []struct {
		name         string
		model        interface{}
		wantTable    string
		wantColCount int
		wantIdxCount int
	}{
		{
			name:         "基本用戶模型",
			model:        User{},
			wantTable:    "users",
			wantColCount: 6,
			wantIdxCount: 2,
		},
		{
			name:         "帶嵌入結構的客戶模型",
			model:        Customer{},
			wantTable:    "customer",
			wantColCount: 5,
			wantIdxCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tableName, columns, indexes := parseModel(tt.model, conf)

			if tableName != tt.wantTable {
				t.Errorf("表名不匹配，got %v, want %v", tableName, tt.wantTable)
			}

			if len(columns) != tt.wantColCount {
				t.Errorf("列數量不匹配，got %v, want %v", len(columns), tt.wantColCount)
			}

			if len(indexes) != tt.wantIdxCount {
				t.Errorf("索引數量不匹配，got %v, want %v", len(indexes), tt.wantIdxCount)
			}
		})
	}
}

func TestParseModelToSQLWithIndexes(t *testing.T) {
	conf := &MigratorConfig{
		IndexPrefix:       "idx_",
		UniqueIndexPrefix: "udx_",
	}

	createTable, indexes, err := parseModelToSQLWithIndexes(User{}, conf)
	if err != nil {
		t.Fatalf("解析模型失敗: %v", err)
	}

	// 驗證 CREATE TABLE 語句
	if !strings.Contains(createTable, "CREATE TABLE `users`") {
		t.Error("CREATE TABLE 語句格式錯誤")
	}

	// 驗證是否包含所有必要的列
	requiredColumns := []string{
		"`id`",
		"`name`",
		"`email`",
		"`age`",
		"`created_at`",
		"`updated_at`",
	}

	for _, col := range requiredColumns {
		if !strings.Contains(createTable, col) {
			t.Errorf("缺少列 %s", col)
		}
	}

	// 驗證索引數量
	if len(indexes) != 2 {
		t.Errorf("索引數量不正確，期望 2，得到 %d", len(indexes))
	}
}

func TestGetSQLType(t *testing.T) {
	tests := []struct {
		name     string
		field    reflect.StructField
		expected string
	}{
		{
			name: "字符串帶大小",
			field: func() reflect.StructField {
				type T struct {
					F string `gorm:"size:100"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "VARCHAR(100)",
		},
		{
			name: "整數自增",
			field: func() reflect.StructField {
				type T struct {
					F uint `gorm:"autoIncrement"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "INTEGER UNSIGNED",
		},
		{
			name: "可空指針",
			field: func() reflect.StructField {
				type T struct {
					F *string
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "VARCHAR(255) NULL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSQLType(tt.field)
			if got != tt.expected {
				t.Errorf("getSQLType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ID", "id"},
		{"UserName", "user_name"},
		{"APIKey", "api_key"},
		{"OAuthToken", "oauth_token"},
		{"OAuth2Token", "oauth2_token"},
		{"SimpleURL", "simple_url"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toSnakeCase(tt.input)
			if got != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetTagValue(t *testing.T) {
	type TestStruct struct {
		Field string `gorm:"size:100;not null;index:idx_field"`
	}

	field := reflect.TypeOf(TestStruct{}).Field(0)

	tests := []struct {
		key      string
		expected string
	}{
		{"size", "100"},
		{"not null", ""},
		{"index", "idx_field"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := getTagValue(field, tt.key)
			if got != tt.expected {
				t.Errorf("getTagValue() with key %q = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

func TestHasTag(t *testing.T) {
	type TestStruct struct {
		Field1 string `gorm:"not null;index"`
		Field2 string `gorm:"size:100"`
	}

	tests := []struct {
		name     string
		field    string
		tag      string
		expected bool
	}{
		{"存在的標籤", "Field1", "not null", true},
		{"不存在的標籤", "Field1", "unique", false},
		{"帶值的標籤", "Field2", "size", true},
	}

	typ := reflect.TypeOf(TestStruct{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, _ := typ.FieldByName(tt.field)
			got := hasTag(field, tt.tag)
			if got != tt.expected {
				t.Errorf("hasTag() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetColumnName(t *testing.T) {
	type TestStruct struct {
		UserID    string `gorm:"column:user_identifier"`
		FirstName string
		LastName  string `gorm:"column:surname"`
	}

	tests := []struct {
		fieldName string
		expected  string
	}{
		{"UserID", "user_identifier"},
		{"FirstName", "first_name"},
		{"LastName", "surname"},
	}

	typ := reflect.TypeOf(TestStruct{})
	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			field, _ := typ.FieldByName(tt.fieldName)
			got := getColumnName(field)
			if got != tt.expected {
				t.Errorf("getColumnName() = %v, want %v", got, tt.expected)
			}
		})
	}
}
