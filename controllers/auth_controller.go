package controllers

import (
	"errors"
	"net/http"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册新用户：必须手机号验证码 + 密码
func Register(ctx *gin.Context) {
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	username := strings.TrimSpace(req.Username)
	phone := strings.TrimSpace(req.Phone)
	if username == "" || phone == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	// 先校验短信验证码
	if err := utils.CheckSMSVerifyCode(phone, req.SMSCode); err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "SMS_VERIFY_FAILED", err.Error())
		return
	}

	hashedPwd, err := utils.HashPassword(req.Password)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal error")
		return
	}

	user := models.User{Username: username, Phone: phone, Password: hashedPwd}

	if err = global.Db.Create(&user).Error; err != nil {
		errMsg := strings.ToLower(err.Error())
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(errMsg, "duplicate") || strings.Contains(errMsg, "unique") {
			utils.RespondError(ctx, http.StatusConflict, "USER_EXISTS", "Username or phone already exists")
			return
		}
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create user")
		return
	}

	token, err := utils.GenerateJWT(user.Username)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate token")
		return
	}
	utils.RespondOK(ctx, gin.H{"token": token})
}

// Login 登录：密码/短信二选一
func Login(ctx *gin.Context) {
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	username := strings.TrimSpace(req.Username)
	phone := strings.TrimSpace(req.Phone)
	password := req.Password
	smsCode := strings.TrimSpace(req.SMSCode)

	// 规则：密码登录 或 短信登录（二选一）
	passwordLogin := strings.TrimSpace(password) != ""
	smsLogin := smsCode != ""
	if passwordLogin == smsLogin {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Choose exactly one of password or sms_code")
		return
	}

	var user models.User
	if passwordLogin {
		// 支持 username 或 phone + password
		if username == "" && phone == "" {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "username or phone is required")
			return
		}
		q := global.Db
		if phone != "" {
			q = q.Where("phone = ?", phone)
		} else {
			q = q.Where("username = ?", username)
		}
		if err := q.First(&user).Error; err != nil {
			utils.RespondError(ctx, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
			return
		}
		if !utils.CheckPassword(password, user.Password) {
			utils.RespondError(ctx, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
			return
		}
	} else {
		// 短信登录：必须 phone + sms_code
		if phone == "" {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "phone is required")
			return
		}
		if err := global.Db.Where("phone = ?", phone).First(&user).Error; err != nil {
			utils.RespondError(ctx, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
			return
		}
		if err := utils.CheckSMSVerifyCode(phone, smsCode); err != nil {
			utils.RespondError(ctx, http.StatusUnauthorized, "SMS_VERIFY_FAILED", err.Error())
			return
		}
	}

	token, err := utils.GenerateJWT(user.Username)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate token")
		return
	}
	utils.RespondOK(ctx, gin.H{"token": token})
}
