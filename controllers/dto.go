package controllers

import "encoding/json"

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Phone    string `json:"phone" binding:"required,min=6,max=32"`
	Password string `json:"password" binding:"required,min=6,max=128"`
	SMSCode  string `json:"sms_code" binding:"required,min=4,max=8"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"omitempty,min=3,max=64"`
	Phone    string `json:"phone" binding:"omitempty,min=6,max=32"`
	Password string `json:"password" binding:"omitempty,min=6,max=128"`
	SMSCode  string `json:"sms_code" binding:"omitempty,min=4,max=8"`
}

type SendSMSRequest struct {
	Phone string `json:"phone" binding:"required,min=6,max=32"`
}

type VerifySMSRequest struct {
	Phone   string `json:"phone" binding:"required,min=6,max=32"`
	SMSCode string `json:"sms_code" binding:"required,min=4,max=8"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type MedicationRequest struct {
	Name             string `json:"name" binding:"required"`
	Dosage           string `json:"dosage"`
	Frequency        string `json:"frequency"`
	ReminderTime     string `json:"reminder_time"`
	ReminderChannels string `json:"reminder_channels"`
	AlertStyle       string `json:"alert_style"`
	StartDate        string `json:"start_date" binding:"required"`
	EndDate          string `json:"end_date"`
	Notes            string `json:"notes"`
}

type ReminderRequest struct {
	Type             string `json:"type" binding:"required"`
	Title            string `json:"title" binding:"required"`
	Description      string `json:"description"`
	ReminderTime     string `json:"reminder_time" binding:"required"`
	RepeatRule       string `json:"repeat_rule"`
	ReminderDate     string `json:"reminder_date"`
	StartDate        string `json:"start_date"`
	EndDate          string `json:"end_date"`
	ReminderChannels string `json:"reminder_channels"`
	AlertStyle       string `json:"alert_style"`
	Notes            string `json:"notes"`
}

type MMSESubmitRequest struct {
	ScaleVersionID uint              `json:"scale_version_id"`
	Answers        []MMSEAnswerInput `json:"answers" binding:"required"`
}

type MMSEArtifactRefInput struct {
	Kind         string          `json:"kind" binding:"required"`
	ArtifactKey  string          `json:"artifact_key" binding:"required"`
	ReviewStatus string          `json:"review_status,omitempty"`
	AutoScore    *int            `json:"auto_score,omitempty"`
	Confidence   *float64        `json:"confidence,omitempty"`
	Meta         json.RawMessage `json:"meta,omitempty"`
}

type MMSEAnswerInput struct {
	QuestionID     uint                   `json:"question_id" binding:"required"`
	UserAnswer     json.RawMessage        `json:"user_answer,omitempty"`
	AnswerPayload  json.RawMessage        `json:"answer_payload,omitempty"`
	ArtifactRefs   []MMSEArtifactRefInput `json:"artifact_refs,omitempty"`
	DeviceMetrics  json.RawMessage        `json:"device_metrics,omitempty"`
	ManualOverride bool                   `json:"manual_override,omitempty"`
	ManualScore    *int                   `json:"manual_score,omitempty"`
}

type CCRTArtifactPresignRequest struct {
	FileName     string `json:"file_name" binding:"required"`
	ContentType  string `json:"content_type" binding:"required"`
	Kind         string `json:"kind" binding:"required"`
	QuestionID   *uint  `json:"question_id,omitempty"`
	AssessmentID *uint  `json:"assessment_id,omitempty"`
}
