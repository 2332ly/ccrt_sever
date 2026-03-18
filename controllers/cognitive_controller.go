package controllers

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

var mmseModuleToDomain = map[string]string{
	"定向力":     "orientation",
	"记忆力":     "memory",
	"注意力和计算力": "calculation",
	"回忆力":     "recall",
	"语言能力":    "language",
}

var gameToDomain = map[string]string{
	"schulte_grid":            "calculation",
	"schulte_grid_quick":      "calculation",
	"penguin_memory":          "recall",
	"leaf_attention":          "calculation",
	"spot_difference":         "language",
	"forward_reverse_numbers": "memory",
}

var gameDisplayName = map[string]string{
	"schulte_grid":            "舒尔特方格",
	"schulte_grid_quick":      "舒尔特方格·快速",
	"penguin_memory":          "企鹅反应",
	"leaf_attention":          "树叶注意力",
	"spot_difference":         "找不同",
	"forward_reverse_numbers": "正倒序数字",
}

var domainDisplayName = map[string]string{
	"orientation": "定向力",
	"memory":      "记忆力",
	"calculation": "注意力和计算力",
	"recall":      "回忆力",
	"language":    "语言能力",
}

var domainToGameCandidates = map[string][]string{
	"orientation": {"schulte_grid"},
	"memory":      {"forward_reverse_numbers", "penguin_memory"},
	"calculation": {"schulte_grid", "leaf_attention"},
	"recall":      {"penguin_memory", "forward_reverse_numbers"},
	"language":    {"spot_difference"},
}

var cognitiveProfileDomains = []string{
	"orientation",
	"memory",
	"calculation",
	"recall",
	"language",
}

var cognitiveProfileDomainWeights = map[string]float64{
	"orientation": 10.0 / 30.0,
	"memory":      3.0 / 30.0,
	"calculation": 5.0 / 30.0,
	"recall":      3.0 / 30.0,
	"language":    9.0 / 30.0,
}

const (
	cognitiveProfileVersion  = "evidence_weighted_v2"
	cognitiveProfileModel    = "mmse_game_fusion_v2"
	cognitivePriorScore      = 55.0
	mmseEvidenceLookbackDays = 180
	mmseEvidenceMaxSamples   = 3
	gameEvidenceLookbackDays = 21
	gameTrendWindowDays      = 14
	mmseEvidenceDecayDays    = 45.0
	mmseConfidenceDecayDays  = 60.0
	gameEvidenceDecayDays    = 10.0
	gameConfidenceDecayDays  = 14.0
)

func GetCognitiveProfile(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	profile, err := buildCognitiveProfile(user.ID)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not build cognitive profile")
		return
	}

	saveCognitiveProfile(user.ID, profile)

	utils.RespondOK(ctx, gin.H{
		"profile_version":          profile.ProfileVersion,
		"scoring_model":            profile.ScoringModel,
		"orientation_score":        profile.OrientationScore,
		"memory_score":             profile.MemoryScore,
		"calculation_score":        profile.CalculationScore,
		"recall_score":             profile.RecallScore,
		"language_score":           profile.LanguageScore,
		"overall_score":            profile.OverallScore,
		"overall_level":            profile.OverallLevel,
		"overall_confidence":       profile.OverallConfidence,
		"mmse_source":              profile.MmseSource,
		"game_source":              profile.GameSource,
		"priority_domains":         profile.PriorityDomains,
		"needs_assessment_domains": profile.NeedsAssessmentDomains,
		"source_summary":           profile.SourceSummary,
		"domain_details":           profile.DomainDetails,
	})
}

func GenerateTrainingPlan(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	profile, err := buildCognitiveProfile(user.ID)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "INTERNAL_ERROR", "could not build cognitive profile")
		return
	}

	plan, err := generateTrainingPlanWithAI(user.ID, profile)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "PLAN_ERROR", err.Error())
		return
	}

	utils.RespondOK(ctx, plan)
}

func GetCurrentTrainingPlan(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var plan models.TrainingPlan
	if err := global.Db.Where("user_id = ?", user.ID).Order("id DESC").First(&plan).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "no training plan found, please generate one")
		return
	}

	var weakDomains []string
	_ = json.Unmarshal([]byte(plan.WeakDomains), &weakDomains)

	var recGames []map[string]any
	_ = json.Unmarshal([]byte(plan.RecommendedGames), &recGames)
	recGames = normalizeTrainingPlanGames(user.ID, recGames)

	utils.RespondOK(ctx, gin.H{
		"id":                plan.ID,
		"weak_domains":      weakDomains,
		"recommended_games": recGames,
		"daily_goal":        plan.DailyGoal,
		"ai_summary":        plan.AISummary,
		"ai_model":          plan.AIModel,
		"created_at":        plan.CreatedAt,
	})
}

func GetCognitiveTrend(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "30"))
	if days <= 0 || days > 365 {
		days = 30
	}

	since := time.Now().AddDate(0, 0, -days)
	mmsePoints := getMMSETrendPoints(user.ID, since)
	gamePoints := getGameTrendPoints(user.ID, since)

	utils.RespondOK(ctx, buildCognitiveTrendPayload(days, mmsePoints, gamePoints))
}

func GetTrainingSummary(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days <= 0 || days > 365 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	var results []models.GameResult
	global.Db.Where("user_id = ? AND created_at >= ?", user.ID, since).
		Order("created_at ASC").
		Find(&results)

	utils.RespondOK(ctx, buildTrainingSummaryPayload(days, results))
}

func GenerateAIReport(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days <= 0 || days > 90 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	profile, _ := buildCognitiveProfile(user.ID)

	var results []models.GameResult
	global.Db.Where("user_id = ? AND created_at >= ?", user.ID, since).
		Order("created_at ASC").
		Find(&results)

	totalSessions := len(results)
	domainScores := map[string][]float64{}
	for _, result := range results {
		domain := gameToDomain[result.GameName]
		if domain == "" {
			domain = "other"
		}
		domainScores[domain] = append(domainScores[domain], historicalRehabScore(result)/100)
	}

	domainAvgs := map[string]float64{}
	for domain, scores := range domainScores {
		var sum float64
		for _, score := range scores {
			sum += score
		}
		domainAvgs[domain] = math.Round(sum/float64(len(scores))*1000) / 1000
	}

	summaryData, _ := json.Marshal(map[string]any{
		"days":                     days,
		"total_sessions":           totalSessions,
		"domain_avg_accuracy":      domainAvgs,
		"cognitive_profile":        profile,
		"overall_confidence":       profile.OverallConfidence,
		"priority_domains":         profile.PriorityDomains,
		"needs_assessment_domains": profile.NeedsAssessmentDomains,
		"source_summary":           profile.SourceSummary,
	})

	messages := []utils.OpenAIMessage{
		{
			Role: "system",
			Content: `你是认知康复训练助手。请根据用户近期训练数据和认知画像，生成一份 200 字以内的阶段性报告。
要求：
1. 直接对用户使用“您/您的”
2. 语气温和、专业
3. 点出表现较好的认知域和需要加强的认知域
4. 给出 2-3 条具体训练建议
5. 只返回报告文本，不要 JSON`,
		},
		{
			Role:    "user",
			Content: string(summaryData),
		},
	}

	content, aiModel, err := utils.OpenAIChat(messages, 0.5)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", err.Error())
		return
	}

	utils.RespondOK(ctx, gin.H{
		"report":   strings.TrimSpace(content),
		"ai_model": aiModel,
		"days":     days,
		"profile":  profile,
	})
}

type cognitiveProfileResp struct {
	ProfileVersion         string                  `json:"profile_version"`
	ScoringModel           string                  `json:"scoring_model"`
	OrientationScore       float64                 `json:"orientation_score"`
	MemoryScore            float64                 `json:"memory_score"`
	CalculationScore       float64                 `json:"calculation_score"`
	RecallScore            float64                 `json:"recall_score"`
	LanguageScore          float64                 `json:"language_score"`
	OverallScore           float64                 `json:"overall_score"`
	OverallLevel           string                  `json:"overall_level"`
	OverallConfidence      float64                 `json:"overall_confidence"`
	MmseSource             bool                    `json:"mmse_source"`
	GameSource             bool                    `json:"game_source"`
	PriorityDomains        []string                `json:"priority_domains"`
	NeedsAssessmentDomains []string                `json:"needs_assessment_domains"`
	SourceSummary          profileSourceSummary    `json:"source_summary"`
	DomainDetails          map[string]domainDetail `json:"domain_details"`
}

type domainDetail struct {
	DisplayName      string        `json:"display_name"`
	Score            float64       `json:"score"`
	MmseScore        float64       `json:"mmse_score"`
	GameScore        float64       `json:"game_score"`
	Level            string        `json:"level"`
	Confidence       float64       `json:"confidence"`
	EvidenceStatus   string        `json:"evidence_status"`
	TrendDelta14d    float64       `json:"trend_delta_14d"`
	DisagreementFlag bool          `json:"disagreement_flag"`
	SourceWeights    sourceWeights `json:"source_weights"`
	LatestMMSEAt     *time.Time    `json:"latest_mmse_at,omitempty"`
	LatestGameAt     *time.Time    `json:"latest_game_at,omitempty"`
	MMSECount        int           `json:"mmse_count"`
	GameSessions21d  int           `json:"game_sessions_21d"`
}

type sourceWeights struct {
	MMSE float64 `json:"mmse"`
	Game float64 `json:"game"`
}

type profileSourceSummary struct {
	MMSECount       int        `json:"mmse_count"`
	LatestMMSEAt    *time.Time `json:"latest_mmse_at,omitempty"`
	GameSessions21d int        `json:"game_sessions_21d"`
	LatestGameAt    *time.Time `json:"latest_game_at,omitempty"`
}

type domainMMSEEvidence struct {
	Score    float64
	Count    int
	LatestAt *time.Time
}

type domainGameEvidence struct {
	Score           float64
	SessionCount    int
	LatestAt        *time.Time
	IndependentRate float64
	TrendDelta14d   float64
}

type planGameSpec struct {
	Game            string
	DisplayName     string
	Reason          string
	StartDifficulty int
	TargetLow       int
	TargetHigh      int
	DailySessions   int
}

func buildCognitiveProfile(userID uint) (cognitiveProfileResp, error) {
	mmseEvidence, mmseCount, latestMMSEAt, err := getRecentMMSEEvidence(userID)
	if err != nil {
		return cognitiveProfileResp{}, err
	}
	gameEvidence, gameSessions21d, latestGameAt, err := getRecentGameEvidence(userID)
	if err != nil {
		return cognitiveProfileResp{}, err
	}

	return assembleCognitiveProfile(
		mmseEvidence,
		mmseCount,
		latestMMSEAt,
		gameEvidence,
		gameSessions21d,
		latestGameAt,
	), nil
}

func assembleCognitiveProfile(
	mmseEvidence map[string]domainMMSEEvidence,
	mmseCount int,
	latestMMSEAt *time.Time,
	gameEvidence map[string]domainGameEvidence,
	gameSessions21d int,
	latestGameAt *time.Time,
) cognitiveProfileResp {
	details := map[string]domainDetail{}
	var overallScore float64
	var overallConfidence float64
	weakDomainCount := 0
	for _, domain := range cognitiveProfileDomains {
		detail := fuseDomainDetail(domain, mmseEvidence[domain], gameEvidence[domain])
		details[domain] = detail
		weight := cognitiveProfileDomainWeights[domain]
		overallScore += detail.Score * weight
		overallConfidence += (detail.Confidence / 100.0) * weight
		if detail.Level == "weak" {
			weakDomainCount++
		}
	}

	priorityDomains := rankPriorityDomains(details)
	needsAssessmentDomains := rankNeedsAssessmentDomains(details)
	finalOverallScore := round1(clampScore(overallScore))
	finalOverallConfidence := round1(clampPercent(overallConfidence * 100))

	return cognitiveProfileResp{
		ProfileVersion:         cognitiveProfileVersion,
		ScoringModel:           cognitiveProfileModel,
		OrientationScore:       details["orientation"].Score,
		MemoryScore:            details["memory"].Score,
		CalculationScore:       details["calculation"].Score,
		RecallScore:            details["recall"].Score,
		LanguageScore:          details["language"].Score,
		OverallScore:           finalOverallScore,
		OverallLevel:           classifyCognitiveOverallLevel(finalOverallScore, weakDomainCount),
		OverallConfidence:      finalOverallConfidence,
		MmseSource:             mmseCount > 0,
		GameSource:             gameSessions21d > 0,
		PriorityDomains:        priorityDomains,
		NeedsAssessmentDomains: needsAssessmentDomains,
		SourceSummary: profileSourceSummary{
			MMSECount:       mmseCount,
			LatestMMSEAt:    latestMMSEAt,
			GameSessions21d: gameSessions21d,
			LatestGameAt:    latestGameAt,
		},
		DomainDetails: details,
	}
}

func getRecentMMSEEvidence(
	userID uint,
) (map[string]domainMMSEEvidence, int, *time.Time, error) {
	result := map[string]domainMMSEEvidence{}
	since := time.Now().AddDate(0, 0, -mmseEvidenceLookbackDays)

	var assessments []models.ScaleAssessment
	if err := global.Db.
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at DESC").
		Limit(mmseEvidenceMaxSamples).
		Find(&assessments).Error; err != nil {
		return result, 0, nil, err
	}
	if len(assessments) == 0 {
		return result, 0, nil, nil
	}

	assessmentIDs := make([]uint, 0, len(assessments))
	for _, assessment := range assessments {
		assessmentIDs = append(assessmentIDs, assessment.ID)
	}

	var moduleScores []models.AssessmentModuleScore
	if err := global.Db.Where("assessment_id IN ?", assessmentIDs).Find(&moduleScores).Error; err != nil {
		return result, 0, nil, err
	}
	if len(moduleScores) == 0 {
		return result, len(assessments), cloneTimePtr(assessmentReferenceTime(assessments[0])), nil
	}

	moduleIDs := make([]uint, 0, len(moduleScores))
	for _, moduleScore := range moduleScores {
		moduleIDs = append(moduleIDs, moduleScore.ModuleID)
	}

	var modules []models.ScaleModule
	if err := global.Db.Where("id IN ?", moduleIDs).Find(&modules).Error; err != nil {
		return result, 0, nil, err
	}

	moduleMap := map[uint]models.ScaleModule{}
	for _, module := range modules {
		moduleMap[module.ID] = module
	}

	assessmentModuleScores := map[uint][]models.AssessmentModuleScore{}
	for _, moduleScore := range moduleScores {
		assessmentModuleScores[moduleScore.AssessmentID] = append(
			assessmentModuleScores[moduleScore.AssessmentID],
			moduleScore,
		)
	}

	weightedScores := map[string]float64{}
	weights := map[string]float64{}
	counts := map[string]int{}
	latestByDomain := map[string]*time.Time{}
	latestOverall := cloneTimePtr(assessmentReferenceTime(assessments[0]))

	for _, assessment := range assessments {
		referenceTime := assessmentReferenceTime(assessment)
		if latestOverall == nil || referenceTime.After(*latestOverall) {
			latestOverall = cloneTimePtr(referenceTime)
		}
		ageDays := time.Since(referenceTime).Hours() / 24
		timeWeight := math.Exp(-ageDays / mmseEvidenceDecayDays)
		seenDomains := map[string]bool{}

		for _, moduleScore := range assessmentModuleScores[assessment.ID] {
			module := moduleMap[moduleScore.ModuleID]
			domain := mmseModuleToDomain[module.Name]
			if domain == "" || module.MaxScore <= 0 {
				continue
			}
			score100 := clampScore(float64(moduleScore.Score) / float64(module.MaxScore) * 100)
			weightedScores[domain] += score100 * timeWeight
			weights[domain] += timeWeight
			seenDomains[domain] = true
			if latestByDomain[domain] == nil || referenceTime.After(*latestByDomain[domain]) {
				latestByDomain[domain] = cloneTimePtr(referenceTime)
			}
		}

		for domain := range seenDomains {
			counts[domain]++
		}
	}

	for _, domain := range cognitiveProfileDomains {
		if weights[domain] <= 0 {
			continue
		}
		result[domain] = domainMMSEEvidence{
			Score:    round1(weightedScores[domain] / weights[domain]),
			Count:    counts[domain],
			LatestAt: latestByDomain[domain],
		}
	}

	return result, len(assessments), latestOverall, nil
}

func getRecentGameEvidence(
	userID uint,
) (map[string]domainGameEvidence, int, *time.Time, error) {
	result := map[string]domainGameEvidence{}
	now := time.Now()
	querySince := now.AddDate(0, 0, -(gameEvidenceLookbackDays + gameTrendWindowDays))
	evidenceSince := now.AddDate(0, 0, -gameEvidenceLookbackDays)
	recentTrendSince := now.AddDate(0, 0, -gameTrendWindowDays)
	previousTrendSince := now.AddDate(0, 0, -(gameTrendWindowDays * 2))

	relevantGames := make([]string, 0, len(gameDifficultyRules))
	for gameName := range gameToDomain {
		relevantGames = append(relevantGames, gameName)
	}

	var resultsRaw []models.GameResult
	if err := global.Db.
		Where("user_id = ? AND created_at >= ? AND game_name IN ?", userID, querySince, relevantGames).
		Order("created_at DESC").
		Find(&resultsRaw).Error; err != nil {
		return result, 0, nil, err
	}
	if len(resultsRaw) == 0 {
		return result, 0, nil, nil
	}

	totalSessions21d := 0
	var latestOverall *time.Time
	for _, gameResult := range resultsRaw {
		if gameResult.CreatedAt.Before(evidenceSince) {
			continue
		}
		totalSessions21d++
		if latestOverall == nil || gameResult.CreatedAt.After(*latestOverall) {
			latestOverall = cloneTimePtr(gameResult.CreatedAt)
		}
	}

	for _, domain := range cognitiveProfileDomains {
		candidates := domainToGameCandidates[domain]
		if len(candidates) == 0 {
			continue
		}

		var weightedScore float64
		var totalWeight float64
		sessionCount := 0
		independentCount := 0
		var latestAt *time.Time
		recentTrendScores := make([]float64, 0)
		previousTrendScores := make([]float64, 0)

		for _, gameResult := range resultsRaw {
			specificityWeight := specificityWeightForDomain(domain, gameResult.GameName)
			if specificityWeight <= 0 {
				continue
			}

			if !gameResult.CreatedAt.Before(recentTrendSince) {
				recentTrendScores = append(recentTrendScores, historicalRehabScore(gameResult))
			} else if !gameResult.CreatedAt.Before(previousTrendSince) {
				previousTrendScores = append(previousTrendScores, historicalRehabScore(gameResult))
			}

			if gameResult.CreatedAt.Before(evidenceSince) {
				continue
			}

			meta := parseMetaString(gameResult.Meta)
			_, _, resolution := extractAssistContext(meta)
			if resolution == "independent" {
				independentCount++
			}

			ageDays := now.Sub(gameResult.CreatedAt).Hours() / 24
			timeWeight := math.Exp(-ageDays / gameEvidenceDecayDays)
			qualityWeight := qualityWeightForResolution(resolution)
			totalWeight += timeWeight * qualityWeight * specificityWeight
			weightedScore += historicalRehabScore(gameResult) * timeWeight * qualityWeight * specificityWeight
			sessionCount++
			if latestAt == nil || gameResult.CreatedAt.After(*latestAt) {
				latestAt = cloneTimePtr(gameResult.CreatedAt)
			}
		}

		if sessionCount == 0 || totalWeight <= 0 {
			continue
		}

		independentRate := 0.0
		if sessionCount > 0 {
			independentRate = float64(independentCount) / float64(sessionCount)
		}
		trendDelta := 0.0
		if len(recentTrendScores) > 0 && len(previousTrendScores) > 0 {
			trendDelta = averageFloat64(recentTrendScores) - averageFloat64(previousTrendScores)
		}

		result[domain] = domainGameEvidence{
			Score:           round1(weightedScore / totalWeight),
			SessionCount:    sessionCount,
			LatestAt:        latestAt,
			IndependentRate: independentRate,
			TrendDelta14d:   round1(trendDelta),
		}
	}

	return result, totalSessions21d, latestOverall, nil
}

func fuseDomainDetail(
	domain string,
	mmse domainMMSEEvidence,
	game domainGameEvidence,
) domainDetail {
	mmseConfidence := mmseConfidenceValue(mmse)
	gameConfidence := gameConfidenceValue(game)
	mmseAvailable := mmse.Count > 0
	gameAvailable := game.SessionCount > 0
	sourceMean := cognitivePriorScore
	combinedConfidence := 0.0
	disagreementPenalty := 0.0
	disagreementFlag := false
	sourceWeight := sourceWeights{}

	switch {
	case mmseAvailable && gameAvailable:
		sourceMean = weightedMean(
			mmse.Score,
			mmseConfidence,
			game.Score,
			gameConfidence,
			cognitivePriorScore,
		)
		disagreement := math.Abs(mmse.Score - game.Score)
		switch {
		case disagreement > 35:
			disagreementPenalty = 0.30
			disagreementFlag = true
		case disagreement > 20:
			disagreementPenalty = 0.15
		}
		combinedConfidence = clampRange(
			0.55*mmseConfidence+0.45*gameConfidence-disagreementPenalty,
			0.15,
			0.95,
		)
		sourceWeight = normalizeSourceWeights(mmseConfidence, gameConfidence)
	case mmseAvailable:
		sourceMean = mmse.Score
		combinedConfidence = clampRange(mmseConfidence, 0, 0.95)
		sourceWeight = sourceWeights{MMSE: 1}
	case gameAvailable:
		sourceMean = game.Score
		combinedConfidence = clampRange(gameConfidence, 0, 0.95)
		sourceWeight = sourceWeights{Game: 1}
	default:
		combinedConfidence = 0
	}

	score := round1(clampScore(cognitivePriorScore + combinedConfidence*(sourceMean-cognitivePriorScore)))
	confidencePercent := round1(clampPercent(combinedConfidence * 100))

	return domainDetail{
		DisplayName:      domainDisplayName[domain],
		Score:            score,
		MmseScore:        round1(mmse.Score),
		GameScore:        round1(game.Score),
		Level:            domainLevelFromScore(score),
		Confidence:       confidencePercent,
		EvidenceStatus:   evidenceStatusFromConfidence(combinedConfidence),
		TrendDelta14d:    round1(game.TrendDelta14d),
		DisagreementFlag: disagreementFlag,
		SourceWeights:    sourceWeight,
		LatestMMSEAt:     mmse.LatestAt,
		LatestGameAt:     game.LatestAt,
		MMSECount:        mmse.Count,
		GameSessions21d:  game.SessionCount,
	}
}

func mmseConfidenceValue(evidence domainMMSEEvidence) float64 {
	if evidence.Count <= 0 || evidence.LatestAt == nil {
		return 0
	}
	ageDays := time.Since(*evidence.LatestAt).Hours() / 24
	freshness := math.Exp(-ageDays / mmseConfidenceDecayDays)
	sampleFactor := math.Min(1, float64(evidence.Count)/2.0)
	return clampRange(freshness*sampleFactor, 0, 1)
}

func gameConfidenceValue(evidence domainGameEvidence) float64 {
	if evidence.SessionCount <= 0 || evidence.LatestAt == nil {
		return 0
	}
	ageDays := time.Since(*evidence.LatestAt).Hours() / 24
	freshness := math.Exp(-ageDays / gameConfidenceDecayDays)
	sampleFactor := math.Min(1, float64(evidence.SessionCount)/6.0)
	qualityFactor := 0.7 + 0.3*evidence.IndependentRate
	return clampRange(freshness*sampleFactor*qualityFactor, 0, 1)
}

func qualityWeightForResolution(resolution string) float64 {
	switch resolution {
	case "corrected_after_hint":
		return 0.85
	case "completed_with_guidance":
		return 0.70
	case "unfinished":
		return 0.60
	default:
		return 1.0
	}
}

func specificityWeightForDomain(domain, gameName string) float64 {
	candidates := domainToGameCandidates[domain]
	for idx, candidate := range candidates {
		if candidate != gameName {
			continue
		}
		if idx == 0 {
			return 1.0
		}
		return 0.75
	}
	return 0
}

func normalizeSourceWeights(mmseWeight, gameWeight float64) sourceWeights {
	total := mmseWeight + gameWeight
	if total <= 0 {
		return sourceWeights{}
	}
	return sourceWeights{
		MMSE: round1(mmseWeight / total),
		Game: round1(gameWeight / total),
	}
}

func weightedMean(aValue, aWeight, bValue, bWeight, fallback float64) float64 {
	totalWeight := aWeight + bWeight
	if totalWeight <= 0 {
		return fallback
	}
	return (aValue*aWeight + bValue*bWeight) / totalWeight
}

func classifyCognitiveOverallLevel(overallScore float64, weakDomainCount int) string {
	switch {
	case overallScore < 45 || weakDomainCount >= 4:
		return "severe"
	case overallScore < 65 || weakDomainCount == 3:
		return "moderate"
	case overallScore < 80 || weakDomainCount == 2:
		return "mild"
	default:
		return "normal"
	}
}

func domainLevelFromScore(score float64) string {
	switch {
	case score >= 78:
		return "good"
	case score >= 55:
		return "fair"
	default:
		return "weak"
	}
}

func evidenceStatusFromConfidence(confidence float64) string {
	switch {
	case confidence >= 0.70:
		return "sufficient"
	case confidence >= 0.35:
		return "limited"
	default:
		return "missing"
	}
}

func rankPriorityDomains(details map[string]domainDetail) []string {
	type domainPriority struct {
		Domain   string
		Priority float64
	}

	list := make([]domainPriority, 0, len(details))
	for _, domain := range cognitiveProfileDomains {
		detail, ok := details[domain]
		if !ok || detail.EvidenceStatus == "missing" {
			continue
		}
		priority := (100-detail.Score)*0.7 + math.Max(0, -detail.TrendDelta14d)*0.3
		list = append(list, domainPriority{
			Domain:   domain,
			Priority: round1(priority),
		})
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority == list[j].Priority {
			return domainIndex(list[i].Domain) < domainIndex(list[j].Domain)
		}
		return list[i].Priority > list[j].Priority
	})

	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, item.Domain)
	}
	return out
}

func rankNeedsAssessmentDomains(details map[string]domainDetail) []string {
	out := make([]string, 0)
	for _, domain := range cognitiveProfileDomains {
		detail, ok := details[domain]
		if ok && detail.EvidenceStatus == "missing" {
			out = append(out, domain)
		}
	}
	return out
}

func domainIndex(domain string) int {
	for idx, item := range cognitiveProfileDomains {
		if item == domain {
			return idx
		}
	}
	return len(cognitiveProfileDomains)
}

func assessmentReferenceTime(assessment models.ScaleAssessment) time.Time {
	if assessment.CompletedAt != nil {
		return *assessment.CompletedAt
	}
	return assessment.CreatedAt
}

func cloneTimePtr(value time.Time) *time.Time {
	copy := value
	return &copy
}

func averageFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func clampRange(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func generateTrainingPlanWithAI(userID uint, profile cognitiveProfileResp) (gin.H, error) {
	weakDomains, recommendedGames, dailyGoal := buildDeterministicTrainingPlanSeed(userID, profile)
	aiSummary, aiModel := enrichTrainingPlanNarrative(profile, weakDomains, recommendedGames, dailyGoal)
	recommendedGames = normalizeTrainingPlanGames(userID, recommendedGames)

	weakJSON, _ := json.Marshal(weakDomains)
	gamesJSON, _ := json.Marshal(recommendedGames)

	plan := models.TrainingPlan{
		UserID:           userID,
		WeakDomains:      string(weakJSON),
		RecommendedGames: string(gamesJSON),
		DailyGoal:        dailyGoal,
		AISummary:        strings.TrimSpace(aiSummary),
		AIModel:          aiModel,
	}
	if err := global.Db.Create(&plan).Error; err != nil {
		return nil, err
	}

	return gin.H{
		"id":                plan.ID,
		"weak_domains":      weakDomains,
		"recommended_games": recommendedGames,
		"daily_goal":        dailyGoal,
		"ai_summary":        plan.AISummary,
		"ai_model":          aiModel,
		"created_at":        plan.CreatedAt,
	}, nil
}

func buildDeterministicTrainingPlanSeed(
	userID uint,
	profile cognitiveProfileResp,
) ([]string, []map[string]any, int) {
	weakDomains := selectWeakDomains(profile)
	needsAssessmentDomains := profile.NeedsAssessmentDomains
	recommendedGames := make([]map[string]any, 0, 3)
	seenGames := map[string]bool{}
	dailyGoal := 0

	for _, domain := range needsAssessmentDomains {
		if len(recommendedGames) >= 3 {
			break
		}
		for _, gameName := range domainToGameCandidates[domain] {
			if seenGames[gameName] {
				continue
			}
			envelope, err := buildGameDifficultyEnvelope(userID, gameName, false)
			if err != nil {
				continue
			}
			recommendedGames = append(recommendedGames, map[string]any{
				"game":                   gameName,
				"display_name":           gameDisplayName[gameName],
				"reason":                 buildCalibrationTrainingReason(domain),
				"start_difficulty":       envelope.Baseline,
				"target_difficulty_low":  envelope.BandLow,
				"target_difficulty_high": envelope.BandHigh,
				"suggested_difficulty":   envelope.Baseline,
				"daily_sessions":         1,
				"plan_mode":              "calibration",
				"target_domain":          domain,
			})
			seenGames[gameName] = true
			dailyGoal++
			break
		}
	}

	for _, domain := range weakDomains {
		if len(recommendedGames) >= 3 {
			break
		}
		score := profile.DomainDetails[domain].Score
		for _, gameName := range domainToGameCandidates[domain] {
			if seenGames[gameName] {
				continue
			}
			envelope, err := buildGameDifficultyEnvelope(userID, gameName, false)
			if err != nil {
				continue
			}
			dailySessions := 1
			if score < 55 {
				dailySessions = 2
			}
			recommendedGames = append(recommendedGames, map[string]any{
				"game":                   gameName,
				"display_name":           gameDisplayName[gameName],
				"reason":                 buildFallbackTrainingReason(gameName, domain),
				"start_difficulty":       envelope.Baseline,
				"target_difficulty_low":  envelope.BandLow,
				"target_difficulty_high": envelope.BandHigh,
				"suggested_difficulty":   envelope.Baseline,
				"daily_sessions":         dailySessions,
				"plan_mode":              "training",
				"target_domain":          domain,
			})
			seenGames[gameName] = true
			dailyGoal += dailySessions
			if len(recommendedGames) >= 3 {
				break
			}
		}
		if len(recommendedGames) >= 3 {
			break
		}
	}

	if len(recommendedGames) == 0 {
		envelope, _ := buildGameDifficultyEnvelope(userID, "schulte_grid", false)
		recommendedGames = append(recommendedGames, map[string]any{
			"game":                   "schulte_grid",
			"display_name":           gameDisplayName["schulte_grid"],
			"reason":                 buildFallbackTrainingReason("schulte_grid", "calculation"),
			"start_difficulty":       envelope.Baseline,
			"target_difficulty_low":  envelope.BandLow,
			"target_difficulty_high": envelope.BandHigh,
			"suggested_difficulty":   envelope.Baseline,
			"daily_sessions":         1,
		})
		dailyGoal = 2
	}

	if dailyGoal < 2 {
		dailyGoal = 2
	}
	if dailyGoal > 5 {
		dailyGoal = 5
	}
	return weakDomains, recommendedGames, dailyGoal
}

func enrichTrainingPlanNarrative(
	profile cognitiveProfileResp,
	weakDomains []string,
	recommendedGames []map[string]any,
	dailyGoal int,
) (string, string) {
	payload, _ := json.Marshal(map[string]any{
		"overall_score":     profile.OverallScore,
		"overall_level":     profile.OverallLevel,
		"weak_domains":      weakDomains,
		"recommended_games": recommendedGames,
		"daily_goal":        dailyGoal,
	})

	messages := []utils.OpenAIMessage{
		{
			Role: "system",
			Content: `你是认知康复训练助手。训练游戏、难度区间和每日次数已经确定，请只补充中文文案，不要修改数值。
请返回严格 JSON：
{
  "summary": "100字以内的整体建议",
  "reasons": [
    {"game": "forward_reverse_numbers", "reason": "20字以内的训练理由"}
  ]
}`,
		},
		{
			Role:    "user",
			Content: string(payload),
		},
	}

	content, aiModel, err := utils.OpenAIChat(messages, 0.3)
	if err != nil {
		return buildFallbackTrainingSummary(profile, weakDomains, dailyGoal), "rule_based"
	}

	var response struct {
		Summary string `json:"summary"`
		Reasons []struct {
			Game   string `json:"game"`
			Reason string `json:"reason"`
		} `json:"reasons"`
	}

	trimmed := strings.TrimSpace(content)
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		start := strings.Index(trimmed, "{")
		end := strings.LastIndex(trimmed, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(trimmed[start:end+1]), &response)
		}
	}

	reasonMap := map[string]string{}
	for _, item := range response.Reasons {
		if strings.TrimSpace(item.Game) == "" || strings.TrimSpace(item.Reason) == "" {
			continue
		}
		reasonMap[item.Game] = strings.TrimSpace(item.Reason)
	}

	for _, game := range recommendedGames {
		name, _ := game["game"].(string)
		if reasonMap[name] != "" {
			game["reason"] = reasonMap[name]
		}
	}

	summary := strings.TrimSpace(response.Summary)
	if summary == "" {
		summary = buildFallbackTrainingSummary(profile, weakDomains, dailyGoal)
	}
	return summary, aiModel
}

func selectWeakDomains(profile cognitiveProfileResp) []string {
	priorityDomains := make([]string, 0, len(profile.PriorityDomains))
	for _, domain := range profile.PriorityDomains {
		if containsString(profile.NeedsAssessmentDomains, domain) {
			continue
		}
		priorityDomains = append(priorityDomains, domain)
		if len(priorityDomains) >= 2 {
			return priorityDomains
		}
	}
	if len(priorityDomains) > 0 {
		return priorityDomains
	}
	if len(profile.NeedsAssessmentDomains) > 0 {
		return append([]string{}, profile.NeedsAssessmentDomains...)
	}
	return []string{"memory", "calculation"}
}

func buildFallbackTrainingReason(gameName, domain string) string {
	if display := domainDisplayName[domain]; display != "" {
		return "重点强化" + display
	}
	if display := gameDisplayName[gameName]; display != "" {
		return "继续巩固" + display
	}
	return "保持稳定训练节奏"
}

func buildCalibrationTrainingReason(domain string) string {
	if display := domainDisplayName[domain]; display != "" {
		return "先做校准训练，补足" + display + "证据"
	}
	return "先做校准训练，补足当前证据"
}

func buildFallbackTrainingSummary(profile cognitiveProfileResp, weakDomains []string, dailyGoal int) string {
	if len(profile.NeedsAssessmentDomains) > 0 {
		labels := make([]string, 0, len(profile.NeedsAssessmentDomains))
		for _, domain := range profile.NeedsAssessmentDomains {
			if name := domainDisplayName[domain]; name != "" {
				labels = append(labels, name)
			}
		}
		if len(labels) > 0 {
			return "当前" + strings.Join(labels, "、") + "证据仍然不足，建议先完成校准训练，再根据稳定表现逐步强化。"
		}
	}
	labels := make([]string, 0, len(weakDomains))
	for _, domain := range weakDomains {
		if name := domainDisplayName[domain]; name != "" {
			labels = append(labels, name)
		}
	}
	if len(labels) == 0 {
		labels = []string{"注意力", "记忆力"}
	}
	return "建议您先围绕" + strings.Join(labels, "、") + "进行训练，每天完成 " +
		strconv.Itoa(dailyGoal) + " 次，先在建议难度区间内稳定表现，再逐步提高。"
}

func normalizeTrainingPlanGames(userID uint, items []map[string]any) []map[string]any {
	normalized := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		gameName, _ := raw["game"].(string)
		if gameName == "" {
			if alt, ok := raw["game_name"].(string); ok {
				gameName = alt
			}
		}
		if gameName == "" {
			continue
		}

		fallback := planGameSpec{
			Game:            gameName,
			DisplayName:     gameDisplayName[gameName],
			StartDifficulty: getGameDifficultyRule(gameName).Default,
			TargetLow:       getGameDifficultyRule(gameName).Default,
			TargetHigh:      getGameDifficultyRule(gameName).Default,
			DailySessions:   1,
		}
		if envelope, err := buildGameDifficultyEnvelope(userID, gameName, false); err == nil {
			fallback.StartDifficulty = envelope.Baseline
			fallback.TargetLow = envelope.BandLow
			fallback.TargetHigh = envelope.BandHigh
		}

		item := applyTrainingGameDefaults(raw, fallback)
		normalized = append(normalized, item)
	}
	return normalized
}

func applyTrainingGameDefaults(item map[string]any, fallback planGameSpec) map[string]any {
	normalized := cloneMetaMap(item)
	gameName, _ := normalized["game"].(string)
	if gameName == "" {
		gameName = fallback.Game
	}
	if gameName == "" {
		return normalized
	}

	start := int(toFloat(normalized["start_difficulty"]))
	if start <= 0 {
		start = int(toFloat(normalized["suggested_difficulty"]))
	}
	if start <= 0 {
		start = fallback.StartDifficulty
	}

	targetLow := int(toFloat(normalized["target_difficulty_low"]))
	if targetLow <= 0 {
		targetLow = fallback.TargetLow
	}
	targetHigh := int(toFloat(normalized["target_difficulty_high"]))
	if targetHigh <= 0 {
		targetHigh = fallback.TargetHigh
	}
	if targetLow > targetHigh {
		targetLow, targetHigh = targetHigh, targetLow
	}

	dailySessions := int(toFloat(normalized["daily_sessions"]))
	if dailySessions <= 0 {
		dailySessions = fallback.DailySessions
		if dailySessions <= 0 {
			dailySessions = 1
		}
	}

	normalized["game"] = gameName
	normalized["display_name"] = valueOrDefaultString(normalized["display_name"], fallback.DisplayName, gameDisplayName[gameName], gameName)
	normalized["reason"] = valueOrDefaultString(normalized["reason"], fallback.Reason, buildFallbackTrainingReason(gameName, gameToDomain[gameName]))
	normalized["start_difficulty"] = start
	normalized["target_difficulty_low"] = targetLow
	normalized["target_difficulty_high"] = targetHigh
	normalized["suggested_difficulty"] = start
	normalized["daily_sessions"] = dailySessions
	return normalized
}

func loadLatestTrainingPlanGameSpec(userID uint, gameName string) (planGameSpec, bool) {
	var plan models.TrainingPlan
	if err := global.Db.Where("user_id = ?", userID).Order("id DESC").First(&plan).Error; err != nil {
		return planGameSpec{}, false
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(plan.RecommendedGames), &items); err != nil {
		return planGameSpec{}, false
	}

	for _, item := range items {
		name, _ := item["game"].(string)
		if name == "" {
			if alt, ok := item["game_name"].(string); ok {
				name = alt
			}
		}
		if name != gameName {
			continue
		}

		rule := getGameDifficultyRule(gameName)
		start := int(toFloat(item["start_difficulty"]))
		if start <= 0 {
			start = int(toFloat(item["suggested_difficulty"]))
		}
		if start <= 0 {
			start = rule.Default
		}
		low := int(toFloat(item["target_difficulty_low"]))
		if low <= 0 {
			low = clampDifficulty(start-1, rule.Min, rule.Max)
		}
		high := int(toFloat(item["target_difficulty_high"]))
		if high <= 0 {
			high = clampDifficulty(start+1, rule.Min, rule.Max)
		}
		if low > high {
			low, high = high, low
		}

		return planGameSpec{
			Game:            gameName,
			DisplayName:     valueOrDefaultString(item["display_name"], gameDisplayName[gameName], gameName),
			Reason:          valueOrDefaultString(item["reason"], buildFallbackTrainingReason(gameName, gameToDomain[gameName])),
			StartDifficulty: start,
			TargetLow:       low,
			TargetHigh:      high,
			DailySessions:   maxInt(1, int(toFloat(item["daily_sessions"]))),
		}, true
	}
	return planGameSpec{}, false
}

func valueOrDefaultString(value any, defaults ...string) string {
	if raw, ok := value.(string); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	for _, fallback := range defaults {
		if strings.TrimSpace(fallback) != "" {
			return strings.TrimSpace(fallback)
		}
	}
	return ""
}

func saveCognitiveProfile(userID uint, profile cognitiveProfileResp) {
	var existing models.CognitiveProfile
	err := global.Db.Where("user_id = ?", userID).First(&existing).Error

	record := models.CognitiveProfile{
		UserID:           userID,
		OrientationScore: profile.OrientationScore,
		MemoryScore:      profile.MemoryScore,
		CalculationScore: profile.CalculationScore,
		RecallScore:      profile.RecallScore,
		LanguageScore:    profile.LanguageScore,
		OverallScore:     profile.OverallScore,
		OverallLevel:     profile.OverallLevel,
		MmseSource:       profile.MmseSource,
		GameSource:       profile.GameSource,
	}

	if err == nil {
		record.ID = existing.ID
		global.Db.Save(&record)
		return
	}
	global.Db.Create(&record)
}

func getMMSETrendPoints(userID uint, since time.Time) []map[string]any {
	var assessments []models.ScaleAssessment
	global.Db.Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at ASC").
		Find(&assessments)

	points := make([]map[string]any, 0, len(assessments))
	for _, assessment := range assessments {
		var moduleScores []models.AssessmentModuleScore
		global.Db.Where("assessment_id = ?", assessment.ID).Find(&moduleScores)

		moduleIDs := make([]uint, 0, len(moduleScores))
		for _, score := range moduleScores {
			moduleIDs = append(moduleIDs, score.ModuleID)
		}

		domainScores := map[string]float64{}
		if len(moduleIDs) > 0 {
			var modules []models.ScaleModule
			global.Db.Where("id IN ?", moduleIDs).Find(&modules)
			moduleMap := map[uint]models.ScaleModule{}
			for _, module := range modules {
				moduleMap[module.ID] = module
			}
			for _, moduleScore := range moduleScores {
				module := moduleMap[moduleScore.ModuleID]
				domain := mmseModuleToDomain[module.Name]
				if domain == "" || module.MaxScore <= 0 {
					continue
				}
				domainScores[domain] = float64(moduleScore.Score) / float64(module.MaxScore) * 100
			}
		}

		points = append(points, map[string]any{
			"date":          assessment.CreatedAt.Format("2006-01-02"),
			"total_score":   assessment.TotalScore,
			"level":         assessment.Level,
			"domain_scores": domainScores,
		})
	}
	return points
}

func getGameTrendPoints(userID uint, since time.Time) []map[string]any {
	var results []models.GameResult
	global.Db.Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at ASC").
		Find(&results)

	type dayData struct {
		sessions       int
		totalRehab     float64
		domainRehabMap map[string][]float64
	}

	dayMap := map[string]*dayData{}
	for _, result := range results {
		day := result.CreatedAt.Format("2006-01-02")
		if dayMap[day] == nil {
			dayMap[day] = &dayData{domainRehabMap: map[string][]float64{}}
		}

		rehabScore := historicalRehabScore(result)
		entry := dayMap[day]
		entry.sessions++
		entry.totalRehab += rehabScore

		domain := gameToDomain[result.GameName]
		if domain != "" {
			entry.domainRehabMap[domain] = append(entry.domainRehabMap[domain], rehabScore)
		}
	}

	days := make([]string, 0, len(dayMap))
	for day := range dayMap {
		days = append(days, day)
	}
	sort.Strings(days)

	points := make([]map[string]any, 0, len(days))
	for _, day := range days {
		entry := dayMap[day]
		if entry == nil || entry.sessions == 0 {
			continue
		}

		avgRehab := round1(entry.totalRehab / float64(entry.sessions))
		domainRehab := map[string]float64{}
		domainAccuracy := map[string]float64{}
		for domain, scores := range entry.domainRehabMap {
			var sum float64
			for _, score := range scores {
				sum += score
			}
			avg := round1(sum / float64(len(scores)))
			domainRehab[domain] = avg
			domainAccuracy[domain] = math.Round((avg/100)*1000) / 1000
		}

		points = append(points, map[string]any{
			"date":               day,
			"sessions":           entry.sessions,
			"avg_rehab_score":    avgRehab,
			"avg_accuracy":       math.Round((avgRehab/100)*1000) / 1000,
			"domain_rehab_score": domainRehab,
			"domain_accuracy":    domainAccuracy,
		})
	}
	return points
}

func buildCognitiveTrendPayload(days int, mmsePoints, gamePoints []map[string]any) gin.H {
	return gin.H{
		"days":         days,
		"mmse_trend":   mmsePoints,
		"game_trend":   gamePoints,
		"points":       buildOverviewTrendPoints(mmsePoints, gamePoints),
		"mmse_history": mmsePoints,
	}
}

func buildOverviewTrendPoints(mmsePoints, gamePoints []map[string]any) []map[string]any {
	type aggregate struct {
		sum   float64
		count int
	}

	byDate := map[string]*aggregate{}
	for _, point := range mmsePoints {
		date, _ := point["date"].(string)
		totalScore := toFloat(point["total_score"])
		if date == "" || totalScore <= 0 {
			continue
		}
		score := clampScore(totalScore / 30 * 100)
		if byDate[date] == nil {
			byDate[date] = &aggregate{}
		}
		byDate[date].sum += score
		byDate[date].count++
	}

	for _, point := range gamePoints {
		date, _ := point["date"].(string)
		avgAccuracy := normalizeAccuracy(toFloat(point["avg_accuracy"]))
		if date == "" || avgAccuracy <= 0 {
			continue
		}
		score := clampScore(avgAccuracy * 100)
		if byDate[date] == nil {
			byDate[date] = &aggregate{}
		}
		byDate[date].sum += score
		byDate[date].count++
	}

	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	points := make([]map[string]any, 0, len(dates))
	for _, date := range dates {
		agg := byDate[date]
		if agg == nil || agg.count == 0 {
			continue
		}
		points = append(points, map[string]any{
			"date":  date,
			"score": round1(agg.sum / float64(agg.count)),
		})
	}
	return points
}

func buildTrainingSummaryPayload(days int, results []models.GameResult) gin.H {
	totalSessions := len(results)
	totalDurationMs := 0
	totalRehabScore := 0.0
	dailyMap := map[string]int{}

	type gameSummaryAggregate struct {
		count       int
		rehabScore  float64
		assistCount int
		assistLevel float64
	}

	gameSummaryMap := map[string]*gameSummaryAggregate{}
	totalAssistCount := 0
	totalAssistLevel := 0.0
	for _, result := range results {
		totalDurationMs += result.DurationMs
		rehabScore := historicalRehabScore(result)
		totalRehabScore += rehabScore
		meta := parseMetaString(result.Meta)
		assistTriggered, assistLevel, _ := extractAssistContext(meta)
		if assistTriggered {
			totalAssistCount++
			totalAssistLevel += float64(assistLevel)
		}

		day := result.CreatedAt.Format("2006-01-02")
		dailyMap[day]++

		if gameSummaryMap[result.GameName] == nil {
			gameSummaryMap[result.GameName] = &gameSummaryAggregate{}
		}
		gameSummaryMap[result.GameName].count++
		gameSummaryMap[result.GameName].rehabScore += rehabScore
		if assistTriggered {
			gameSummaryMap[result.GameName].assistCount++
			gameSummaryMap[result.GameName].assistLevel += float64(assistLevel)
		}
	}

	avgRehabScore := 0.0
	if totalSessions > 0 {
		avgRehabScore = round1(totalRehabScore / float64(totalSessions))
	}
	avgAccuracyCompat := math.Round((avgRehabScore/100)*1000) / 1000
	assistRate := 0.0
	if totalSessions > 0 {
		assistRate = math.Round((float64(totalAssistCount)/float64(totalSessions))*1000) / 1000
	}
	avgAssistLevel := 0.0
	if totalAssistCount > 0 {
		avgAssistLevel = round1(totalAssistLevel / float64(totalAssistCount))
	}

	dailyBreakdown := make([]map[string]any, 0, len(dailyMap))
	for date, sessions := range dailyMap {
		dailyBreakdown = append(dailyBreakdown, map[string]any{
			"date":     date,
			"sessions": sessions,
		})
	}
	sort.Slice(dailyBreakdown, func(i, j int) bool {
		return dailyBreakdown[i]["date"].(string) < dailyBreakdown[j]["date"].(string)
	})

	gameSummary := make([]map[string]any, 0, len(gameSummaryMap))
	for gameName, aggregate := range gameSummaryMap {
		displayName := gameDisplayName[gameName]
		if displayName == "" {
			displayName = gameName
		}
		avgGameRehab := 0.0
		if aggregate.count > 0 {
			avgGameRehab = round1(aggregate.rehabScore / float64(aggregate.count))
		}
		gameAssistRate := 0.0
		if aggregate.count > 0 {
			gameAssistRate = math.Round((float64(aggregate.assistCount)/float64(aggregate.count))*1000) / 1000
		}
		gameAvgAssistLevel := 0.0
		if aggregate.assistCount > 0 {
			gameAvgAssistLevel = round1(aggregate.assistLevel / float64(aggregate.assistCount))
		}
		gameSummary = append(gameSummary, map[string]any{
			"game_name":        gameName,
			"display_name":     displayName,
			"sessions":         aggregate.count,
			"count":            aggregate.count,
			"avg_rehab_score":  avgGameRehab,
			"avg_accuracy":     math.Round((avgGameRehab/100)*1000) / 1000,
			"assist_sessions":  aggregate.assistCount,
			"assist_rate":      gameAssistRate,
			"avg_assist_level": gameAvgAssistLevel,
		})
	}
	sort.Slice(gameSummary, func(i, j int) bool {
		return gameSummary[i]["display_name"].(string) < gameSummary[j]["display_name"].(string)
	})

	totalDurationMinutes := math.Round((float64(totalDurationMs)/60000)*10) / 10

	return gin.H{
		"days":                   days,
		"total_sessions":         totalSessions,
		"active_days":            len(dailyMap),
		"total_duration_ms":      totalDurationMs,
		"total_duration_minutes": totalDurationMinutes,
		"avg_rehab_score":        avgRehabScore,
		"average_rehab_score":    avgRehabScore,
		"avg_accuracy":           avgAccuracyCompat,
		"average_accuracy":       avgAccuracyCompat,
		"assist_sessions":        totalAssistCount,
		"assist_rate":            assistRate,
		"avg_assist_level":       avgAssistLevel,
		"daily_breakdown":        dailyBreakdown,
		"game_summary":           gameSummary,
		"game_breakdown":         gameSummary,
	}
}

func toFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case uint:
		return float64(v)
	case uint64:
		return float64(v)
	default:
		return 0
	}
}

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
