package controllers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type reminderScheduleItem struct {
	ReminderID       uint       `json:"reminder_id"`
	Type             string     `json:"type"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	ReminderTime     string     `json:"reminder_time"`
	ReminderChannels string     `json:"reminder_channels"`
	AlertStyle       string     `json:"alert_style"`
	ScheduledDate    string     `json:"scheduled_date"`
	ScheduledTime    string     `json:"scheduled_time"`
	ScheduledAt      time.Time  `json:"scheduled_at"`
	Status           string     `json:"status"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	Notes            string     `json:"notes"`
}

type reminderCheckinRequest struct {
	ReminderID    uint   `json:"reminder_id" binding:"required"`
	ScheduledDate string `json:"scheduled_date" binding:"required"`
	ScheduledTime string `json:"scheduled_time" binding:"required"`
	Status        string `json:"status" binding:"required"`
	Notes         string `json:"notes"`
}

type reminderStatsDay struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Skipped int    `json:"skipped"`
	Pending int    `json:"pending"`
}

// GetReminderSchedule returns the generic reminder schedule for a single day.
func GetReminderSchedule(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	date, err := parseDateOrToday(ctx.Query("date"))
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "date format must be YYYY-MM-DD")
		return
	}

	var reminders []models.Reminder
	if err := global.Db.Where("user_id = ?", user.ID).Find(&reminders).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch reminders")
		return
	}

	var checkins []models.ReminderCheckin
	if err := global.Db.Where("user_id = ? AND scheduled_date = ?", user.ID, date).Find(&checkins).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch reminder checkins")
		return
	}

	checkinMap := make(map[string]models.ReminderCheckin)
	for _, checkin := range checkins {
		checkinMap[buildReminderCheckinKey(checkin.ReminderID, checkin.ScheduledTime)] = checkin
	}

	items := make([]reminderScheduleItem, 0)
	for _, reminder := range reminders {
		if !reminderIsActiveOnDate(reminder, date) {
			continue
		}

		for _, timeText := range parseReminderTimesSorted(reminder.ReminderTime) {
			scheduledAt, err := combineDateAndTime(date, timeText)
			if err != nil {
				continue
			}

			item := reminderScheduleItem{
				ReminderID:       reminder.ID,
				Type:             reminder.Type,
				Title:            reminder.Title,
				Description:      reminder.Description,
				ReminderTime:     reminder.ReminderTime,
				ReminderChannels: normalizeReminderChannels(reminder.ReminderChannels),
				AlertStyle:       normalizeAlertStyle(reminder.AlertStyle),
				ScheduledDate:    reminderFormatDate(date),
				ScheduledTime:    timeText,
				ScheduledAt:      scheduledAt,
				Status:           "pending",
				Notes:            reminder.Notes,
			}
			if checkin, ok := checkinMap[buildReminderCheckinKey(reminder.ID, timeText)]; ok {
				item.Status = checkin.Status
				item.CompletedAt = checkin.CompletedAt
			}
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ScheduledAt.Before(items[j].ScheduledAt)
	})

	utils.RespondOK(ctx, gin.H{
		"date":  reminderFormatDate(date),
		"items": items,
	})
}

// CreateReminderCheckin marks a generic reminder item as done or skipped.
func CreateReminderCheckin(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var req reminderCheckinRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != reminderStatusDone && status != reminderStatusSkipped {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_STATUS", "status must be done or skipped")
		return
	}

	scheduledDate, err := parseDate(req.ScheduledDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "scheduled_date format must be YYYY-MM-DD")
		return
	}
	scheduledTime, err := normalizeScheduleTime(req.ScheduledTime)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_TIME", "scheduled_time format must be HH:MM")
		return
	}

	var reminder models.Reminder
	if err := global.Db.Where("id = ? AND user_id = ?", req.ReminderID, user.ID).First(&reminder).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Reminder not found")
		return
	}

	var checkin models.ReminderCheckin
	err = global.Db.Where(
		"user_id = ? AND reminder_id = ? AND scheduled_date = ? AND scheduled_time = ?",
		user.ID,
		req.ReminderID,
		scheduledDate,
		scheduledTime,
	).First(&checkin).Error

	now := time.Now()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		checkin = models.ReminderCheckin{
			UserID:        user.ID,
			ReminderID:    req.ReminderID,
			ScheduledDate: scheduledDate,
			ScheduledTime: scheduledTime,
			Status:        status,
			Notes:         strings.TrimSpace(req.Notes),
		}
		if status == reminderStatusDone {
			checkin.CompletedAt = &now
		}
		if err := global.Db.Create(&checkin).Error; err != nil {
			utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not create reminder checkin")
			return
		}
		utils.RespondOK(ctx, gin.H{"checkin": checkin})
		return
	}
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch reminder checkin")
		return
	}

	checkin.Status = status
	checkin.Notes = strings.TrimSpace(req.Notes)
	if status == reminderStatusDone {
		checkin.CompletedAt = &now
	} else {
		checkin.CompletedAt = nil
	}

	if err := global.Db.Save(&checkin).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update reminder checkin")
		return
	}

	utils.RespondOK(ctx, gin.H{"checkin": checkin})
}

// GetReminderStats returns aggregated generic reminder adherence for a date range.
func GetReminderStats(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	days := resolveStatsRange(ctx.Query("range"), ctx.Query("days"))
	if days <= 0 {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_RANGE", "range must be week/month or days must be > 0")
		return
	}

	endDate := dateOnly(time.Now())
	startDate := endDate.AddDate(0, 0, -(days - 1))

	var reminders []models.Reminder
	if err := global.Db.Where("user_id = ?", user.ID).Find(&reminders).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch reminders")
		return
	}

	daily := make(map[string]*reminderStatsDay)
	order := make([]string, 0, days)
	total := 0

	for i := 0; i < days; i++ {
		date := startDate.AddDate(0, 0, i)
		dateKey := reminderFormatDate(date)
		order = append(order, dateKey)
		daily[dateKey] = &reminderStatsDay{Date: dateKey}

		for _, reminder := range reminders {
			if !reminderIsActiveOnDate(reminder, date) {
				continue
			}
			times := parseReminderTimesSorted(reminder.ReminderTime)
			if len(times) == 0 {
				continue
			}
			daily[dateKey].Total += len(times)
			total += len(times)
		}
	}

	var checkins []models.ReminderCheckin
	if err := global.Db.Where(
		"user_id = ? AND scheduled_date >= ? AND scheduled_date <= ?",
		user.ID,
		startDate,
		endDate,
	).Find(&checkins).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch reminder checkins")
		return
	}

	done := 0
	skipped := 0
	for _, checkin := range checkins {
		dateKey := reminderFormatDate(checkin.ScheduledDate)
		day := daily[dateKey]
		if day == nil {
			continue
		}
		switch checkin.Status {
		case reminderStatusDone:
			done++
			day.Done++
		case reminderStatusSkipped:
			skipped++
			day.Skipped++
		}
	}

	pending := total - done - skipped
	if pending < 0 {
		pending = 0
	}

	dailyList := make([]reminderStatsDay, 0, len(order))
	for _, dateKey := range order {
		day := daily[dateKey]
		if day == nil {
			continue
		}
		day.Pending = day.Total - day.Done - day.Skipped
		if day.Pending < 0 {
			day.Pending = 0
		}
		dailyList = append(dailyList, *day)
	}

	compliance := 0.0
	if total > 0 {
		compliance = float64(done) / float64(total)
	}

	utils.RespondOK(ctx, gin.H{
		"range":      resolveRangeName(ctx.Query("range"), days),
		"start_date": reminderFormatDate(startDate),
		"end_date":   reminderFormatDate(endDate),
		"summary": gin.H{
			"total":      total,
			"done":       done,
			"skipped":    skipped,
			"pending":    pending,
			"compliance": compliance,
		},
		"daily": dailyList,
	})
}

func buildReminderCheckinKey(reminderID uint, timeText string) string {
	return strconv.FormatUint(uint64(reminderID), 10) + "|" + timeText
}
