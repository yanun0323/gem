package gem

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type BaseModel struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

type Address struct {
	Street  string `gorm:"size:100"`
	City    string `gorm:"size:50"`
	Country string `gorm:"size:50"`
}

type User struct {
	BaseModel
	Name       string    `gorm:"size:255;not null"`
	Email      string    `gorm:"unique;size:100"`
	Age        uint8     `gorm:"default:18"`
	Address    Address   `gorm:"embedded;embeddedPrefix:address_"`
	DeletedAt  time.Time `gorm:"-:all"`
	SecretNote string    `gorm:"column:secret_text;type:TEXT"`
}

func TestParseModelToSQL(t *testing.T) {
	tests := []struct {
		name     string
		model    interface{}
		contains []string
		excludes []string
	}{
		{
			name:  "基本模型測試",
			model: User{},
			contains: []string{
				"CREATE TABLE user",
				"id INTEGER PRIMARY KEY AUTO_INCREMENT",
				"created_at DATETIME NOT NULL",
				"updated_at DATETIME NOT NULL",
				"name VARCHAR(255) NOT NULL",
				"email VARCHAR(100) UNIQUE",
				"age TINYINT UNSIGNED DEFAULT 18",
				"address_street VARCHAR(100)",
				"address_city VARCHAR(50)",
				"address_country VARCHAR(50)",
				"secret_text TEXT",
			},
			excludes: []string{
				"deleted_at",
			},
		},
		{
			name:  "簡單結構體測試",
			model: Address{},
			contains: []string{
				"CREATE TABLE address",
				"street VARCHAR(100)",
				"city VARCHAR(50)",
				"country VARCHAR(50)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql := parseModelToSQL(tt.model)

			// 檢查必須包含的字串
			for _, contain := range tt.contains {
				if !strings.Contains(sql, contain) {
					t.Errorf("期望 SQL 包含 '%s', 但沒有找到\nSQL: %s", contain, sql)
				}
			}

			// 檢查必須排除的字串
			for _, exclude := range tt.excludes {
				if strings.Contains(sql, exclude) {
					t.Errorf("SQL 不應該包含 '%s', 但找到了\nSQL: %s", exclude, sql)
				}
			}
		})
	}
}

func TestParseModelToSQL_SpecialCases(t *testing.T) {
	type SpecialTypes struct {
		ID        int64   `gorm:"primaryKey"`
		Binary    []byte  `gorm:"type:BLOB"`
		FloatNum  float64 `gorm:"type:DECIMAL(10,2)"`
		IsActive  bool
		CreatedAt time.Time
	}

	sql := parseModelToSQL(SpecialTypes{})
	expectedParts := []string{
		"BLOB",
		"DECIMAL(10,2)",
		"BOOLEAN",
		"DATETIME",
	}

	for _, part := range expectedParts {
		if !strings.Contains(sql, part) {
			t.Errorf("期望 SQL 包含 '%s', 但沒有找到\nSQL: %s", part, sql)
		}
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"UserName", "user_name"},
		{"ID", "id"},
		{"APIKey", "api_key"},
		{"SimpleText", "simple_text"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toSnakeCase(tt.input)
			if result != tt.expected {
				t.Errorf("toSnakeCase(%s) = %s; 期望 %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetTagValue(t *testing.T) {
	type TestStruct struct {
		Field string `gorm:"size:255;column:custom_name;default:test"`
	}

	field, _ := reflect.TypeOf(TestStruct{}).FieldByName("Field")
	tests := []struct {
		key      string
		expected string
	}{
		{"size", "255"},
		{"column", "custom_name"},
		{"default", "test"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := getTagValue(field, tt.key)
			if result != tt.expected {
				t.Errorf("getTagValue(%s) = %s; 期望 %s", tt.key, result, tt.expected)
			}
		})
	}
}

type NullableFields struct {
	ID        uint    `gorm:"primaryKey"`
	Name      *string `gorm:"size:100"`
	Age       *int
	IsActive  *bool
	Score     *float64 `gorm:"type:DECIMAL(5,2)"`
	UpdatedAt *time.Time
}

func TestParseModelToSQL_NullableFields(t *testing.T) {
	sql := parseModelToSQL(NullableFields{})
	expectedParts := []string{
		"name VARCHAR(100) NULL",
		"age INTEGER NULL",
		"is_active BOOLEAN NULL",
		"score DECIMAL(5,2) NULL",
		"updated_at DATETIME NULL",
	}

	for _, part := range expectedParts {
		if !strings.Contains(sql, part) {
			t.Errorf("期望 SQL 包含 '%s', 但沒有找到\nSQL: %s", part, sql)
		}
	}
}

func TestParseModelToSQL_AdvancedFeatures(t *testing.T) {
	type AdvancedModel struct {
		ID    uint    `gorm:"primaryKey"`
		Price float64 `gorm:"precision:10;scale:2;check:price >= 0"`
		Name  string  `gorm:"index;comment:'用戶名稱'"`
		Email string  `gorm:"uniqueIndex"`
	}

	sql, indexes := parseModelToSQLWithIndexes(AdvancedModel{})

	// 打印實際的 SQL 和索引，方便調試
	t.Logf("Generated SQL: %s", sql)
	t.Logf("Generated Indexes: %v", indexes)

	expectedParts := []string{
		"price DECIMAL(10,2) CHECK (price >= 0) NOT NULL",
		"name VARCHAR(255) NOT NULL COMMENT '用戶名稱'",
	}

	for _, part := range expectedParts {
		if !strings.Contains(sql, part) {
			t.Errorf("期望 SQL 包含 '%s', 但沒有找到\nSQL: %s", part, sql)
		}
	}

	expectedIndexes := []string{
		"CREATE INDEX idx_name ON advanced_model (name);",
		"CREATE UNIQUE INDEX udx_email ON advanced_model (email);",
	}

	if len(indexes) != len(expectedIndexes) {
		t.Errorf("期望的索引數量為 %d，但實際為 %d", len(expectedIndexes), len(indexes))
	}

	for i, expectedIdx := range expectedIndexes {
		if i >= len(indexes) {
			t.Errorf("缺少索引: %s", expectedIdx)
			continue
		}
		if indexes[i] != expectedIdx {
			t.Errorf("索引不匹配:\n期望: %s\n實際: %s", expectedIdx, indexes[i])
		}
	}
}
