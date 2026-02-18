package controllers

import "encoding/json"

// RegisterRequest 注册请求 DTO：必须手机号验证码 + 密码
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Phone    string `json:"phone" binding:"required,min=6,max=32"`
	Password string `json:"password" binding:"required,min=6,max=128"`
	SMSCode  string `json:"sms_code" binding:"required,min=4,max=8"`
}

// LoginRequest 登录请求 DTO：密码登录或短信登录二选一
// - 若提供 password：按 username/phone + password 登录
// - 若提供 sms_code：按 phone + sms_code 登录
type LoginRequest struct {
	Username string `json:"username" binding:"omitempty,min=3,max=64"`
	Phone    string `json:"phone" binding:"omitempty,min=6,max=32"`
	Password string `json:"password" binding:"omitempty,min=6,max=128"`
	SMSCode  string `json:"sms_code" binding:"omitempty,min=4,max=8"`
}

// SendSMSRequest 发送短信验证码
type SendSMSRequest struct {
	Phone string `json:"phone" binding:"required,min=6,max=32"`
}

// VerifySMSRequest 校验短信验证码
type VerifySMSRequest struct {
	Phone   string `json:"phone" binding:"required,min=6,max=32"`
	SMSCode string `json:"sms_code" binding:"required,min=4,max=8"`
}

// RefreshTokenRequest 刷新 access_token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// MedicationRequest 吃药提醒请求 DTO
type MedicationRequest struct {
	Name             string `json:"name" binding:"required"`
	Dosage           string `json:"dosage"`
	Frequency        string `json:"frequency"`
	ReminderTime     string `json:"reminder_time"`                 // e.g. "08:00,20:00"
	ReminderChannels string `json:"reminder_channels"`             // e.g. "app,sms,voice"
	AlertStyle       string `json:"alert_style"`                   // strong/normal
	StartDate        string `json:"start_date" binding:"required"` // Format: 2006-01-02
	EndDate          string `json:"end_date"`                      // Format: 2006-01-02
	Notes            string `json:"notes"`
}

// MMSESubmitRequest MMSE 提交作答
type MMSESubmitRequest struct {
	ScaleVersionID uint              `json:"scale_version_id"`
	Answers        []MMSEAnswerInput `json:"answers" binding:"required"`
}

// MMSEAnswerInput 单题作答
type MMSEAnswerInput struct {
	QuestionID  uint            `json:"question_id" binding:"required"`
	UserAnswer  json.RawMessage `json:"user_answer"`
	ManualScore *int            `json:"manual_score,omitempty"`
}
