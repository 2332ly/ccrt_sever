package models

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Username string `gorm:"unique" json:"username"`
	Phone    string `gorm:"unique" json:"phone"`
	Password string `json:"password"`
}
