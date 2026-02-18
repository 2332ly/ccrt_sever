package controllers

import (
	"net/http"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

type sosRequest struct {
	Note string `json:"note"`
}

// CreateSOSEvent records an emergency call event for the current user.
func CreateSOSEvent(ctx *gin.Context) {
	var req sosRequest
	_ = ctx.ShouldBindJSON(&req)

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	contactName := strings.TrimSpace(user.EmergencyContactName)
	contactPhone := strings.TrimSpace(user.EmergencyContactPhone)
	if contactPhone == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "EMERGENCY_CONTACT_MISSING", "Emergency contact phone is empty")
		return
	}

	note := trimRunes(strings.TrimSpace(req.Note), 200)
	event := models.SosEvent{
		UserID:       user.ID,
		ContactName:  contactName,
		ContactPhone: contactPhone,
		Note:         note,
	}

	if err := global.Db.Create(&event).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not create sos event")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"id":            event.ID,
		"contact_name":  contactName,
		"contact_phone": contactPhone,
		"created_at":    event.CreatedAt,
	})
}

func trimRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
