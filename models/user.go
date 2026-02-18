package models

import (
	"time"

	"gorm.io/gorm"
)

// User basic account + profile info
type User struct {
	gorm.Model
	Username string `gorm:"unique;size:64" json:"username"`
	Phone    string `gorm:"unique;size:32" json:"phone"`
	Password string `json:"password"`

	// Profile
	Nickname      string     `json:"nickname" gorm:"size:64"`
	RealName      string     `json:"real_name" gorm:"size:64"`
	Gender        string     `json:"gender" gorm:"size:16"` // male/female/other/unknown
	Birthday      *time.Time `json:"birthday"`
	AvatarURL     string     `json:"avatar_url" gorm:"size:255"`
	City          string     `json:"city" gorm:"size:64"`
	Address       string     `json:"address" gorm:"size:255"`
	IDNumber      string     `json:"id_number" gorm:"size:64;index"`
	MedicalNo     string     `json:"medical_no" gorm:"size:64;index"`
	Education     string     `json:"education" gorm:"size:32"`
	MaritalStatus string     `json:"marital_status" gorm:"size:32"`

	// Health
	HeightCM  int    `json:"height_cm"` // centimeters
	WeightKG  int    `json:"weight_kg"` // kilograms
	BloodType string `json:"blood_type" gorm:"size:8"`

	// Emergency contact
	EmergencyContactName  string `json:"emergency_contact_name" gorm:"size:64"`
	EmergencyContactPhone string `json:"emergency_contact_phone" gorm:"size:32"`
}
