package models

import "gorm.io/gorm"

// CognitiveProfile stores the latest calculated cognitive ability scores for a user.
type CognitiveProfile struct {
	gorm.Model
	UserID           uint    `json:"user_id" gorm:"uniqueIndex;not null"`
	OrientationScore float64 `json:"orientation_score"`            // 定向力 0-100
	MemoryScore      float64 `json:"memory_score"`                 // 记忆力 0-100
	CalculationScore float64 `json:"calculation_score"`            // 注意力和计算力 0-100
	RecallScore      float64 `json:"recall_score"`                 // 回忆力 0-100
	LanguageScore    float64 `json:"language_score"`               // 语言能力 0-100
	OverallScore     float64 `json:"overall_score"`                // 综合评分 0-100
	OverallLevel     string  `json:"overall_level" gorm:"size:32"` // normal/mild/moderate/severe
	MmseSource       bool    `json:"mmse_source"`                  // 是否有MMSE数据参与
	GameSource       bool    `json:"game_source"`                  // 是否有游戏数据参与
}

// TrainingPlan stores an AI-generated personalized training plan.
type TrainingPlan struct {
	gorm.Model
	UserID           uint   `json:"user_id" gorm:"index;not null"`
	WeakDomains      string `json:"weak_domains" gorm:"type:text"`      // JSON array
	RecommendedGames string `json:"recommended_games" gorm:"type:text"` // JSON array
	DailyGoal        int    `json:"daily_goal"`
	AISummary        string `json:"ai_summary" gorm:"type:text"`
	AIModel          string `json:"ai_model" gorm:"size:64"`
}
