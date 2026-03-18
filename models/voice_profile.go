package models

import (
	"time"

	"gorm.io/gorm"
)

// VoiceProfile stores cloned family voice metadata for a user.
type VoiceProfile struct {
	gorm.Model
	UserID             uint       `gorm:"index" json:"user_id"`
	DisplayName        string     `gorm:"size:64" json:"display_name"`
	Relationship       string     `gorm:"size:64" json:"relationship"`
	Vendor             string     `gorm:"size:32;index" json:"vendor"`
	VendorVoiceID      string     `gorm:"size:128;index" json:"vendor_voice_id"`
	VendorTaskID       string     `gorm:"size:128" json:"vendor_task_id"`
	Status             string     `gorm:"size:32;index" json:"status"`
	IsDefault          bool       `gorm:"index" json:"is_default"`
	SourceType         string     `gorm:"size:16" json:"source_type"`
	SampleFileName     string     `gorm:"size:255" json:"sample_file_name"`
	SampleDurationMs   int64      `json:"sample_duration_ms"`
	LastError          string     `gorm:"size:512" json:"last_error"`
	ConsentConfirmedAt *time.Time `json:"consent_confirmed_at"`
	ReadyAt            *time.Time `json:"ready_at"`
}
