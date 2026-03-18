package services

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ccrt_sever/config"
	"ccrt_sever/global"
	"ccrt_sever/models"
)

var (
	genericReminderOnce    sync.Once
	genericReminderRunning int32
)

const (
	genericReminderRepeatOnce = "once"
	genericReminderStatusDone = "done"
	genericReminderStatusSkip = "skipped"
)

// StartGenericReminderScheduler starts dispatching non-medication reminders.
func StartGenericReminderScheduler() {
	if config.AppConfig == nil || !config.AppConfig.Notify.Enabled {
		return
	}
	genericReminderOnce.Do(func() {
		interval := time.Duration(config.AppConfig.Notify.PollIntervalSec) * time.Second
		if interval <= 0 {
			interval = time.Minute
		}

		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				runGenericReminderTick()
				<-ticker.C
			}
		}()
	})
}

func runGenericReminderTick() {
	if !atomic.CompareAndSwapInt32(&genericReminderRunning, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&genericReminderRunning, 0)

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

	reminders, err := loadActiveReminders(startDate, endDate)
	if err != nil {
		log.Printf("generic reminder: load reminders failed: %v", err)
		return
	}
	if len(reminders) == 0 {
		return
	}

	userMap, err := loadUsersForReminders(reminders)
	if err != nil {
		log.Printf("generic reminder: load users failed: %v", err)
		return
	}
	checkinMap, err := loadReminderCheckinMap(startDate, endDate)
	if err != nil {
		log.Printf("generic reminder: load checkins failed: %v", err)
		return
	}
	dispatchMap, err := loadReminderDispatchMap(startDate, endDate)
	if err != nil {
		log.Printf("generic reminder: load dispatches failed: %v", err)
		return
	}

	for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
		for _, reminder := range reminders {
			if !isReminderActiveOnDate(reminder, date) {
				continue
			}

			for _, timeText := range parseReminderTimes(reminder.ReminderTime) {
				scheduledAt, err := combineDateAndTime(date, timeText)
				if err != nil {
					continue
				}
				if scheduledAt.Before(windowStart) || scheduledAt.After(windowEnd) {
					continue
				}
				if hasReminderCheckin(checkinMap, reminder.UserID, reminder.ID, date, timeText) {
					continue
				}

				channels := parseChannels(reminder.ReminderChannels)
				if len(channels) == 0 {
					continue
				}

				user := userMap[reminder.UserID]
				for _, channel := range channels {
					if channel == "app" {
						continue
					}
					if !isChannelEnabled(channel) {
						continue
					}

					key := buildReminderDispatchKey(reminder.UserID, reminder.ID, date, timeText, channel)
					if dispatch, ok := dispatchMap[key]; ok && dispatch.Status == "sent" {
						continue
					}

					if err := dispatchGenericReminder(channel, user, reminder, date, timeText, scheduledAt); err != nil {
						saveReminderDispatchFailure(dispatchMap[key], user.ID, reminder.ID, date, timeText, channel, err)
						continue
					}
					saveReminderDispatchSuccess(dispatchMap[key], user.ID, reminder.ID, date, timeText, channel)
				}
			}
		}
	}
}

func loadActiveReminders(startDate, endDate time.Time) ([]models.Reminder, error) {
	var reminders []models.Reminder
	err := global.Db.
		Where("start_date <= ? AND (end_date IS NULL OR end_date >= ?)", endDate, startDate).
		Find(&reminders).Error
	return reminders, err
}

func loadUsersForReminders(reminders []models.Reminder) (map[uint]models.User, error) {
	ids := make([]uint, 0)
	seen := make(map[uint]bool)
	for _, reminder := range reminders {
		if seen[reminder.UserID] {
			continue
		}
		seen[reminder.UserID] = true
		ids = append(ids, reminder.UserID)
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

func loadReminderCheckinMap(startDate, endDate time.Time) (map[string]models.ReminderCheckin, error) {
	var checkins []models.ReminderCheckin
	if err := global.Db.
		Where("scheduled_date >= ? AND scheduled_date <= ?", startDate, endDate).
		Find(&checkins).Error; err != nil {
		return nil, err
	}
	result := make(map[string]models.ReminderCheckin)
	for _, checkin := range checkins {
		key := buildReminderCheckinDispatchKey(checkin.UserID, checkin.ReminderID, checkin.ScheduledDate, checkin.ScheduledTime)
		result[key] = checkin
	}
	return result, nil
}

func loadReminderDispatchMap(startDate, endDate time.Time) (map[string]models.ReminderDispatch, error) {
	var dispatches []models.ReminderDispatch
	if err := global.Db.
		Where("scheduled_date >= ? AND scheduled_date <= ?", startDate, endDate).
		Find(&dispatches).Error; err != nil {
		return nil, err
	}
	result := make(map[string]models.ReminderDispatch)
	for _, dispatch := range dispatches {
		key := buildReminderDispatchKey(dispatch.UserID, dispatch.ReminderID, dispatch.ScheduledDate, dispatch.ScheduledTime, dispatch.Channel)
		result[key] = dispatch
	}
	return result, nil
}

func hasReminderCheckin(checkinMap map[string]models.ReminderCheckin, userID, reminderID uint, date time.Time, timeText string) bool {
	key := buildReminderCheckinDispatchKey(userID, reminderID, date, timeText)
	checkin, ok := checkinMap[key]
	if !ok {
		return false
	}
	return checkin.Status == genericReminderStatusDone || checkin.Status == genericReminderStatusSkip
}

func dispatchGenericReminder(channel string, user models.User, reminder models.Reminder, date time.Time, timeText string, scheduledAt time.Time) error {
	if user.Phone == "" {
		return errors.New("user phone is empty")
	}
	message := buildGenericReminderMessage(reminder, timeText)
	payload := map[string]any{
		"channel":        channel,
		"user_id":        user.ID,
		"username":       user.Username,
		"phone":          user.Phone,
		"reminder_id":    reminder.ID,
		"reminder_type":  reminder.Type,
		"title":          reminder.Title,
		"description":    reminder.Description,
		"notes":          reminder.Notes,
		"alert_style":    normalizeAlertStyle(reminder.AlertStyle),
		"scheduled_date": formatDate(date),
		"scheduled_time": timeText,
		"scheduled_at":   scheduledAt.Format(time.RFC3339),
		"message":        message,
	}
	endpoint := webhookEndpoint(channel)
	if endpoint == "" {
		return errors.New("webhook endpoint not configured")
	}
	return postJSON(endpoint, payload)
}

func saveReminderDispatchSuccess(existing models.ReminderDispatch, userID, reminderID uint, date time.Time, timeText, channel string) {
	now := time.Now()
	if existing.ID != 0 {
		global.Db.Model(&existing).Updates(map[string]any{
			"status":        "sent",
			"sent_at":       &now,
			"error_message": "",
		})
		return
	}
	record := models.ReminderDispatch{
		UserID:        userID,
		ReminderID:    reminderID,
		ScheduledDate: date,
		ScheduledTime: timeText,
		Channel:       channel,
		Status:        "sent",
		SentAt:        &now,
	}
	_ = global.Db.Create(&record).Error
}

func saveReminderDispatchFailure(existing models.ReminderDispatch, userID, reminderID uint, date time.Time, timeText, channel string, err error) {
	message := err.Error()
	if existing.ID != 0 {
		global.Db.Model(&existing).Updates(map[string]any{
			"status":        "failed",
			"error_message": message,
		})
		return
	}
	record := models.ReminderDispatch{
		UserID:        userID,
		ReminderID:    reminderID,
		ScheduledDate: date,
		ScheduledTime: timeText,
		Channel:       channel,
		Status:        "failed",
		ErrorMessage:  message,
	}
	_ = global.Db.Create(&record).Error
}

func buildGenericReminderMessage(reminder models.Reminder, timeText string) string {
	prefix := "提醒"
	if normalizeAlertStyle(reminder.AlertStyle) == "strong" {
		prefix = "强提醒"
	}
	description := strings.TrimSpace(reminder.Description)
	if description != "" {
		description = "，" + description
	}
	return fmt.Sprintf("%s：%s %s%s。", prefix, timeText, reminder.Title, description)
}

func isReminderActiveOnDate(reminder models.Reminder, date time.Time) bool {
	dateStr := formatDate(date)
	switch strings.ToLower(strings.TrimSpace(reminder.RepeatRule)) {
	case genericReminderRepeatOnce:
		if reminder.ReminderDate != nil {
			return formatDate(*reminder.ReminderDate) == dateStr
		}
		return formatDate(reminder.StartDate) == dateStr
	default:
		if formatDate(reminder.StartDate) > dateStr {
			return false
		}
		if reminder.EndDate != nil && formatDate(*reminder.EndDate) < dateStr {
			return false
		}
		return true
	}
}

func buildReminderCheckinDispatchKey(userID, reminderID uint, date time.Time, timeText string) string {
	return fmt.Sprintf("%d|%d|%s|%s", userID, reminderID, formatDate(date), timeText)
}

func buildReminderDispatchKey(userID, reminderID uint, date time.Time, timeText, channel string) string {
	return fmt.Sprintf("%d|%d|%s|%s|%s", userID, reminderID, formatDate(date), timeText, channel)
}
