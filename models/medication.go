package models

import (
	"time"

	"gorm.io/gorm"
)

// Medication 吃药提醒模型
type Medication struct {
	gorm.Model
	UserID           uint       `json:"user_id" gorm:"index"` // 关联用户ID
	Name             string     `json:"name" gorm:"not null"` // 药品名称
	Dosage           string     `json:"dosage"`               // 剂量 (如: 1粒, 5ml)
	Frequency        string     `json:"frequency"`            // 频率描述 (如: 每日一次)
	ReminderTime     string     `json:"reminder_time"`        // 提醒时间 (如: "08:00,20:00")
	ReminderChannels string     `json:"reminder_channels"`    // 提醒渠道 (如: "app,sms,voice")
	AlertStyle       string     `json:"alert_style"`          // 提醒强度 (strong/normal)
	StartDate        time.Time  `json:"start_date"`           // 开始时间
	EndDate          *time.Time `json:"end_date"`             // 结束时间 (可选)
	Notes            string     `json:"notes"`                // 备注
}
