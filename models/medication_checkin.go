package models

import (
	"time"

	"gorm.io/gorm"
)

// MedicationCheckin tracks a user's medication intake status.
type MedicationCheckin struct {
	gorm.Model
	UserID        uint       `json:"user_id" gorm:"uniqueIndex:uniq_med_checkin"`
	MedicationID  uint       `json:"medication_id" gorm:"uniqueIndex:uniq_med_checkin"`
	ScheduledDate time.Time  `json:"scheduled_date" gorm:"type:date;uniqueIndex:uniq_med_checkin;index"`
	ScheduledTime string     `json:"scheduled_time" gorm:"size:8;uniqueIndex:uniq_med_checkin"`
	Status        string     `json:"status" gorm:"size:16"`
	TakenAt       *time.Time `json:"taken_at,omitempty"`
	Notes         string     `json:"notes"`
}
