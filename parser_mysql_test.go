package gem

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseModelToSQLWithIndexes_MySQL(t *testing.T) {
	createTable, indexes, err := parseModelToSQLWithIndexes(User{}, DialectMySQL)
	if err != nil {
		t.Fatalf("Failed to parse model: %v", err)
	}

	// Validate CREATE TABLE statement
	if !strings.Contains(createTable, "CREATE TABLE IF NOT EXISTS `users`") {
		t.Fatal("Invalid CREATE TABLE statement format")
	}

	// Validate if all necessary columns are included
	requiredColumns := []string{
		"`id`",
		"`name`",
		"`email_address`",
		"`age`",
		"`created_at`",
		"`updated_at`",
	}

	for _, col := range requiredColumns {
		if !strings.Contains(createTable, col) {
			t.Fatalf("Missing column %s", col)
		}
	}

	expectedIndex := []string{
		"CREATE INDEX `idx_name` ON `users` (`name`);",
		"CREATE UNIQUE INDEX `udx_email_address` ON `users` (`email_address`);",
	}

	// Validate index count
	if len(indexes) != len(expectedIndex) {
		t.Fatalf("Incorrect index count, expected 2, got %d", len(indexes))
	}

	for i, index := range indexes {
		if !strings.Contains(index, expectedIndex[i]) {
			t.Fatalf("Index Mismatch\nexpected: %s\nbut got : %s\n", expectedIndex[i], index)
		}
	}

}

func TestGetSQLType_MySQL(t *testing.T) {
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
			got := getSQLType(tt.field, DialectMySQL)
			if got != tt.expected {
				t.Fatalf("getSQLType() = %v, want %v", got, tt.expected)
			}
		})
	}
}
