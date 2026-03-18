package models

import "gorm.io/gorm"

// CareContact stores the user's care circle contacts.
type CareContact struct {
	gorm.Model
	UserID       uint   `json:"user_id" gorm:"index;not null"`
	Name         string `json:"name" gorm:"size:64;not null"`
	Relationship string `json:"relationship" gorm:"size:32;not null;default:other"`
	Phone        string `json:"phone" gorm:"size:32;not null"`
	Note         string `json:"note" gorm:"size:255"`
	IsPrimary    bool   `json:"is_primary" gorm:"index;default:false"`
}
