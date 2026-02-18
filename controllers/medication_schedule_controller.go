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

type medicationScheduleItem struct {
	MedicationID     uint       `json:"medication_id"`
	Name             string     `json:"name"`
	Dosage           string     `json:"dosage"`
	Frequency        string     `json:"frequency"`
	ReminderTime     string     `json:"reminder_time"`
	ReminderChannels string     `json:"reminder_channels"`
	AlertStyle       string     `json:"alert_style"`
	ScheduledDate    string     `json:"scheduled_date"`
	ScheduledTime    string     `json:"scheduled_time"`
	ScheduledAt      time.Time  `json:"scheduled_at"`
	Status           string     `json:"status"`
	TakenAt          *time.Time `json:"taken_at,omitempty"`
	Notes            string     `json:"notes"`
}

// GetMedicationSchedule 返回指定日期的用药计划
func GetMedicationSchedule(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	queryDate := strings.TrimSpace(ctx.Query("date"))
	date, err := parseDateOrToday(queryDate)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_DATE", "date format must be YYYY-MM-DD")
		return
	}

	var meds []models.Medication
	if err := global.Db.Where("user_id = ?", user.ID).Find(&meds).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch medications")
		return
	}

	var checkins []models.MedicationCheckin
	if err := global.Db.Where("user_id = ? AND scheduled_date = ?", user.ID, date).Find(&checkins).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch checkins")
		return
	}

	checkinMap := make(map[string]models.MedicationCheckin)
	for _, checkin := range checkins {
		key := buildCheckinKey(checkin.MedicationID, checkin.ScheduledTime)
		checkinMap[key] = checkin
	}

	items := make([]medicationScheduleItem, 0)
	for _, med := range meds {
		if !isMedicationActiveOnDate(med, date) {
			continue
		}
		times := parseReminderTimes(med.ReminderTime)
		for _, t := range times {
			scheduledAt, err := combineDateAndTime(date, t)
			if err != nil {
				continue
			}
			item := medicationScheduleItem{
				MedicationID:     med.ID,
				Name:             med.Name,
				Dosage:           med.Dosage,
				Frequency:        med.Frequency,
				ReminderTime:     med.ReminderTime,
				ReminderChannels: normalizeReminderChannels(med.ReminderChannels),
				AlertStyle:       normalizeAlertStyle(med.AlertStyle),
				ScheduledDate:    formatDate(date),
				ScheduledTime:    t,
				ScheduledAt:      scheduledAt,
				Status:           "pending",
				Notes:            med.Notes,
			}
			if checkin, ok := checkinMap[buildCheckinKey(med.ID, t)]; ok {
				item.Status = checkin.Status
				item.TakenAt = checkin.TakenAt
			}
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ScheduledAt.Before(items[j].ScheduledAt)
	})

	utils.RespondOK(ctx, gin.H{
		"date":  formatDate(date),
		"items": items,
	})
}

type medicationCheckinRequest struct {
	MedicationID  uint   `json:"medication_id" binding:"required"`
	ScheduledDate string `json:"scheduled_date" binding:"required"`
	ScheduledTime string `json:"scheduled_time" binding:"required"`
	Status        string `json:"status" binding:"required"` // taken/skipped
	Notes         string `json:"notes"`
}

// CreateMedicationCheckin 服药打卡
func CreateMedicationCheckin(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var req medicationCheckinRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "taken" && status != "skipped" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_STATUS", "status must be taken or skipped")
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

	var med models.Medication
	if err := global.Db.Where("id = ? AND user_id = ?", req.MedicationID, user.ID).First(&med).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "Medication not found")
		return
	}

	var checkin models.MedicationCheckin
	err = global.Db.Where(
		"user_id = ? AND medication_id = ? AND scheduled_date = ? AND scheduled_time = ?",
		user.ID,
		req.MedicationID,
		scheduledDate,
		scheduledTime,
	).First(&checkin).Error

	now := time.Now()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		checkin = models.MedicationCheckin{
			UserID:        user.ID,
			MedicationID:  req.MedicationID,
			ScheduledDate: scheduledDate,
			ScheduledTime: scheduledTime,
			Status:        status,
			Notes:         strings.TrimSpace(req.Notes),
		}
		if status == "taken" {
			checkin.TakenAt = &now
		}
		if err := global.Db.Create(&checkin).Error; err != nil {
			utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not create checkin")
			return
		}
		utils.RespondOK(ctx, gin.H{"checkin": checkin})
		return
	}

	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch checkin")
		return
	}

	checkin.Status = status
	checkin.Notes = strings.TrimSpace(req.Notes)
	if status == "taken" {
		checkin.TakenAt = &now
	} else {
		checkin.TakenAt = nil
	}

	if err := global.Db.Save(&checkin).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update checkin")
		return
	}

	utils.RespondOK(ctx, gin.H{"checkin": checkin})
}

type medicationStatsDay struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	Taken   int    `json:"taken"`
	Skipped int    `json:"skipped"`
	Pending int    `json:"pending"`
}

// GetMedicationStats 返回服药统计
func GetMedicationStats(ctx *gin.Context) {
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

	var meds []models.Medication
	if err := global.Db.Where("user_id = ?", user.ID).Find(&meds).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch medications")
		return
	}

	daily := make(map[string]*medicationStatsDay)
	order := make([]string, 0, days)
	total := 0

	for i := 0; i < days; i++ {
		date := startDate.AddDate(0, 0, i)
		dateStr := formatDate(date)
		order = append(order, dateStr)
		daily[dateStr] = &medicationStatsDay{
			Date: dateStr,
		}

		for _, med := range meds {
			if !isMedicationActiveOnDate(med, date) {
				continue
			}
			times := parseReminderTimes(med.ReminderTime)
			if len(times) == 0 {
				continue
			}
			daily[dateStr].Total += len(times)
			total += len(times)
		}
	}

	var checkins []models.MedicationCheckin
	if err := global.Db.Where(
		"user_id = ? AND scheduled_date >= ? AND scheduled_date <= ?",
		user.ID,
		startDate,
		endDate,
	).Find(&checkins).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch checkins")
		return
	}

	taken := 0
	skipped := 0
	for _, checkin := range checkins {
		dateStr := formatDate(checkin.ScheduledDate)
		day := daily[dateStr]
		if day == nil {
			continue
		}
		switch checkin.Status {
		case "taken":
			taken++
			day.Taken++
		case "skipped":
			skipped++
			day.Skipped++
		}
	}

	pending := total - taken - skipped
	if pending < 0 {
		pending = 0
	}

	dailyList := make([]medicationStatsDay, 0, len(order))
	for _, dateStr := range order {
		day := daily[dateStr]
		if day == nil {
			continue
		}
		day.Pending = day.Total - day.Taken - day.Skipped
		if day.Pending < 0 {
			day.Pending = 0
		}
		dailyList = append(dailyList, *day)
	}

	adherence := 0.0
	if total > 0 {
		adherence = float64(taken) / float64(total)
	}
	currentStreak, bestStreak := computeStreaks(dailyList)

	utils.RespondOK(ctx, gin.H{
		"range":      resolveRangeName(ctx.Query("range"), days),
		"start_date": formatDate(startDate),
		"end_date":   formatDate(endDate),
		"summary": gin.H{
			"total":          total,
			"taken":          taken,
			"skipped":        skipped,
			"pending":        pending,
			"adherence":      adherence,
			"streak_current": currentStreak,
			"streak_best":    bestStreak,
		},
		"daily": dailyList,
	})
}

func parseReminderTimes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	times := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, err := time.Parse("15:04", part); err != nil {
			continue
		}
		times = append(times, part)
	}
	sort.Strings(times)
	return times
}

func combineDateAndTime(date time.Time, timeStr string) (time.Time, error) {
	parsed, err := time.Parse("15:04", timeStr)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(date.Year(), date.Month(), date.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.Local), nil
}

func normalizeScheduleTime(raw string) (string, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return parsed.Format("15:04"), nil
}

func parseDateOrToday(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return dateOnly(time.Now()), nil
	}
	return parseDate(raw)
}

func parseDate(raw string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, err
	}
	return dateOnly(parsed), nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func isMedicationActiveOnDate(med models.Medication, date time.Time) bool {
	dateStr := formatDate(date)
	startStr := formatDate(med.StartDate)
	if startStr > dateStr {
		return false
	}
	if med.EndDate != nil && formatDate(*med.EndDate) < dateStr {
		return false
	}
	return true
}

func buildCheckinKey(medID uint, timeStr string) string {
	return strconv.FormatUint(uint64(medID), 10) + "|" + timeStr
}

func resolveStatsRange(rangeParam, daysParam string) int {
	if strings.TrimSpace(daysParam) != "" {
		if days, err := strconv.Atoi(strings.TrimSpace(daysParam)); err == nil && days > 0 {
			return days
		}
	}
	switch strings.ToLower(strings.TrimSpace(rangeParam)) {
	case "month":
		return 30
	case "week":
		return 7
	case "":
		return 7
	default:
		return 0
	}
}

func resolveRangeName(rangeParam string, days int) string {
	switch strings.ToLower(strings.TrimSpace(rangeParam)) {
	case "month":
		return "month"
	case "week":
		return "week"
	case "":
		if days == 30 {
			return "month"
		}
		if days == 7 {
			return "week"
		}
	}
	return strconv.Itoa(days) + "d"
}

func computeStreaks(daily []medicationStatsDay) (current int, best int) {
	streak := 0
	for _, day := range daily {
		if day.Total > 0 && day.Taken == day.Total {
			streak++
			if streak > best {
				best = streak
			}
		} else {
			streak = 0
		}
	}

	for i := len(daily) - 1; i >= 0; i-- {
		day := daily[i]
		if day.Total > 0 && day.Taken == day.Total {
			current++
		} else {
			break
		}
	}
	return current, best
}
