package controllers

import (
	"net/http"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

type emergencyContactRequest struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// Me 示例受保护接口：返回当前 token 里的 username
func Me(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}
	utils.RespondOK(ctx, gin.H{
		"username":                user.Username,
		"emergency_contact_name":  user.EmergencyContactName,
		"emergency_contact_phone": user.EmergencyContactPhone,
		"status":                  http.StatusOK,
	})
}

// UpdateEmergencyContact updates the emergency contact for the current user.
func UpdateEmergencyContact(ctx *gin.Context) {
	var req emergencyContactRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	name := strings.TrimSpace(req.Name)
	phone := strings.TrimSpace(req.Phone)
	user.EmergencyContactName = name
	user.EmergencyContactPhone = phone

	if err := global.Db.Save(&user).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update emergency contact")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"emergency_contact_name":  name,
		"emergency_contact_phone": phone,
	})
}
