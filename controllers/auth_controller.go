package controllers

import (
	"errors"
	"net/http"
	"strings"
	"time"

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

	respondWithTokens(ctx, user)
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

	respondWithTokens(ctx, user)
}

// RefreshToken 使用 refresh_token 换取新的 access_token（并轮换 refresh_token）
func RefreshToken(ctx *gin.Context) {
	var req RefreshTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}
	raw := strings.TrimSpace(req.RefreshToken)
	if raw == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "refresh_token is required")
		return
	}

	hash := utils.HashToken(raw)
	var token models.RefreshToken
	if err := global.Db.Where("token_hash = ?", hash).First(&token).Error; err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid refresh token")
		return
	}
	if token.RevokedAt != nil || time.Now().After(token.ExpiresAt) {
		utils.RespondError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "Refresh token expired")
		return
	}

	var user models.User
	if err := global.Db.First(&user, token.UserID).Error; err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	accessToken, accessExp, err := utils.GenerateAccessToken(user.Username)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate token")
		return
	}
	refreshToken, refreshExp, err := utils.GenerateRefreshToken()
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate refresh token")
		return
	}

	now := time.Now()
	newRecord := models.RefreshToken{
		UserID:    user.ID,
		TokenHash: utils.HashToken(refreshToken),
		ExpiresAt: refreshExp,
	}

	tx := global.Db.Begin()
	if err := tx.Model(&token).Update("revoked_at", &now).Error; err != nil {
		tx.Rollback()
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not rotate token")
		return
	}
	if err := tx.Create(&newRecord).Error; err != nil {
		tx.Rollback()
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not rotate token")
		return
	}
	if err := tx.Commit().Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not rotate token")
		return
	}

	expiresIn := int64(accessExp.Sub(now).Seconds())
	refreshExpiresIn := int64(refreshExp.Sub(now).Seconds())
	utils.RespondOK(ctx, gin.H{
		"access_token":        accessToken,
		"refresh_token":       refreshToken,
		"token_type":          "Bearer",
		"expires_in":          expiresIn,
		"refresh_expires_in":  refreshExpiresIn,
		"token":               utils.FormatBearerToken(accessToken),
	})
}

func respondWithTokens(ctx *gin.Context, user models.User) {
	accessToken, accessExp, err := utils.GenerateAccessToken(user.Username)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate token")
		return
	}
	refreshToken, refreshExp, err := utils.GenerateRefreshToken()
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate refresh token")
		return
	}

	record := models.RefreshToken{
		UserID:    user.ID,
		TokenHash: utils.HashToken(refreshToken),
		ExpiresAt: refreshExp,
	}
	if err := global.Db.Create(&record).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not persist refresh token")
		return
	}

	now := time.Now()
	expiresIn := int64(accessExp.Sub(now).Seconds())
	refreshExpiresIn := int64(refreshExp.Sub(now).Seconds())
	utils.RespondOK(ctx, gin.H{
		"access_token":       accessToken,
		"refresh_token":      refreshToken,
		"token_type":         "Bearer",
		"expires_in":         expiresIn,
		"refresh_expires_in": refreshExpiresIn,
		"token":              utils.FormatBearerToken(accessToken),
	})
}
