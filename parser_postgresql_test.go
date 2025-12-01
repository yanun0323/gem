package gem

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseModelToSQLWithIndexes_PostgreSQL(t *testing.T) {
	createTable, indexes, err := parseModelToSQLWithIndexes(User{}, DialectPostgreSQL)
	if err != nil {
		t.Fatalf("Failed to parse model: %v", err)
	}

	if !strings.Contains(createTable, "CREATE TABLE IF NOT EXISTS \"users\"") {
		t.Fatalf("invalid CREATE TABLE for PostgreSQL: %s", createTable)
	}

	if !strings.Contains(createTable, "\"id\" SERIAL NOT NULL") {
		t.Fatalf("id column not using SERIAL: %s", createTable)
	}

	expectedIndexes := []string{
		"CREATE INDEX \"idx_name\" ON \"users\" (\"name\");",
		"CREATE UNIQUE INDEX \"udx_email_address\" ON \"users\" (\"email_address\");",
	}

	if len(indexes) != len(expectedIndexes) {
		t.Fatalf("Incorrect index count, expected %d, got %d", len(expectedIndexes), len(indexes))
	}

	for i, idx := range indexes {
		if indexes[i] != expectedIndexes[i] {
			t.Fatalf("Index Mismatch\nexpected: %s\ngot: %s", expectedIndexes[i], idx)
		}
	}
}

func TestGetSQLType_PostgreSQL(t *testing.T) {
	tests := []struct {
		name     string
		field    reflect.StructField
		expected string
	}{
		{
			name: "String with size",
			field: func() reflect.StructField {
				type T struct {
					F string `gorm:"size:50"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "VARCHAR(50)",
		},
		{
			name: "Auto increment int -> SERIAL",
			field: func() reflect.StructField {
				type T struct {
					F int `gorm:"autoIncrement"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "SERIAL",
		},
		{
			name: "Auto increment int64 -> BIGSERIAL",
			field: func() reflect.StructField {
				type T struct {
					F int64 `gorm:"autoIncrement"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "BIGSERIAL",
		},
		{
			name: "Nullable pointer string",
			field: func() reflect.StructField {
				type T struct {
					F *string
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "VARCHAR(255) NULL",
		},
		{
			name: "Time type",
			field: func() reflect.StructField {
				type T struct {
					F time.Time
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "TIMESTAMP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSQLType(tt.field, DialectPostgreSQL)
			if got != tt.expected {
				t.Fatalf("getSQLType() = %v, want %v", got, tt.expected)
			}
		})
	}
}
