package gem

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Test model structure
type User struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"size:100;not null;index:idx_name"`
	Email     string    `gorm:"column:email_address;size:150;uniqueIndex"`
	Age       *int      `gorm:"default:18"`
	CreatedAt time.Time `gorm:"type:DATETIME;not null"`
	UpdatedAt time.Time `gorm:"type:DATETIME;not null"`
}

func (User) TableName() string {
	return "users"
}

// Test embedded structure
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
	tests := []struct {
		name      string
		model     interface{}
		quoteChar rune
		wantCols  []string
	}{
		{
			name:      "MySQL style quotes",
			model:     User{},
			quoteChar: '`',
			wantCols: []string{
				"`id` INTEGER UNSIGNED AUTO_INCREMENT NOT NULL",
				"`name` VARCHAR(100) NOT NULL",
				"`email_address` VARCHAR(150) NOT NULL",
				"`age` INTEGER NULL DEFAULT 18",
				"`created_at` DATETIME NOT NULL",
				"`updated_at` DATETIME NOT NULL",
			},
		},
		{
			name:      "PostgreSQL style quotes",
			model:     User{},
			quoteChar: '"',
			wantCols: []string{
				"\"id\" INTEGER UNSIGNED AUTO_INCREMENT NOT NULL",
				"\"name\" VARCHAR(100) NOT NULL",
				"\"email_address\" VARCHAR(150) NOT NULL",
				"\"age\" INTEGER NULL DEFAULT 18",
				"\"created_at\" DATETIME NOT NULL",
				"\"updated_at\" DATETIME NOT NULL",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote := newQuote(tt.quoteChar)
			tableName, columns, indexes := parseModel(tt.model, quote)

			// Verify table name
			if tableName != "users" {
				t.Errorf("parseModel() tableName = %v, want %v", tableName, "users")
			}

			// Verify columns
			for _, wantCol := range tt.wantCols {
				found := false
				for _, col := range columns {
					if strings.TrimSpace(col) == strings.TrimSpace(wantCol) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("parseModel() missing or incorrect column\nwant: %v\ngot columns: %v", wantCol, columns)
				}
			}

			// Verify indexes
			if len(indexes) != 2 {
				t.Errorf("parseModel() got %v indexes, want 2", len(indexes))
			}

			// Check name index
			if idx, ok := indexes["idx_name"]; !ok {
				t.Error("parseModel() missing name index")
			} else if !containsColumn(idx.Columns, "name") {
				t.Error("parseModel() name index doesn't contain 'name' column")
			}

			// Check email unique index
			if idx, ok := indexes["udx_email_address"]; !ok {
				t.Error("parseModel() missing email_address unique index")
			} else if !idx.IsUnique {
				t.Error("parseModel() email_address index should be unique")
			} else if !containsColumn(idx.Columns, "email_address") {
				t.Error("parseModel() email_address index doesn't contain 'email_address' column")
			}
		})
	}
}

func containsColumn(columns []string, target string) bool {
	for _, col := range columns {
		if col == target {
			return true
		}
	}
	return false
}

func TestParseModelToSQLWithIndexes(t *testing.T) {
	tests := []struct {
		name      string
		model     interface{}
		quoteChar rune
		wantTable string
		wantIndex []string
	}{
		{
			name:      "MySQL style quotes",
			model:     User{},
			quoteChar: '`',
			wantTable: "CREATE TABLE IF NOT EXISTS `users` (",
			wantIndex: []string{
				"CREATE INDEX idx_name ON `users` (`name`);",
				"CREATE UNIQUE INDEX udx_email_address ON `users` (`email_address`);",
			},
		},
		{
			name:      "PostgreSQL style quotes",
			model:     User{},
			quoteChar: '"',
			wantTable: "CREATE TABLE IF NOT EXISTS \"users\" (",
			wantIndex: []string{
				"CREATE INDEX idx_name ON \"users\" (\"name\");",
				"CREATE UNIQUE INDEX udx_email_address ON \"users\" (\"email_address\");",
			},
		},
		{
			name:      "Default style (MySQL) when quoteChar is 0",
			model:     User{},
			quoteChar: 0,
			wantTable: "CREATE TABLE IF NOT EXISTS `users` (",
			wantIndex: []string{
				"CREATE INDEX idx_name ON `users` (`name`);",
				"CREATE UNIQUE INDEX udx_email_address ON `users` (`email_address`);",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote := newQuote(tt.quoteChar)
			createTable, indexes, err := parseModelToSQLWithIndexes(tt.model, quote)
			if err != nil {
				t.Fatalf("Failed to parse model: %v", err)
			}

			// Validate CREATE TABLE statement
			if !strings.HasPrefix(createTable, tt.wantTable) {
				t.Errorf("Invalid CREATE TABLE statement format\nwant prefix: %s\ngot: %s", tt.wantTable, createTable)
			}

			// Validate if all necessary columns are included with correct quotes
			requiredColumns := []string{
				"id",
				"name",
				"email_address",
				"age",
				"created_at",
				"updated_at",
			}

			for _, col := range requiredColumns {
				quotedCol := quote.Wrap(col)
				if !strings.Contains(createTable, quotedCol) {
					t.Errorf("Missing column %s (quoted as %s)", col, quotedCol)
				}
			}

			// Validate index count
			if len(indexes) != len(tt.wantIndex) {
				t.Fatalf("Incorrect index count, expected %d, got %d", len(tt.wantIndex), len(indexes))
			}

			// Sort both expected and actual indexes for comparison
			sort.Strings(tt.wantIndex)
			sort.Strings(indexes)

			for i, index := range indexes {
				if index != tt.wantIndex[i] {
					t.Errorf("Index Mismatch\nexpected: %s\nbut got : %s", tt.wantIndex[i], index)
				}
			}
		})
	}
}

func TestQuote(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		quoteChar rune
		want      string
	}{
		{
			name:      "MySQL style quote",
			input:     "column_name",
			quoteChar: '`',
			want:      "`column_name`",
		},
		{
			name:      "PostgreSQL style quote",
			input:     "column_name",
			quoteChar: '"',
			want:      "\"column_name\"",
		},
		{
			name:      "MSSQL style quote",
			input:     "column_name",
			quoteChar: '[',
			want:      "[column_name]",
		},
		{
			name:      "Default quote (MySQL) when quoteChar is 0",
			input:     "column_name",
			quoteChar: 0,
			want:      "`column_name`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote := newQuote(tt.quoteChar)
			got := quote.Wrap(tt.input)
			if got != tt.want {
				t.Errorf("quote() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseField(t *testing.T) {
	type TestStruct struct {
		Name        string    `gorm:"size:255;not null"`
		Email       string    `gorm:"unique;size:100"`
		Age         int       `gorm:"type:integer"`
		OptionalPtr *string   `gorm:"size:50"`
		CreatedAt   time.Time `gorm:"autoCreateTime;comment:'Creation time'"`
	}

	testType := reflect.TypeOf(TestStruct{})
	tests := []struct {
		name      string
		field     string
		quoteChar rune
		contains  []string
	}{
		{
			name:      "MySQL quotes with constraints",
			field:     "Name",
			quoteChar: '`',
			contains: []string{
				"`name`",
				"VARCHAR(255)",
				"NOT NULL",
			},
		},
		{
			name:      "PostgreSQL quotes with constraints",
			field:     "Name",
			quoteChar: '"',
			contains: []string{
				"\"name\"",
				"VARCHAR(255)",
				"NOT NULL",
			},
		},
		{
			name:      "Nullable pointer field",
			field:     "OptionalPtr",
			quoteChar: '`',
			contains: []string{
				"`optional_ptr`",
				"VARCHAR(50)",
				"NULL",
			},
		},
		{
			name:      "Field with comment",
			field:     "CreatedAt",
			quoteChar: '"',
			contains: []string{
				"\"created_at\"",
				"DATETIME",
				"COMMENT 'Creation time'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote := newQuote(tt.quoteChar)
			field, found := testType.FieldByName(tt.field)
			if !found {
				t.Fatalf("Field %s not found", tt.field)
			}
			got := parseField(field, quote)

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("parseField() = %v, should contain %v", got, want)
				}
			}
		})
	}
}

func TestGetSQLType(t *testing.T) {
	tests := []struct {
		name     string
		field    reflect.StructField
		expected string
	}{
		{
			name: "String with size",
			field: func() reflect.StructField {
				type T struct {
					F string `gorm:"size:100"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "VARCHAR(100)",
		},
		{
			name: "Integer with auto increment",
			field: func() reflect.StructField {
				type T struct {
					F uint `gorm:"autoIncrement"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "INTEGER UNSIGNED",
		},
		{
			name: "Nullable pointer",
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
				t.Fatalf("getSQLType() = %v, want %v", got, tt.expected)
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
				t.Fatalf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
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
				t.Fatalf("getTagValue() with key %q = %q, want %q", tt.key, got, tt.expected)
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
		{"Existing tag", "Field1", "not null", true},
		{"Non-existent tag", "Field1", "unique", false},
		{"Tag with value", "Field2", "size", true},
	}

	typ := reflect.TypeOf(TestStruct{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, _ := typ.FieldByName(tt.field)
			got := hasTag(field, tt.tag)
			if got != tt.expected {
				t.Fatalf("hasTag() = %v, want %v", got, tt.expected)
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
				t.Fatalf("getColumnName() = %v, want %v", got, tt.expected)
			}
		})
	}
}
