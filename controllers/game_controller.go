package controllers

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type gameResultRequest struct {
	GameName   string          `json:"game_name" binding:"required"`
	Score      int             `json:"score"`
	DurationMs int             `json:"duration_ms"`
	Difficulty int             `json:"difficulty"`
	Accuracy   float64         `json:"accuracy"`
	Success    *bool           `json:"success"`
	Answers    json.RawMessage `json:"answers"`
	Meta       map[string]any  `json:"meta"`
}

// CreateGameResult 保存康复训练小游戏的结果
func CreateGameResult(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var req gameResultRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}
	name := strings.TrimSpace(req.GameName)
	if name == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "game_name is required")
		return
	}

	metaStr := ""
	if len(req.Meta) > 0 {
		if b, err := json.Marshal(req.Meta); err == nil {
			metaStr = string(b)
		}
	}

	rule := getGameDifficultyRule(name)
	difficulty := req.Difficulty
	if difficulty <= 0 {
		difficulty = rule.Default
	}
	if difficulty <= 0 {
		difficulty = rule.Min
	}
	difficulty = clampDifficulty(difficulty, rule.Min, rule.Max)

	success := deriveSuccess(req.Success, req.Accuracy, req.Score)
	accuracy := normalizeAccuracy(req.Accuracy)
	if accuracy == 0 && success {
		accuracy = 1
	}

	answersStr := ""
	if len(req.Answers) > 0 && string(req.Answers) != "null" {
		answersStr = string(req.Answers)
	}

	gr := models.GameResult{
		UserID:     user.ID,
		GameName:   name,
		Score:      req.Score,
		DurationMs: req.DurationMs,
		Difficulty: difficulty,
		Accuracy:   accuracy,
		Success:    success,
		Answers:    answersStr,
		Meta:       metaStr,
	}
	if err := global.Db.Create(&gr).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not save game result")
		return
	}

	nextDifficulty := computeNextDifficulty(rule, difficulty, success, accuracy)
	if rule.UseAdversarial {
		if history, err := loadRecentResults(user.ID, name, adversarialHistoryLimit); err == nil {
			nextDifficulty = computeAdversarialDifficulty(rule, difficulty, history)
		}
	}

	utils.RespondOK(ctx, gin.H{
		"id":              gr.ID,
		"game_name":       gr.GameName,
		"score":           gr.Score,
		"duration_ms":     gr.DurationMs,
		"difficulty":      gr.Difficulty,
		"accuracy":        gr.Accuracy,
		"success":         gr.Success,
		"next_difficulty": nextDifficulty,
		"min_difficulty":  rule.Min,
		"max_difficulty":  rule.Max,
	})
}

// GetGameDifficulty 返回建议的下一难度
func GetGameDifficulty(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	name := strings.TrimSpace(ctx.Query("game_name"))
	if name == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "game_name is required")
		return
	}

	rule := getGameDifficultyRule(name)

	if rule.UseAdversarial {
		history, err := loadRecentResults(user.ID, name, adversarialHistoryLimit)
		if err != nil {
			utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load game result")
			return
		}
		if len(history) == 0 {
			utils.RespondOK(ctx, gin.H{
				"game_name":      name,
				"difficulty":     rule.Default,
				"min_difficulty": rule.Min,
				"max_difficulty": rule.Max,
			})
			return
		}

		last := history[0]
		nextDifficulty := computeAdversarialDifficulty(rule, last.Difficulty, history)
		utils.RespondOK(ctx, gin.H{
			"game_name":       name,
			"difficulty":      nextDifficulty,
			"last_difficulty": last.Difficulty,
			"last_accuracy":   last.Accuracy,
			"last_success":    last.Success,
			"min_difficulty":  rule.Min,
			"max_difficulty":  rule.Max,
		})
		return
	}

	var last models.GameResult
	if err := global.Db.
		Where("user_id = ? AND game_name = ?", user.ID, name).
		Order("id DESC").
		First(&last).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondOK(ctx, gin.H{
				"game_name":      name,
				"difficulty":     rule.Default,
				"min_difficulty": rule.Min,
				"max_difficulty": rule.Max,
			})
			return
		}
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load game result")
		return
	}

	nextDifficulty := computeNextDifficulty(rule, last.Difficulty, last.Success, last.Accuracy)
	utils.RespondOK(ctx, gin.H{
		"game_name":       name,
		"difficulty":      nextDifficulty,
		"last_difficulty": last.Difficulty,
		"last_accuracy":   last.Accuracy,
		"last_success":    last.Success,
		"min_difficulty":  rule.Min,
		"max_difficulty":  rule.Max,
	})
}

type gameDifficultyRule struct {
	Min           int
	Max           int
	Default       int
	UpThreshold   float64
	DownThreshold float64
	UseAdversarial bool
	TargetAccuracy float64
}

var gameDifficultyRules = map[string]gameDifficultyRule{
	"penguin_memory":     {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6, UseAdversarial: true, TargetAccuracy: 0.75},
	"schulte_grid":       {Min: 1, Max: 6, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
	"schulte_grid_quick": {Min: 1, Max: 4, Default: 1, UpThreshold: 0.9, DownThreshold: 0.6},
	"mmse_warmup":        {Min: 1, Max: 1, Default: 1, UpThreshold: 1, DownThreshold: 0},
	"spot_difference":    {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
	"forward_reverse_numbers": {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
	"leaf_attention":     {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
}

func getGameDifficultyRule(gameName string) gameDifficultyRule {
	if rule, ok := gameDifficultyRules[gameName]; ok {
		return rule
	}
	return gameDifficultyRule{Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6}
}

const (
	adversarialHistoryLimit = 6
	adversarialDecay        = 0.7
	adversarialSlope        = 6.0
	adversarialSpeedMidMs   = 12000.0
	adversarialSpeedScaleMs = 2200.0
	adversarialMaxStep      = 1
)

func loadRecentResults(userID uint, gameName string, limit int) ([]models.GameResult, error) {
	var results []models.GameResult
	err := global.Db.
		Where("user_id = ? AND game_name = ?", userID, gameName).
		Order("id DESC").
		Limit(limit).
		Find(&results).Error
	return results, err
}

func computeAdversarialDifficulty(rule gameDifficultyRule, current int, history []models.GameResult) int {
	if current <= 0 && len(history) > 0 {
		current = history[0].Difficulty
	}
	if current <= 0 {
		current = rule.Default
	}
	current = clampDifficulty(current, rule.Min, rule.Max)
	if len(history) == 0 || rule.Max <= rule.Min {
		return current
	}

	skill := computeSkillScore(history)
	target := rule.TargetAccuracy
	if target <= 0 {
		target = 0.75
	}

	best := current
	bestDelta := math.MaxFloat64
	for d := rule.Min; d <= rule.Max; d++ {
		expected := expectedSuccessProbability(skill, d, rule)
		delta := math.Abs(expected - target)
		if delta < bestDelta {
			best = d
			bestDelta = delta
		}
	}

	if best > current+adversarialMaxStep {
		best = current + adversarialMaxStep
	}
	if best < current-adversarialMaxStep {
		best = current - adversarialMaxStep
	}

	return clampDifficulty(best, rule.Min, rule.Max)
}

func computeSkillScore(history []models.GameResult) float64 {
	var sum float64
	var weightSum float64
	weight := 1.0

	for _, result := range history {
		perf := performanceFromResult(result)
		sum += perf * weight
		weightSum += weight
		weight *= adversarialDecay
	}

	if weightSum <= 0 {
		return 0.5
	}
	return clamp01(sum / weightSum)
}

func performanceFromResult(result models.GameResult) float64 {
	acc := normalizeAccuracy(result.Accuracy)
	if acc == 0 && result.Success {
		acc = 1
	}

	speedScore := 0.0
	if result.DurationMs > 0 {
		speedScore = 1 / (1 + math.Exp((float64(result.DurationMs)-adversarialSpeedMidMs)/adversarialSpeedScaleMs))
	}

	return clamp01(acc*0.8 + speedScore*0.2)
}

func expectedSuccessProbability(skill float64, difficulty int, rule gameDifficultyRule) float64 {
	if rule.Max <= rule.Min {
		return 1
	}
	diffNorm := float64(difficulty-rule.Min) / float64(rule.Max-rule.Min)
	return 1 / (1 + math.Exp(adversarialSlope*(diffNorm-skill)))
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func computeNextDifficulty(rule gameDifficultyRule, current int, success bool, accuracy float64) int {
	if current <= 0 {
		current = rule.Default
	}
	if current <= 0 {
		current = rule.Min
	}

	acc := normalizeAccuracy(accuracy)
	if acc == 0 && success {
		acc = 1
	}

	next := current
	if success && acc >= rule.UpThreshold {
		next = current + 1
	} else if !success || (acc > 0 && acc < rule.DownThreshold) {
		next = current - 1
	}

	return clampDifficulty(next, rule.Min, rule.Max)
}

func clampDifficulty(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

func normalizeAccuracy(raw float64) float64 {
	if raw <= 0 {
		return 0
	}
	if raw > 1 && raw <= 100 {
		raw = raw / 100
	}
	if raw > 1 {
		return 1
	}
	return raw
}

func deriveSuccess(explicit *bool, accuracy float64, score int) bool {
	if explicit != nil {
		return *explicit
	}
	acc := normalizeAccuracy(accuracy)
	if acc > 0 {
		return acc >= 0.8
	}
	return score > 0
}
