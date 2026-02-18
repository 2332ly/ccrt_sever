package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ccrt_sever/config"
	"ccrt_sever/global"
	"ccrt_sever/models"
)

var (
	reminderOnce    sync.Once
	reminderRunning int32
)

// StartMedicationReminderScheduler starts the background reminder dispatcher.
func StartMedicationReminderScheduler() {
	if config.AppConfig == nil || !config.AppConfig.Notify.Enabled {
		return
	}
	reminderOnce.Do(func() {
		interval := time.Duration(config.AppConfig.Notify.PollIntervalSec) * time.Second
		if interval <= 0 {
			interval = time.Minute
		}

		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				runMedicationReminderTick()
				<-ticker.C
			}
		}()
	})
}

func runMedicationReminderTick() {
	if !atomic.CompareAndSwapInt32(&reminderRunning, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&reminderRunning, 0)

	cfg := config.AppConfig.Notify
	now := time.Now()
	lead := time.Duration(cfg.LeadMinutes) * time.Minute
	grace := time.Duration(cfg.GraceMinutes) * time.Minute
	if lead < 0 {
		lead = 0
	}
	if grace < 0 {
		grace = 0
	}

	windowStart := now.Add(-grace)
	windowEnd := now.Add(lead)
	startDate := dateOnly(windowStart)
	endDate := dateOnly(windowEnd)

	meds, err := loadActiveMedications(startDate, endDate)
	if err != nil {
		log.Printf("reminder: load meds failed: %v", err)
		return
	}
	if len(meds) == 0 {
		return
	}

	userMap, err := loadUsersForMedications(meds)
	if err != nil {
		log.Printf("reminder: load users failed: %v", err)
		return
	}

	checkinMap, err := loadCheckinMap(startDate, endDate)
	if err != nil {
		log.Printf("reminder: load checkins failed: %v", err)
		return
	}

	dispatchMap, err := loadDispatchMap(startDate, endDate)
	if err != nil {
		log.Printf("reminder: load dispatches failed: %v", err)
		return
	}

	for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
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
				if scheduledAt.Before(windowStart) || scheduledAt.After(windowEnd) {
					continue
				}

				if hasCheckin(checkinMap, med.UserID, med.ID, date, t) {
					continue
				}

				channels := parseChannels(med.ReminderChannels)
				if len(channels) == 0 {
					continue
				}

				user := userMap[med.UserID]
				for _, channel := range channels {
					if channel == "app" {
						continue
					}
					if !isChannelEnabled(channel) {
						continue
					}
					key := buildDispatchKey(med.UserID, med.ID, date, t, channel)
					if dispatch, ok := dispatchMap[key]; ok && dispatch.Status == "sent" {
						continue
					}
					err := dispatchReminder(channel, user, med, date, t, scheduledAt)
					if err != nil {
						saveDispatchFailure(dispatchMap[key], user.ID, med.ID, date, t, channel, err)
						continue
					}
					saveDispatchSuccess(dispatchMap[key], user.ID, med.ID, date, t, channel)
				}
			}
		}
	}
}

func loadActiveMedications(startDate, endDate time.Time) ([]models.Medication, error) {
	var meds []models.Medication
	err := global.Db.
		Where("start_date <= ? AND (end_date IS NULL OR end_date >= ?)", endDate, startDate).
		Find(&meds).Error
	return meds, err
}

func loadUsersForMedications(meds []models.Medication) (map[uint]models.User, error) {
	ids := make([]uint, 0)
	seen := make(map[uint]bool)
	for _, med := range meds {
		if seen[med.UserID] {
			continue
		}
		seen[med.UserID] = true
		ids = append(ids, med.UserID)
	}
	if len(ids) == 0 {
		return map[uint]models.User{}, nil
	}

	var users []models.User
	if err := global.Db.Where("id IN (?)", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]models.User)
	for _, user := range users {
		result[user.ID] = user
	}
	return result, nil
}

func loadCheckinMap(startDate, endDate time.Time) (map[string]models.MedicationCheckin, error) {
	var checkins []models.MedicationCheckin
	if err := global.Db.
		Where("scheduled_date >= ? AND scheduled_date <= ?", startDate, endDate).
		Find(&checkins).Error; err != nil {
		return nil, err
	}
	result := make(map[string]models.MedicationCheckin)
	for _, checkin := range checkins {
		key := buildCheckinKey(checkin.UserID, checkin.MedicationID, checkin.ScheduledDate, checkin.ScheduledTime)
		result[key] = checkin
	}
	return result, nil
}

func loadDispatchMap(startDate, endDate time.Time) (map[string]models.MedicationDispatch, error) {
	var dispatches []models.MedicationDispatch
	if err := global.Db.
		Where("scheduled_date >= ? AND scheduled_date <= ?", startDate, endDate).
		Find(&dispatches).Error; err != nil {
		return nil, err
	}
	result := make(map[string]models.MedicationDispatch)
	for _, dispatch := range dispatches {
		key := buildDispatchKey(dispatch.UserID, dispatch.MedicationID, dispatch.ScheduledDate, dispatch.ScheduledTime, dispatch.Channel)
		result[key] = dispatch
	}
	return result, nil
}

func hasCheckin(checkinMap map[string]models.MedicationCheckin, userID, medID uint, date time.Time, timeStr string) bool {
	key := buildCheckinKey(userID, medID, date, timeStr)
	checkin, ok := checkinMap[key]
	if !ok {
		return false
	}
	return checkin.Status == "taken" || checkin.Status == "skipped"
}

func dispatchReminder(channel string, user models.User, med models.Medication, date time.Time, timeStr string, scheduledAt time.Time) error {
	if user.Phone == "" {
		return errors.New("user phone is empty")
	}
	message := buildReminderMessage(med, timeStr)
	payload := map[string]any{
		"channel":        channel,
		"user_id":        user.ID,
		"username":       user.Username,
		"phone":          user.Phone,
		"medication_id":  med.ID,
		"medication":     med.Name,
		"dosage":         med.Dosage,
		"frequency":      med.Frequency,
		"notes":          med.Notes,
		"alert_style":    normalizeAlertStyle(med.AlertStyle),
		"scheduled_date": formatDate(date),
		"scheduled_time": timeStr,
		"scheduled_at":   scheduledAt.Format(time.RFC3339),
		"message":        message,
	}
	endpoint := webhookEndpoint(channel)
	if endpoint == "" {
		return errors.New("webhook endpoint not configured")
	}
	return postJSON(endpoint, payload)
}

func saveDispatchSuccess(existing models.MedicationDispatch, userID, medID uint, date time.Time, timeStr, channel string) {
	now := time.Now()
	if existing.ID != 0 {
		global.Db.Model(&existing).Updates(map[string]any{
			"status":        "sent",
			"sent_at":       &now,
			"error_message": "",
		})
		return
	}
	record := models.MedicationDispatch{
		UserID:        userID,
		MedicationID:  medID,
		ScheduledDate: date,
		ScheduledTime: timeStr,
		Channel:       channel,
		Status:        "sent",
		SentAt:        &now,
	}
	_ = global.Db.Create(&record).Error
}

func saveDispatchFailure(existing models.MedicationDispatch, userID, medID uint, date time.Time, timeStr, channel string, err error) {
	msg := err.Error()
	if existing.ID != 0 {
		global.Db.Model(&existing).Updates(map[string]any{
			"status":        "failed",
			"error_message": msg,
		})
		return
	}
	record := models.MedicationDispatch{
		UserID:        userID,
		MedicationID:  medID,
		ScheduledDate: date,
		ScheduledTime: timeStr,
		Channel:       channel,
		Status:        "failed",
		ErrorMessage:  msg,
	}
	_ = global.Db.Create(&record).Error
}

func postJSON(url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("webhook status %d", resp.StatusCode)
}

func buildReminderMessage(med models.Medication, timeStr string) string {
	prefix := "提醒"
	if normalizeAlertStyle(med.AlertStyle) == "strong" {
		prefix = "强提醒"
	}
	dosage := strings.TrimSpace(med.Dosage)
	if dosage != "" {
		dosage = " " + dosage
	}
	return fmt.Sprintf("%s：%s 服用%s%s。", prefix, timeStr, med.Name, dosage)
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

func parseChannels(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"app"}
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, part)
	}
	return result
}

func isChannelEnabled(channel string) bool {
	cfg := config.AppConfig.Notify
	switch channel {
	case "sms":
		return cfg.Channels.SMS
	case "voice":
		return cfg.Channels.Voice
	case "call":
		return cfg.Channels.Call
	default:
		return false
	}
}

func webhookEndpoint(channel string) string {
	cfg := config.AppConfig.Notify
	switch channel {
	case "sms":
		return strings.TrimSpace(cfg.Webhook.SMS)
	case "voice":
		return strings.TrimSpace(cfg.Webhook.Voice)
	case "call":
		return strings.TrimSpace(cfg.Webhook.Call)
	default:
		return ""
	}
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

func buildCheckinKey(userID, medID uint, date time.Time, timeStr string) string {
	return fmt.Sprintf("%d|%d|%s|%s", userID, medID, formatDate(date), timeStr)
}

func buildDispatchKey(userID, medID uint, date time.Time, timeStr, channel string) string {
	return fmt.Sprintf("%d|%d|%s|%s|%s", userID, medID, formatDate(date), timeStr, channel)
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
