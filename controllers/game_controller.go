package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
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

type scoreBreakdown struct {
	Accuracy       float64 `json:"accuracy"`
	Completion     float64 `json:"completion"`
	TimeEfficiency float64 `json:"time_efficiency"`
	Stability      float64 `json:"stability"`
}

type difficultyEnvelope struct {
	Baseline    int
	Recommended int
	BandLow     int
	BandHigh    int
	Source      string
	Reason      string
	History     []models.GameResult
}

type baselineSelection struct {
	Baseline int
	BandLow  int
	BandHigh int
	Source   string
	Reason   string
}

type gameDifficultyRule struct {
	Min            int
	Max            int
	Default        int
	UpThreshold    float64
	DownThreshold  float64
	UseAdversarial bool
	TargetAccuracy float64
}

type gameScoringProfile struct {
	TargetDurationMs map[int]int
}

var gameDifficultyRules = map[string]gameDifficultyRule{
	"penguin_memory":          {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6, UseAdversarial: true, TargetAccuracy: 0.75},
	"schulte_grid":            {Min: 1, Max: 6, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
	"schulte_grid_quick":      {Min: 1, Max: 4, Default: 1, UpThreshold: 0.9, DownThreshold: 0.6},
	"mmse_warmup":             {Min: 1, Max: 1, Default: 1, UpThreshold: 1, DownThreshold: 0},
	"spot_difference":         {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
	"forward_reverse_numbers": {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
	"leaf_attention":          {Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6},
}

var gameScoringProfiles = map[string]gameScoringProfile{
	"penguin_memory": {
		TargetDurationMs: map[int]int{1: 22000, 2: 18000, 3: 15000, 4: 13000, 5: 11000},
	},
	"schulte_grid": {
		TargetDurationMs: map[int]int{1: 30000, 2: 45000, 3: 60000, 4: 75000, 5: 90000, 6: 90000},
	},
	"schulte_grid_quick": {
		TargetDurationMs: map[int]int{1: 25000, 2: 32000, 3: 40000, 4: 50000},
	},
	"spot_difference": {
		TargetDurationMs: map[int]int{1: 90000, 2: 80000, 3: 70000, 4: 60000, 5: 55000},
	},
	"forward_reverse_numbers": {
		TargetDurationMs: map[int]int{1: 120000, 2: 110000, 3: 100000, 4: 90000, 5: 80000},
	},
	"leaf_attention": {
		TargetDurationMs: map[int]int{1: 60000, 2: 60000, 3: 60000, 4: 60000, 5: 60000},
	},
	"mmse_warmup": {
		TargetDurationMs: map[int]int{1: 30000},
	},
}

const (
	performanceWindow = 5
	stabilityWindow   = 3
)

// CreateGameResult stores a rehab game outcome and returns the next difficulty.
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

	rule := getGameDifficultyRule(name)
	envelope, err := buildGameDifficultyEnvelope(user.ID, name, true)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not resolve difficulty context")
		return
	}

	difficulty := req.Difficulty
	if difficulty <= 0 {
		difficulty = envelope.Recommended
	}
	if difficulty <= 0 {
		difficulty = rule.Default
	}
	difficulty = clampDifficulty(difficulty, rule.Min, rule.Max)

	success := deriveSuccess(req.Success, req.Accuracy, req.Score)
	accuracy := normalizeAccuracy(req.Accuracy)
	if accuracy == 0 && success {
		accuracy = 1
	}

	meta := cloneMetaMap(req.Meta)
	answersStr := ""
	if len(req.Answers) > 0 && string(req.Answers) != "null" {
		answersStr = string(req.Answers)
	}

	result := models.GameResult{
		UserID:     user.ID,
		GameName:   name,
		Score:      req.Score,
		DurationMs: req.DurationMs,
		Difficulty: difficulty,
		Accuracy:   accuracy,
		Success:    success,
		Answers:    answersStr,
	}

	meta["raw_score"] = req.Score
	meta["baseline_difficulty"] = envelope.Baseline
	meta["target_band_low"] = envelope.BandLow
	meta["target_band_high"] = envelope.BandHigh
	meta["difficulty_source"] = envelope.Source

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid meta payload")
		return
	}
	result.Meta = string(metaBytes)

	rehabScore, breakdown := computeRehabScore(name, result, envelope.History)
	resultHistory := append([]models.GameResult{result}, envelope.History...)
	if len(resultHistory) > performanceWindow {
		resultHistory = resultHistory[:performanceWindow]
	}
	nextDifficulty, _, _, _, adjustReason := computeNextDifficultyFromHistory(
		difficulty,
		resultHistory,
		envelope.BandLow,
		envelope.BandHigh,
		rule.Min,
		rule.Max,
	)

	nextReason := envelope.Reason
	if adjustReason != "" {
		if nextReason != "" {
			nextReason += "；"
		}
		nextReason += adjustReason
	}

	meta["rehab_score"] = rehabScore
	meta["score_breakdown"] = breakdown
	meta["next_difficulty"] = nextDifficulty
	meta["next_difficulty_reason"] = nextReason

	metaBytes, err = json.Marshal(meta)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid meta payload")
		return
	}
	result.Meta = string(metaBytes)

	if err := global.Db.Create(&result).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not save game result")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"id":                     result.ID,
		"game_name":              result.GameName,
		"score":                  result.Score,
		"duration_ms":            result.DurationMs,
		"difficulty":             result.Difficulty,
		"accuracy":               result.Accuracy,
		"success":                result.Success,
		"rehab_score":            rehabScore,
		"score_breakdown":        breakdown,
		"baseline_difficulty":    envelope.Baseline,
		"target_band_low":        envelope.BandLow,
		"target_band_high":       envelope.BandHigh,
		"difficulty_source":      envelope.Source,
		"next_difficulty":        nextDifficulty,
		"next_difficulty_reason": nextReason,
		"assist_triggered":       meta["assist_triggered"],
		"assist_level_max":       meta["assist_level_max"],
		"assist_resolution":      meta["assist_resolution"],
		"softened_next_round":    meta["softened_next_round"],
		"min_difficulty":         rule.Min,
		"max_difficulty":         rule.Max,
	})
}

// GetGameDifficulty returns the recommended difficulty for the next round.
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
	envelope, err := buildGameDifficultyEnvelope(user.ID, name, true)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load game result")
		return
	}

	payload := gin.H{
		"game_name":           name,
		"difficulty":          envelope.Recommended,
		"baseline_difficulty": envelope.Baseline,
		"target_band_low":     envelope.BandLow,
		"target_band_high":    envelope.BandHigh,
		"difficulty_source":   envelope.Source,
		"reason":              envelope.Reason,
		"min_difficulty":      rule.Min,
		"max_difficulty":      rule.Max,
	}

	if len(envelope.History) > 0 {
		last := envelope.History[0]
		payload["last_difficulty"] = last.Difficulty
		payload["last_accuracy"] = last.Accuracy
		payload["last_success"] = last.Success
		payload["last_rehab_score"] = historicalRehabScore(last)
	}

	utils.RespondOK(ctx, payload)
}

func getGameDifficultyRule(gameName string) gameDifficultyRule {
	if rule, ok := gameDifficultyRules[gameName]; ok {
		return rule
	}
	return gameDifficultyRule{Min: 1, Max: 5, Default: 1, UpThreshold: 0.85, DownThreshold: 0.6}
}

func getGameScoringProfile(gameName string) gameScoringProfile {
	if profile, ok := gameScoringProfiles[gameName]; ok {
		return profile
	}
	return gameScoringProfile{
		TargetDurationMs: map[int]int{
			1: 60000,
			2: 60000,
			3: 60000,
			4: 60000,
			5: 60000,
		},
	}
}

func loadRecentResults(userID uint, gameName string, limit int) ([]models.GameResult, error) {
	var results []models.GameResult
	err := global.Db.
		Where("user_id = ? AND game_name = ?", userID, gameName).
		Order("id DESC").
		Limit(limit).
		Find(&results).Error
	return results, err
}

func buildGameDifficultyEnvelope(userID uint, gameName string, includePlan bool) (difficultyEnvelope, error) {
	rule := getGameDifficultyRule(gameName)
	history, err := loadRecentResults(userID, gameName, performanceWindow)
	if err != nil {
		return difficultyEnvelope{}, err
	}

	selection := resolveBaselineSelection(userID, gameName, rule, history, includePlan)
	currentDifficulty := selection.Baseline
	if len(history) > 0 && history[0].Difficulty > 0 {
		currentDifficulty = clampDifficulty(history[0].Difficulty, rule.Min, rule.Max)
	}
	recommended, _, _, _, adjustReason := computeNextDifficultyFromHistory(
		currentDifficulty,
		history,
		selection.BandLow,
		selection.BandHigh,
		rule.Min,
		rule.Max,
	)

	reason := selection.Reason
	if adjustReason != "" {
		if reason != "" {
			reason += "；"
		}
		reason += adjustReason
	}

	return difficultyEnvelope{
		Baseline:    selection.Baseline,
		Recommended: recommended,
		BandLow:     selection.BandLow,
		BandHigh:    selection.BandHigh,
		Source:      selection.Source,
		Reason:      reason,
		History:     history,
	}, nil
}

func resolveBaselineSelection(
	userID uint,
	gameName string,
	rule gameDifficultyRule,
	history []models.GameResult,
	includePlan bool,
) baselineSelection {
	if includePlan {
		if planSpec, ok := loadLatestTrainingPlanGameSpec(userID, gameName); ok {
			selection := baselineSelection{
				Baseline: clampDifficulty(planSpec.StartDifficulty, rule.Min, rule.Max),
				BandLow:  clampDifficulty(planSpec.TargetLow, rule.Min, rule.Max),
				BandHigh: clampDifficulty(planSpec.TargetHigh, rule.Min, rule.Max),
				Source:   "plan",
			}
			if selection.Baseline <= 0 {
				selection.Baseline = rule.Default
			}
			if selection.BandLow <= 0 {
				selection.BandLow = clampDifficulty(selection.Baseline-1, rule.Min, rule.Max)
			}
			if selection.BandHigh <= 0 {
				selection.BandHigh = clampDifficulty(selection.Baseline+1, rule.Min, rule.Max)
			}
			if selection.BandLow > selection.BandHigh {
				selection.BandLow, selection.BandHigh = selection.BandHigh, selection.BandLow
			}
			selection.Reason = fmt.Sprintf(
				"当前训练方案建议从 Lv.%d 开始，目标区间 Lv.%d-Lv.%d",
				selection.Baseline,
				selection.BandLow,
				selection.BandHigh,
			)
			return selection
		}
	}

	historyDifficulty, hasHistory := computeHistoryDifficulty(rule, history)
	profileDifficulty, hasProfile, profileScore := computeProfileDifficulty(userID, gameName, rule)
	switch {
	case hasHistory && hasProfile:
		baseline := clampDifficulty(
			int(math.Round(float64(historyDifficulty)*0.6+float64(profileDifficulty)*0.4)),
			rule.Min,
			rule.Max,
		)
		domainName := domainDisplayName[gameToDomain[gameName]]
		if domainName == "" {
			domainName = "认知"
		}
		return baselineSelection{
			Baseline: baseline,
			BandLow:  clampDifficulty(baseline-1, rule.Min, rule.Max),
			BandHigh: clampDifficulty(baseline+1, rule.Min, rule.Max),
			Source:   "history",
			Reason:   fmt.Sprintf("结合近 5 局表现和%s评分 %.1f 分，建议从 Lv.%d 开始", domainName, profileScore, baseline),
		}
	case hasHistory:
		return baselineSelection{
			Baseline: historyDifficulty,
			BandLow:  clampDifficulty(historyDifficulty-1, rule.Min, rule.Max),
			BandHigh: clampDifficulty(historyDifficulty+1, rule.Min, rule.Max),
			Source:   "history",
			Reason:   fmt.Sprintf("根据近 5 局稳定表现，建议从 Lv.%d 开始", historyDifficulty),
		}
	case hasProfile:
		domainName := domainDisplayName[gameToDomain[gameName]]
		if domainName == "" {
			domainName = "认知"
		}
		return baselineSelection{
			Baseline: profileDifficulty,
			BandLow:  clampDifficulty(profileDifficulty-1, rule.Min, rule.Max),
			BandHigh: clampDifficulty(profileDifficulty+1, rule.Min, rule.Max),
			Source:   "profile",
			Reason:   fmt.Sprintf("根据%s评分 %.1f 分，建议从 Lv.%d 开始", domainName, profileScore, profileDifficulty),
		}
	default:
		return baselineSelection{
			Baseline: rule.Default,
			BandLow:  clampDifficulty(rule.Default-1, rule.Min, rule.Max),
			BandHigh: clampDifficulty(rule.Default+1, rule.Min, rule.Max),
			Source:   "profile",
			Reason:   fmt.Sprintf("暂无足够历史数据，先从 Lv.%d 建立基线", rule.Default),
		}
	}
}

func computeHistoryDifficulty(rule gameDifficultyRule, history []models.GameResult) (int, bool) {
	if len(history) == 0 {
		return 0, false
	}
	weights := []float64{1.0, 0.85, 0.7, 0.55, 0.4}
	var sum float64
	var weightSum float64
	for idx, result := range history {
		if idx >= len(weights) {
			break
		}
		difficulty := clampDifficulty(result.Difficulty, rule.Min, rule.Max)
		confidence := 0.7 + historicalRehabScore(result)/200
		weight := weights[idx] * confidence
		sum += float64(difficulty) * weight
		weightSum += weight
	}
	if weightSum <= 0 {
		return 0, false
	}
	return clampDifficulty(int(math.Round(sum/weightSum)), rule.Min, rule.Max), true
}

func computeProfileDifficulty(userID uint, gameName string, rule gameDifficultyRule) (int, bool, float64) {
	domain := gameToDomain[gameName]
	if domain == "" {
		return 0, false, 0
	}
	profile, err := buildCognitiveProfile(userID)
	if err != nil {
		return 0, false, 0
	}
	detail, ok := profile.DomainDetails[domain]
	if !ok || detail.Score <= 0 {
		return 0, false, 0
	}
	return mapScoreToDifficulty(detail.Score, rule.Min, rule.Max), true, detail.Score
}

func mapScoreToDifficulty(score float64, minLevel, maxLevel int) int {
	if maxLevel <= minLevel {
		return minLevel
	}
	normalized := clamp01((score - 35) / 55)
	return clampDifficulty(
		minLevel+int(math.Round(normalized*float64(maxLevel-minLevel))),
		minLevel,
		maxLevel,
	)
}

func computeNextDifficultyFromHistory(
	current int,
	history []models.GameResult,
	bandLow int,
	bandHigh int,
	minLevel int,
	maxLevel int,
) (int, float64, int, int, string) {
	if current <= 0 {
		current = minLevel
	}
	if bandLow <= 0 {
		bandLow = minLevel
	}
	if bandHigh <= 0 {
		bandHigh = maxLevel
	}
	if bandLow > bandHigh {
		bandLow, bandHigh = bandHigh, bandLow
	}
	current = clampDifficulty(current, minLevel, maxLevel)
	ewma := computeHistoricalRehabEWMA(history)
	highStreak, lowStreak := countPerformanceStreaks(history)
	next := current
	reason := fmt.Sprintf("近期康复分 %.1f 分，维持当前训练带", ewma)

	switch {
	case len(history) == 0:
		reason = fmt.Sprintf("暂无近期成绩，先在目标区间 Lv.%d-Lv.%d 内建立节奏", bandLow, bandHigh)
	case highStreak >= 2 && ewma >= 85:
		next = current + 1
		reason = fmt.Sprintf("近 %d 局康复分持续高于 85 分，下一局上调 1 档", highStreak)
	case lowStreak >= 2:
		next = current - 1
		reason = fmt.Sprintf("近 %d 局连续未达目标，下一局下调 1 档", lowStreak)
	case historicalRehabScore(history[0]) < 60:
		next = current - 1
		reason = "本局康复分低于 60 分，下一局下调 1 档"
	}

	if len(history) > 0 {
		next, reason = applyAssistDifficultyConstraint(
			current,
			next,
			minLevel,
			reason,
			parseMetaString(history[0].Meta),
		)
	}

	next = constrainStepWithinBand(current, next, bandLow, bandHigh, minLevel, maxLevel)
	if next == current && (current < bandLow || current > bandHigh) {
		if current < bandLow {
			next = minInt(current+1, bandLow)
		} else {
			next = maxInt(current-1, bandHigh)
		}
		reason = fmt.Sprintf("先回到目标区间 Lv.%d-Lv.%d 内，再继续调档", bandLow, bandHigh)
	}

	return next, round1(ewma), highStreak, lowStreak, reason
}

func applyAssistDifficultyConstraint(
	current int,
	proposed int,
	minLevel int,
	currentReason string,
	meta map[string]any,
) (int, string) {
	assistTriggered, assistLevel, resolution := extractAssistContext(meta)
	if !assistTriggered && resolution == "independent" && assistLevel < 2 {
		return proposed, currentReason
	}

	switch resolution {
	case "completed_with_guidance", "unfinished":
		return maxInt(minLevel, minInt(current-1, proposed)), "本局已触发温柔辅助，下一局放缓一档"
	case "corrected_after_hint":
		return minInt(current, proposed), "本局在提示下完成，下一局保持当前难度"
	default:
		if assistLevel >= 2 {
			return minInt(current, proposed), "本局已触发语音辅助，下一局不升难"
		}
	}

	return proposed, currentReason
}

func constrainStepWithinBand(current, target, bandLow, bandHigh, minLevel, maxLevel int) int {
	target = clampDifficulty(target, minLevel, maxLevel)
	if target > current+1 {
		target = current + 1
	} else if target < current-1 {
		target = current - 1
	}
	if current < bandLow {
		return minInt(current+1, bandLow)
	}
	if current > bandHigh {
		return maxInt(current-1, bandHigh)
	}
	if target < bandLow {
		return maxInt(current-1, bandLow)
	}
	if target > bandHigh {
		return minInt(current+1, bandHigh)
	}
	return clampDifficulty(target, minLevel, maxLevel)
}

func computeHistoricalRehabEWMA(history []models.GameResult) float64 {
	if len(history) == 0 {
		return 0
	}
	alpha := 0.6
	ewma := historicalRehabScore(history[len(history)-1])
	for idx := len(history) - 2; idx >= 0; idx-- {
		score := historicalRehabScore(history[idx])
		ewma = alpha*score + (1-alpha)*ewma
	}
	return ewma
}

func countPerformanceStreaks(history []models.GameResult) (int, int) {
	highStreak := 0
	lowStreak := 0
	for _, result := range history {
		score := historicalRehabScore(result)
		if score >= 85 && result.Success {
			highStreak++
		} else {
			break
		}
	}
	for _, result := range history {
		score := historicalRehabScore(result)
		if score < 60 || !result.Success {
			lowStreak++
		} else {
			break
		}
	}
	return highStreak, lowStreak
}

func historicalRehabScore(result models.GameResult) float64 {
	meta := parseMetaString(result.Meta)
	if score := toFloat(meta["rehab_score"]); score > 0 {
		return clampPercent(score)
	}
	rehabScore, _ := computeRehabScore(result.GameName, result, nil)
	return rehabScore
}

func computeRehabScore(gameName string, result models.GameResult, recent []models.GameResult) (float64, scoreBreakdown) {
	meta := parseMetaString(result.Meta)
	accuracyScore := round1(normalizeAccuracy(result.Accuracy) * 100)
	completionScore := round1(computeCompletionScore(result, meta, accuracyScore))
	timeEfficiency := round1(computeTimeEfficiencyScore(gameName, result.Difficulty, result.DurationMs))
	stability := round1(computeStabilityScore(result, meta, recent, accuracyScore))

	rehabScore := round1(
		accuracyScore*0.55 +
			completionScore*0.20 +
			timeEfficiency*0.15 +
			stability*0.10,
	)

	return rehabScore, scoreBreakdown{
		Accuracy:       accuracyScore,
		Completion:     completionScore,
		TimeEfficiency: timeEfficiency,
		Stability:      stability,
	}
}

func computeCompletionScore(result models.GameResult, meta map[string]any, accuracyScore float64) float64 {
	completed, total := extractTaskProgress(meta)
	if total > 0 {
		return clampPercent(float64(completed) / float64(total) * 100)
	}
	if result.Success {
		return 100
	}
	return accuracyScore
}

func computeTimeEfficiencyScore(gameName string, difficulty, durationMs int) float64 {
	if durationMs <= 0 {
		return 50
	}
	profile := getGameScoringProfile(gameName)
	targetMs := profile.TargetDurationMs[clampDifficulty(difficulty, 1, 10)]
	if targetMs <= 0 {
		targetMs = 60000
	}
	return clampPercent(float64(targetMs) / float64(durationMs) * 100)
}

func computeStabilityScore(
	result models.GameResult,
	meta map[string]any,
	recent []models.GameResult,
	accuracyScore float64,
) float64 {
	errorRate := extractErrorRate(meta, accuracyScore)
	samples := []float64{accuracyScore}
	for idx, item := range recent {
		if idx >= stabilityWindow {
			break
		}
		samples = append(samples, normalizeAccuracy(item.Accuracy)*100)
	}

	var variationPenalty float64
	if len(samples) > 1 {
		var totalDelta float64
		for idx := 0; idx < len(samples)-1; idx++ {
			totalDelta += math.Abs(samples[idx] - samples[idx+1])
		}
		variationPenalty = totalDelta / float64(len(samples)-1)
	}

	stability := 100 - variationPenalty - errorRate*30
	if result.Success && accuracyScore >= 85 {
		stability += 3
	}
	return clampPercent(stability)
}

func extractTaskProgress(meta map[string]any) (int, int) {
	completed := int(toFloat(meta["task_completed"]))
	total := int(toFloat(meta["task_total"]))
	if total > 0 {
		return completed, total
	}

	if found := int(toFloat(meta["found"])); found > 0 {
		if diffCount := int(toFloat(meta["diff_count"])); diffCount > 0 {
			return found, diffCount
		}
	}
	if correct := int(toFloat(meta["correct"])); correct >= 0 {
		if totalCount := int(toFloat(meta["total"])); totalCount > 0 {
			return correct, totalCount
		}
		if wrong := int(toFloat(meta["wrong"])); wrong >= 0 && correct+wrong > 0 {
			return correct, correct + wrong
		}
		if pairs := int(toFloat(meta["pairs"])); pairs > 0 {
			if items := int(toFloat(meta["items"])); items > 0 {
				return correct, pairs * items
			}
		}
	}
	return 0, 0
}

func extractErrorRate(meta map[string]any, accuracyScore float64) float64 {
	if total := toFloat(meta["total"]); total > 0 {
		wrong := toFloat(meta["wrong"])
		return clamp01(wrong / total)
	}
	if diffCount := toFloat(meta["diff_count"]); diffCount > 0 {
		found := toFloat(meta["found"])
		return clamp01((diffCount - found) / diffCount)
	}
	return clamp01(1 - accuracyScore/100)
}

func parseMetaString(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	meta := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return map[string]any{}
	}
	return meta
}

func extractAssistContext(meta map[string]any) (bool, int, string) {
	assistTriggered, _ := meta["assist_triggered"].(bool)
	assistLevel := int(toFloat(meta["assist_level_max"]))
	resolution, _ := meta["assist_resolution"].(string)
	if resolution == "" {
		resolution = "independent"
	}
	if assistLevel > 0 {
		assistTriggered = true
	}
	return assistTriggered, assistLevel, resolution
}

func cloneMetaMap(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(meta))
	for key, value := range meta {
		cloned[key] = value
	}
	return cloned
}

func round1(value float64) float64 {
	return math.Round(value*10) / 10
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
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

func findLatestResult(userID uint, gameName string) (models.GameResult, error) {
	var last models.GameResult
	err := global.Db.
		Where("user_id = ? AND game_name = ?", userID, gameName).
		Order("id DESC").
		First(&last).Error
	return last, err
}

func resultExists(userID uint, gameName string) bool {
	_, err := findLatestResult(userID, gameName)
	return !errors.Is(err, gorm.ErrRecordNotFound)
}
