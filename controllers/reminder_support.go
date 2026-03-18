package controllers

import (
	"sort"
	"strings"
	"time"

	"ccrt_sever/models"
)

const (
	reminderTypeMedication = "medication"
	reminderTypeHabit      = "habit"
	reminderTypeTraining   = "training"
	reminderTypeTask       = "task"

	reminderRepeatOnce  = "once"
	reminderRepeatDaily = "daily"

	reminderStatusDone    = "done"
	reminderStatusSkipped = "skipped"
)

func normalizeReminderType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case reminderTypeMedication:
		return reminderTypeMedication
	case reminderTypeHabit:
		return reminderTypeHabit
	case reminderTypeTraining:
		return reminderTypeTraining
	default:
		return reminderTypeTask
	}
}

func normalizeReminderRepeatRule(raw string, hasExplicitDate bool) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case reminderRepeatOnce:
		return reminderRepeatOnce
	case reminderRepeatDaily:
		return reminderRepeatDaily
	default:
		if hasExplicitDate {
			return reminderRepeatOnce
		}
		return reminderRepeatDaily
	}
}

func parseReminderOptionalDate(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	date := reminderDateOnly(parsed)
	return &date, nil
}

func parseReminderRequiredOrToday(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return reminderDateOnly(time.Now()), nil
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, err
	}
	return reminderDateOnly(parsed), nil
}

func reminderDateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func reminderFormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func reminderIsActiveOnDate(reminder models.Reminder, date time.Time) bool {
	date = reminderDateOnly(date)
	switch reminder.RepeatRule {
	case reminderRepeatOnce:
		if reminder.ReminderDate != nil {
			return reminderFormatDate(*reminder.ReminderDate) == reminderFormatDate(date)
		}
		return reminderFormatDate(reminder.StartDate) == reminderFormatDate(date)
	default:
		if reminderFormatDate(reminder.StartDate) > reminderFormatDate(date) {
			return false
		}
		if reminder.EndDate != nil && reminderFormatDate(*reminder.EndDate) < reminderFormatDate(date) {
			return false
		}
		return true
	}
}

func reminderTypeLabel(reminderType string) string {
	switch normalizeReminderType(reminderType) {
	case reminderTypeMedication:
		return "用药"
	case reminderTypeHabit:
		return "习惯"
	case reminderTypeTraining:
		return "训练"
	default:
		return "事项"
	}
}

func reminderDefaultTitle(reminderType string) string {
	switch normalizeReminderType(reminderType) {
	case reminderTypeMedication:
		return "按时用药"
	case reminderTypeHabit:
		return "生活习惯提醒"
	case reminderTypeTraining:
		return "脑力训练提醒"
	default:
		return "待办提醒"
	}
}

func parseReminderTimesSorted(raw string) []string {
	times := parseReminderTimes(raw)
	sort.Strings(times)
	return times
}
