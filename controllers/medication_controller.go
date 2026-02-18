package controllers

import (
	"net/http"
	"strings"
	"time"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

// Helper: 获取当前登录用户ID
func getCurrentUser(ctx *gin.Context) (models.User, error) {
	username, _ := ctx.Get("username")
	var user models.User
	if err := global.Db.Where("username = ?", username).First(&user).Error; err != nil {
		return user, err
	}
	return user, nil
}

// CreateMedication 创建提醒
func CreateMedication(ctx *gin.Context) {
	var req MedicationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "start_date format must be YYYY-MM-DD")
		return
	}

	var endDate *time.Time
	if req.EndDate != "" {
		parsedEnd, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date format must be YYYY-MM-DD")
			return
		}
		if parsedEnd.Before(startDate) {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date must be after start_date")
			return
		}
		endDate = &parsedEnd
	}

	// 校验 ReminderTime 格式
	if req.ReminderTime != "" && !isValidReminderTime(req.ReminderTime) {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_TIME", "reminder_time format must be HH:MM (comma separated for multiple)")
		return
	}

	med := models.Medication{
		UserID:           user.ID,
		Name:             req.Name,
		Dosage:           req.Dosage,
		Frequency:        req.Frequency,
		ReminderTime:     req.ReminderTime,
		ReminderChannels: normalizeReminderChannels(req.ReminderChannels),
		AlertStyle:       normalizeAlertStyle(req.AlertStyle),
		StartDate:        startDate,
		EndDate:          endDate,
		Notes:            req.Notes,
	}

	if err := global.Db.Create(&med).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not create medication")
		return
	}

	utils.RespondOK(ctx, gin.H{"id": med.ID})
}

// GetMedications 获取当前用户的所有提醒
func GetMedications(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var meds []models.Medication
	if err := global.Db.Where("user_id = ?", user.ID).Order("id desc").Find(&meds).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch medications")
		return
	}

	utils.RespondOK(ctx, gin.H{"data": meds})
}

// UpdateMedication 更新提醒
func UpdateMedication(ctx *gin.Context) {
	id := ctx.Param("id")
	var req MedicationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var med models.Medication
	if err := global.Db.Where("id = ? AND user_id = ?", id, user.ID).First(&med).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Medication not found")
		return
	}

	// Update fields
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "start_date format must be YYYY-MM-DD")
		return
	}
	med.StartDate = startDate

	if req.EndDate != "" {
		parsedEnd, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date format must be YYYY-MM-DD")
			return
		}
		if parsedEnd.Before(startDate) {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date must be after start_date")
			return
		}
		med.EndDate = &parsedEnd
	} else {
		med.EndDate = nil
	}

	// 校验 ReminderTime 格式
	if req.ReminderTime != "" && !isValidReminderTime(req.ReminderTime) {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_TIME", "reminder_time format must be HH:MM (comma separated for multiple)")
		return
	}

	med.Name = req.Name
	med.Dosage = req.Dosage
	med.Frequency = req.Frequency
	med.ReminderTime = req.ReminderTime
	med.ReminderChannels = normalizeReminderChannels(req.ReminderChannels)
	med.AlertStyle = normalizeAlertStyle(req.AlertStyle)
	med.Notes = req.Notes

	if err := global.Db.Save(&med).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update medication")
		return
	}

	utils.RespondOK(ctx, gin.H{"success": true})
}

// DeleteMedication 删除提醒
func DeleteMedication(ctx *gin.Context) {
	id := ctx.Param("id")
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	result := global.Db.Where("id = ? AND user_id = ?", id, user.ID).Delete(&models.Medication{})
	if result.Error != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not delete medication")
		return
	}
	if result.RowsAffected == 0 {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Medication not found")
		return
	}

	utils.RespondOK(ctx, gin.H{"success": true})
}

// GetMedication 获取单条提醒
func GetMedication(ctx *gin.Context) {
	id := ctx.Param("id")
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var med models.Medication
	if err := global.Db.Where("id = ? AND user_id = ?", id, user.ID).First(&med).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Medication not found")
		return
	}

	utils.RespondOK(ctx, gin.H{"medication": med})
}

// isValidReminderTime 校验提醒时间格式 (HH:MM, 支持逗号分隔多个时间)
func isValidReminderTime(timeStr string) bool {
	times := strings.Split(timeStr, ",")
	for _, t := range times {
		t = strings.TrimSpace(t)
		if _, err := time.Parse("15:04", t); err != nil {
			return false
		}
	}
	return true
}

func normalizeReminderChannels(raw string) string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool)
	var normalized []string
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		normalized = append(normalized, p)
	}
	if len(normalized) == 0 {
		return "app"
	}
	return strings.Join(normalized, ",")
}

func normalizeAlertStyle(raw string) string {
	style := strings.ToLower(strings.TrimSpace(raw))
	switch style {
	case "strong", "normal":
		return style
	default:
		return "strong"
	}
}
