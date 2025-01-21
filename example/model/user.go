package model

import "time"

type User struct {
	ID        uint      `gorm:"column:id;primaryKey;autoIncrement;autoIncrementIncrement:2"`
	UUID      string    `gorm:"column:uuid;type:varchar(36);unique;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt int64     `gorm:"column:updated_at;type:bigint;autoUpdateTime"`
}

func (User) TableName() string {
	return "users"
}
