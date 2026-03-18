package controllers

import (
	"net/http"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

// CreateReminder creates a generic reminder record.
func CreateReminder(ctx *gin.Context) {
	var req ReminderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	if !isValidReminderTime(req.ReminderTime) {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_TIME", "reminder_time format must be HH:MM (comma separated for multiple)")
		return
	}

	reminderDate, err := parseReminderOptionalDate(req.ReminderDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "reminder_date format must be YYYY-MM-DD")
		return
	}
	startDate, err := parseReminderRequiredOrToday(req.StartDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "start_date format must be YYYY-MM-DD")
		return
	}
	endDate, err := parseReminderOptionalDate(req.EndDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date format must be YYYY-MM-DD")
		return
	}

	repeatRule := normalizeReminderRepeatRule(req.RepeatRule, reminderDate != nil)
	if repeatRule == reminderRepeatOnce {
		if reminderDate == nil {
			reminderDate = &startDate
		}
		startDate = *reminderDate
	}
	if endDate != nil && endDate.Before(startDate) {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date must be after start_date")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = reminderDefaultTitle(req.Type)
	}

	reminder := models.Reminder{
		UserID:           user.ID,
		Type:             normalizeReminderType(req.Type),
		Title:            title,
		Description:      strings.TrimSpace(req.Description),
		ReminderTime:     req.ReminderTime,
		RepeatRule:       repeatRule,
		ReminderDate:     reminderDate,
		StartDate:        startDate,
		EndDate:          endDate,
		ReminderChannels: normalizeReminderChannels(req.ReminderChannels),
		AlertStyle:       normalizeAlertStyle(req.AlertStyle),
		Notes:            strings.TrimSpace(req.Notes),
	}

	if err := global.Db.Create(&reminder).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not create reminder")
		return
	}

	utils.RespondOK(ctx, gin.H{"id": reminder.ID})
}

// GetReminders lists generic reminders for the current user.
func GetReminders(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var reminders []models.Reminder
	if err := global.Db.Where("user_id = ?", user.ID).Order("id desc").Find(&reminders).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch reminders")
		return
	}

	utils.RespondOK(ctx, gin.H{"data": reminders})
}

// GetReminder returns a single generic reminder.
func GetReminder(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var reminder models.Reminder
	if err := global.Db.Where("id = ? AND user_id = ?", ctx.Param("id"), user.ID).First(&reminder).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Reminder not found")
		return
	}

	utils.RespondOK(ctx, gin.H{"reminder": reminder})
}

// UpdateReminder updates a generic reminder.
func UpdateReminder(ctx *gin.Context) {
	var req ReminderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var reminder models.Reminder
	if err := global.Db.Where("id = ? AND user_id = ?", ctx.Param("id"), user.ID).First(&reminder).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Reminder not found")
		return
	}

	if !isValidReminderTime(req.ReminderTime) {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_TIME", "reminder_time format must be HH:MM (comma separated for multiple)")
		return
	}

	reminderDate, err := parseReminderOptionalDate(req.ReminderDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "reminder_date format must be YYYY-MM-DD")
		return
	}
	startDate, err := parseReminderRequiredOrToday(req.StartDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "start_date format must be YYYY-MM-DD")
		return
	}
	endDate, err := parseReminderOptionalDate(req.EndDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date format must be YYYY-MM-DD")
		return
	}

	repeatRule := normalizeReminderRepeatRule(req.RepeatRule, reminderDate != nil)
	if repeatRule == reminderRepeatOnce {
		if reminderDate == nil {
			reminderDate = &startDate
		}
		startDate = *reminderDate
	}
	if endDate != nil && endDate.Before(startDate) {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "end_date must be after start_date")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = reminder.Title
	}

	reminder.Type = normalizeReminderType(req.Type)
	reminder.Title = title
	reminder.Description = strings.TrimSpace(req.Description)
	reminder.ReminderTime = req.ReminderTime
	reminder.RepeatRule = repeatRule
	reminder.ReminderDate = reminderDate
	reminder.StartDate = startDate
	reminder.EndDate = endDate
	reminder.ReminderChannels = normalizeReminderChannels(req.ReminderChannels)
	reminder.AlertStyle = normalizeAlertStyle(req.AlertStyle)
	reminder.Notes = strings.TrimSpace(req.Notes)

	if err := global.Db.Save(&reminder).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update reminder")
		return
	}

	utils.RespondOK(ctx, gin.H{"success": true})
}

// DeleteReminder removes a generic reminder.
func DeleteReminder(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	result := global.Db.Where("id = ? AND user_id = ?", ctx.Param("id"), user.ID).Delete(&models.Reminder{})
	if result.Error != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not delete reminder")
		return
	}
	if result.RowsAffected == 0 {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Reminder not found")
		return
	}

	utils.RespondOK(ctx, gin.H{"success": true})
}
