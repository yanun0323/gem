package model

type UserAlias User

func (UserAlias) TableName() string {
	return "users_alias"
}
