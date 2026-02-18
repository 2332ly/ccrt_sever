package models

import "gorm.io/gorm"

// SosEvent stores emergency call events for a user.
type SosEvent struct {
	gorm.Model
	UserID       uint   `json:"user_id" gorm:"index"`
	ContactName  string `json:"contact_name" gorm:"size:64"`
	ContactPhone string `json:"contact_phone" gorm:"size:32"`
	Note         string `json:"note" gorm:"size:255"`
}
