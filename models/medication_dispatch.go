package models

import (
	"time"

	"gorm.io/gorm"
)

// MedicationDispatch tracks notification delivery attempts.
type MedicationDispatch struct {
	gorm.Model
	UserID        uint       `json:"user_id" gorm:"uniqueIndex:uniq_med_dispatch"`
	MedicationID  uint       `json:"medication_id" gorm:"uniqueIndex:uniq_med_dispatch"`
	ScheduledDate time.Time  `json:"scheduled_date" gorm:"type:date;uniqueIndex:uniq_med_dispatch;index"`
	ScheduledTime string     `json:"scheduled_time" gorm:"size:8;uniqueIndex:uniq_med_dispatch"`
	Channel       string     `json:"channel" gorm:"size:16;uniqueIndex:uniq_med_dispatch"`
	Status        string     `json:"status" gorm:"size:16"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
	ErrorMessage  string     `json:"error_message"`
}
