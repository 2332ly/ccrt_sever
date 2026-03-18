package models

import (
	"time"

	"gorm.io/gorm"
)

// Reminder stores generic reminders such as habit, training, and task prompts.
type Reminder struct {
	gorm.Model
	UserID           uint       `json:"user_id" gorm:"index"`
	Type             string     `json:"type" gorm:"size:32;index;not null"`
	Title            string     `json:"title" gorm:"size:128;not null"`
	Description      string     `json:"description" gorm:"size:255"`
	ReminderTime     string     `json:"reminder_time" gorm:"size:64;not null"`
	RepeatRule       string     `json:"repeat_rule" gorm:"size:16;not null"`
	ReminderDate     *time.Time `json:"reminder_date"`
	StartDate        time.Time  `json:"start_date"`
	EndDate          *time.Time `json:"end_date"`
	ReminderChannels string     `json:"reminder_channels" gorm:"size:64"`
	AlertStyle       string     `json:"alert_style" gorm:"size:16"`
	Notes            string     `json:"notes" gorm:"size:255"`
}

// ReminderCheckin records completion or skip status for a scheduled reminder item.
type ReminderCheckin struct {
	gorm.Model
	UserID        uint       `json:"user_id" gorm:"index"`
	ReminderID    uint       `json:"reminder_id" gorm:"index"`
	ScheduledDate time.Time  `json:"scheduled_date" gorm:"index"`
	ScheduledTime string     `json:"scheduled_time" gorm:"size:16;index"`
	Status        string     `json:"status" gorm:"size:16;index"`
	CompletedAt   *time.Time `json:"completed_at"`
	Notes         string     `json:"notes" gorm:"size:255"`
}

// ReminderDispatch stores outbound reminder delivery attempts for non-app channels.
type ReminderDispatch struct {
	gorm.Model
	UserID        uint       `json:"user_id" gorm:"index"`
	ReminderID    uint       `json:"reminder_id" gorm:"index"`
	ScheduledDate time.Time  `json:"scheduled_date" gorm:"index"`
	ScheduledTime string     `json:"scheduled_time" gorm:"size:16;index"`
	Channel       string     `json:"channel" gorm:"size:32;index"`
	Status        string     `json:"status" gorm:"size:16"`
	SentAt        *time.Time `json:"sent_at"`
	ErrorMessage  string     `json:"error_message" gorm:"size:255"`
}
