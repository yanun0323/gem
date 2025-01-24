package model

type UserEmbed struct {
	User
}

func (UserEmbed) TableName() string {
	return "users_new"
}
