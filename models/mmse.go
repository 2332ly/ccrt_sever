package models

import (
	"time"

	"gorm.io/gorm"
)

type Scale struct {
	gorm.Model
	Name       string `json:"name" gorm:"uniqueIndex;size:64;not null"`
	TotalScore int    `json:"total_score"`
}

type ScaleVersion struct {
	gorm.Model
	ScaleID    uint          `json:"scale_id" gorm:"index"`
	Scale      Scale         `json:"-"`
	Name       string        `json:"name" gorm:"size:64;not null"`
	Version    string        `json:"version" gorm:"size:32;not null"`
	TotalScore int           `json:"total_score"`
	IsActive   bool          `json:"is_active" gorm:"index"`
	Modules    []ScaleModule `json:"modules" gorm:"foreignKey:ScaleVersionID"`
}

type ScaleModule struct {
	gorm.Model
	ScaleVersionID uint            `json:"scale_version_id" gorm:"index"`
	Name           string          `json:"name" gorm:"size:128;not null"`
	MaxScore       int             `json:"max_score"`
	SortOrder      int             `json:"sort_order"`
	Questions      []ScaleQuestion `json:"questions" gorm:"foreignKey:ModuleID"`
}

type ScaleQuestion struct {
	gorm.Model
	ModuleID   uint   `json:"module_id" gorm:"index"`
	Type       string `json:"type" gorm:"size:32;not null"`
	Content    string `json:"content" gorm:"type:text"`
	MaxScore   int    `json:"max_score"`
	AnswerRule string `json:"answer_rule" gorm:"type:text"`
	SortOrder  int    `json:"sort_order"`
}

type ScaleAssessment struct {
	gorm.Model
	UserID         uint       `json:"user_id" gorm:"index"`
	ScaleVersionID uint       `json:"scale_version_id" gorm:"index"`
	TotalScore     int        `json:"total_score"`
	Level          string     `json:"level" gorm:"size:32"`
	AIScore        int        `json:"ai_score"`
	AILevel        string     `json:"ai_level" gorm:"size:32"`
	AIComment      string     `json:"ai_comment" gorm:"type:text"`
	AIResult       string     `json:"ai_result" gorm:"type:text"`
	AIModel        string     `json:"ai_model" gorm:"size:64"`
	AIAt           *time.Time `json:"ai_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

type ScaleAnswer struct {
	gorm.Model
	AssessmentID uint   `json:"assessment_id" gorm:"index"`
	UserID       uint   `json:"user_id" gorm:"index"`
	QuestionID   uint   `json:"question_id" gorm:"index"`
	UserAnswer   string `json:"user_answer" gorm:"type:text"`
	Score        int    `json:"score"`
}

type AssessmentModuleScore struct {
	gorm.Model
	AssessmentID uint `json:"assessment_id" gorm:"index"`
	ModuleID     uint `json:"module_id" gorm:"index"`
	Score        int  `json:"score"`
}
