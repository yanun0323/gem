package model

import (
	"database/sql"
	"time"
)

// Model 是一個用於測試所有 GORM tag 功能的結構體
type Model struct {
	// 基本欄位和主鍵
	ID        uint      `gorm:"column:id;primaryKey;autoIncrement;autoIncrementIncrement:2"`
	UUID      string    `gorm:"column:uuid;type:varchar(36);unique;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt int64     `gorm:"column:updated_at;type:bigint;autoUpdateTime"`

	// 各種數據類型和約束
	Name        string    `gorm:"column:name;type:varchar(100);not null;index:idx_name_age"`
	Age         int       `gorm:"column:age;check:age > 0;index:idx_name_age"`
	Email       string    `gorm:"column:email;size:255;uniqueIndex:udx_email_score_is_active"`
	Score       float64   `gorm:"column:score;precision:10;scale:2;default:0.00;uniqueIndex:udx_email_score_is_active"`
	IsActive    bool      `gorm:"column:is_active;default:true;uniqueIndex:udx_email_score_is_active"`
	Birthday    time.Time `gorm:"column:birth_day;type:date"`
	Description string    `gorm:"column:description;type:text;comment:使用者備註"`

	// JSON 序列化
	Settings map[string]interface{} `gorm:"column:settings;serializer:json"`

	// 唯讀和寫入權限控制
	ReadOnly   string `gorm:"column:read_only;->"`
	WriteOnly  string `gorm:"column:write_only;<-"`
	CreateOnly string `gorm:"column:create_only;<-:create"`
	UpdateOnly string `gorm:"column:update_only;<-:update"`
	Ignored    string `gorm:"-"`
	NoMigrate  string `gorm:"-:migration"`

	// 嵌入結構
	Address Address `gorm:"embedded;embeddedPrefix:addr_"`

	// 可為空的欄位
	DeletedAt    sql.NullTime   `gorm:"column:deleted_at;index:idx_deleted_at_new"`
	OptionalData sql.NullString `gorm:"column:optional_data"`
}

func (Model) TableName() string {
	return "models"
}

// Address 是嵌入的子結構體
type Address struct {
	Street  string `gorm:"column:street;type:varchar(255);uniqueIndex:udx_address_street_city_country"`
	City    string `gorm:"column:city;type:varchar(100);uniqueIndex:udx_address_street_city_country"`
	Country string `gorm:"column:country;type:varchar(100);uniqueIndex:udx_address_street_city_country"`
}
