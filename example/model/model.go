package model

import (
	"database/sql"
	"time"
)

// Model is a struct for testing all GORM tag features
// Basic fields and primary key
// Various data types and constraints
// JSON serialization
// Read and write permission control
// Embedded structure
// Nullable fields
// Address is an embedded sub-struct
type Model struct {
	// Basic fields and primary key
	ID        uint      `gorm:"column:id;primaryKey;autoIncrement;autoIncrementIncrement:2"`
	UUID      string    `gorm:"column:uuid;type:varchar(36);unique;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt int64     `gorm:"column:updated_at;type:bigint;autoUpdateTime"`

	// Various data types and constraints
	Name        string    `gorm:"column:name;type:varchar(100);not null;uniqueIndex"`
	Age         int       `gorm:"column:age;check:age > 0"`
	Email       string    `gorm:"column:email;size:255;uniqueIndex:udx_email_score_is_active"`
	Score       float64   `gorm:"column:score;precision:10;scale:2;default:0.00;uniqueIndex:udx_email_score_is_active"`
	IsActive    bool      `gorm:"column:is_active;default:true;uniqueIndex:udx_email_score_is_active"`
	Birthday    time.Time `gorm:"column:birth_day;type:date;index"`
	Description string    `gorm:"column:description;type:text;comment:User notes"`

	// JSON serialization
	Settings map[string]interface{} `gorm:"column:settings;serializer:json"`

	// Read and write permission control
	ReadOnly   string `gorm:"column:read_only;->"`
	WriteOnly  string `gorm:"column:write_only;<-"`
	CreateOnly string `gorm:"column:create_only;<-:create"`
	UpdateOnly string `gorm:"column:update_only;<-:update"`
	Ignored    string `gorm:"-"`
	NoMigrate  string `gorm:"-:migration"`

	// Embedded structure
	Address Address `gorm:"embedded;embeddedPrefix:addr_"`

	// Nullable fields
	DeletedAt    sql.NullTime   `gorm:"column:deleted_at;index:idx_deleted_at_new"`
	OptionalData sql.NullString `gorm:"column:optional_data"`
}

func (Model) TableName() string {
	return "models"
}

// Address is an embedded sub-struct
type Address struct {
	Street  string `gorm:"column:street;type:varchar(255);uniqueIndex:udx_address_street_city_country"`
	City    string `gorm:"column:city;type:varchar(100);uniqueIndex:udx_address_street_city_country"`
	Country string `gorm:"column:country;type:varchar(100);uniqueIndex:udx_address_street_city_country"`
}
