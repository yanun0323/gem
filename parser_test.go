package gem

import (
	"reflect"
	"testing"
	"time"
)

// Shared test models
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
		name         string
		model        interface{}
		wantTable    string
		wantColCount int
		wantIdxCount int
	}{
		{
			name:         "Basic User Model",
			model:        User{},
			wantTable:    "users",
			wantColCount: 6,
			wantIdxCount: 2,
		},
		{
			name:         "Customer Model with Embedded Structure",
			model:        Customer{},
			wantTable:    "customers",
			wantColCount: 5,
			wantIdxCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tableName, columns, indexes := parseModel(tt.model)

			if tableName != tt.wantTable {
				t.Fatalf("Table name mismatch, got %v, want %v", tableName, tt.wantTable)
			}

			if len(columns) != tt.wantColCount {
				t.Fatalf("Column count mismatch, got %v, want %v", len(columns), tt.wantColCount)
			}

			if len(indexes) != tt.wantIdxCount {
				t.Fatalf("Index count mismatch, got %v, want %v", len(indexes), tt.wantIdxCount)
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
