package controllers

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
