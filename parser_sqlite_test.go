package gem

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseModelToSQLWithIndexes_SQLite(t *testing.T) {
	createTable, indexes, err := parseModelToSQLWithIndexes(User{}, DialectSQLite)
	if err != nil {
		t.Fatalf("Failed to parse model: %v", err)
	}

	if !strings.Contains(createTable, "CREATE TABLE IF NOT EXISTS `users`") {
		t.Fatalf("invalid CREATE TABLE for SQLite: %s", createTable)
	}

	if !strings.Contains(createTable, "`id` INTEGER AUTOINCREMENT NOT NULL") {
		t.Fatalf("id column not using AUTOINCREMENT: %s", createTable)
	}

	expectedIndexes := []string{
		"CREATE INDEX `idx_name` ON `users` (`name`);",
		"CREATE UNIQUE INDEX `udx_email_address` ON `users` (`email_address`);",
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

func TestGetSQLType_SQLite(t *testing.T) {
	tests := []struct {
		name     string
		field    reflect.StructField
		expected string
	}{
		{
			name: "String with size",
			field: func() reflect.StructField {
				type T struct {
					F string `gorm:"size:30"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "VARCHAR(30)",
		},
		{
			name: "Auto increment int",
			field: func() reflect.StructField {
				type T struct {
					F int `gorm:"autoIncrement"`
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "INTEGER",
		},
		{
			name: "Pointer string nullable",
			field: func() reflect.StructField {
				type T struct {
					F *string
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "TEXT NULL",
		},
		{
			name: "Time type",
			field: func() reflect.StructField {
				type T struct {
					F time.Time
				}
				return reflect.TypeOf(T{}).Field(0)
			}(),
			expected: "DATETIME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSQLType(tt.field, DialectSQLite)
			if got != tt.expected {
				t.Fatalf("getSQLType() = %v, want %v", got, tt.expected)
			}
		})
	}
}
