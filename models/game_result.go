package models

import "gorm.io/gorm"

// GameResult stores rehab game outcomes for a user.
type GameResult struct {
	gorm.Model
	UserID     uint    `json:"user_id" gorm:"index"`
	GameName   string  `json:"game_name" gorm:"size:64;not null;index"`
	Score      int     `json:"score"`
	DurationMs int     `json:"duration_ms"`
	Difficulty int     `json:"difficulty"`
	Accuracy   float64 `json:"accuracy"`
	Success    bool    `json:"success"`
	Answers    string  `json:"answers" gorm:"type:text"` // JSON payload with answers
	Meta       string  `json:"meta" gorm:"type:text"`    // JSON payload with extra info
}
