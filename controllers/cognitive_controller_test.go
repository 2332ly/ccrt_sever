package controllers

import (
	"encoding/json"
	"testing"
	"time"

	"ccrt_sever/models"
	"gorm.io/gorm"
)

func TestBuildCognitiveTrendPayloadProvidesCompatibilityFields(t *testing.T) {
	mmsePoints := []map[string]any{
		{
			"date":        "2026-03-01",
			"total_score": 27,
		},
	}
	gamePoints := []map[string]any{
		{
			"date":         "2026-03-01",
			"avg_accuracy": 0.8,
		},
		{
			"date":         "2026-03-02",
			"avg_accuracy": 0.5,
		},
	}

	payload := buildCognitiveTrendPayload(7, mmsePoints, gamePoints)

	if payload["days"] != 7 {
		t.Fatalf("expected days=7, got %#v", payload["days"])
	}

	mmseHistory, ok := payload["mmse_history"].([]map[string]any)
	if !ok || len(mmseHistory) != 1 {
		t.Fatalf("expected mmse_history compatibility field, got %#v", payload["mmse_history"])
	}

	points, ok := payload["points"].([]map[string]any)
	if !ok || len(points) != 2 {
		t.Fatalf("expected 2 overview points, got %#v", payload["points"])
	}

	if points[0]["date"] != "2026-03-01" || points[0]["score"] != 85.0 {
		t.Fatalf("unexpected first overview point: %#v", points[0])
	}
	if points[1]["date"] != "2026-03-02" || points[1]["score"] != 50.0 {
		t.Fatalf("unexpected second overview point: %#v", points[1])
	}
}

func TestBuildTrainingSummaryPayloadProvidesCompatibilityFields(t *testing.T) {
	assistL2 := mustJSONMeta(t, map[string]any{
		"assist_triggered":  true,
		"assist_level_max":  2,
		"assist_resolution": "corrected_after_hint",
	})
	assistL1 := mustJSONMeta(t, map[string]any{
		"assist_triggered":  true,
		"assist_level_max":  1,
		"assist_resolution": "unfinished",
	})
	results := []models.GameResult{
		{
			Model:      gorm.Model{CreatedAt: time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local)},
			GameName:   "schulte_grid",
			DurationMs: 60000,
			Accuracy:   0.8,
			Meta:       assistL2,
		},
		{
			Model:      gorm.Model{CreatedAt: time.Date(2026, 3, 1, 11, 0, 0, 0, time.Local)},
			GameName:   "schulte_grid",
			DurationMs: 120000,
			Accuracy:   80,
		},
		{
			Model:      gorm.Model{CreatedAt: time.Date(2026, 3, 2, 10, 0, 0, 0, time.Local)},
			GameName:   "leaf_attention",
			DurationMs: 30000,
			Accuracy:   0.5,
			Meta:       assistL1,
		},
	}

	payload := buildTrainingSummaryPayload(7, results)

	if payload["total_sessions"] != 3 {
		t.Fatalf("expected total_sessions=3, got %#v", payload["total_sessions"])
	}
	if payload["active_days"] != 2 {
		t.Fatalf("expected active_days=2, got %#v", payload["active_days"])
	}
	if payload["total_duration_ms"] != 210000 {
		t.Fatalf("expected total_duration_ms=210000, got %#v", payload["total_duration_ms"])
	}
	if payload["total_duration_minutes"] != 3.5 {
		t.Fatalf("expected total_duration_minutes=3.5, got %#v", payload["total_duration_minutes"])
	}
	if payload["avg_rehab_score"] != 70.4 || payload["average_rehab_score"] != 70.4 {
		t.Fatalf("expected rehab score fields, got avg=%#v average=%#v", payload["avg_rehab_score"], payload["average_rehab_score"])
	}
	if payload["avg_accuracy"] != 0.704 || payload["average_accuracy"] != 0.704 {
		t.Fatalf("expected compatibility accuracy fields, got avg=%#v average=%#v", payload["avg_accuracy"], payload["average_accuracy"])
	}
	if payload["assist_sessions"] != 2 {
		t.Fatalf("expected assist_sessions=2, got %#v", payload["assist_sessions"])
	}
	if payload["assist_rate"] != 0.667 {
		t.Fatalf("expected assist_rate=0.667, got %#v", payload["assist_rate"])
	}
	if payload["avg_assist_level"] != 1.5 {
		t.Fatalf("expected avg_assist_level=1.5, got %#v", payload["avg_assist_level"])
	}

	gameSummary, ok := payload["game_summary"].([]map[string]any)
	if !ok || len(gameSummary) != 2 {
		t.Fatalf("expected game_summary rows, got %#v", payload["game_summary"])
	}
	gameBreakdown, ok := payload["game_breakdown"].([]map[string]any)
	if !ok || len(gameBreakdown) != 2 {
		t.Fatalf("expected game_breakdown compatibility rows, got %#v", payload["game_breakdown"])
	}

	schulte := findGameSummaryRow(gameSummary, "schulte_grid")
	if schulte == nil {
		t.Fatalf("expected schulte_grid row in %#v", gameSummary)
	}
	if schulte["sessions"] != 2 || schulte["count"] != 2 || schulte["avg_accuracy"] != 0.751 || schulte["avg_rehab_score"] != 75.1 {
		t.Fatalf("unexpected schulte_grid summary row: %#v", schulte)
	}
	if schulte["assist_sessions"] != 1 || schulte["assist_rate"] != 0.5 || schulte["avg_assist_level"] != 2.0 {
		t.Fatalf("unexpected schulte_grid assist row: %#v", schulte)
	}

	leaf := findGameSummaryRow(gameSummary, "leaf_attention")
	if leaf == nil {
		t.Fatalf("expected leaf_attention row in %#v", gameSummary)
	}
	if leaf["assist_sessions"] != 1 || leaf["assist_rate"] != 1.0 || leaf["avg_assist_level"] != 1.0 {
		t.Fatalf("unexpected leaf_attention assist row: %#v", leaf)
	}
}

func TestApplyTrainingGameDefaultsAddsBandAndCompatibilityFields(t *testing.T) {
	item := map[string]any{
		"game":   "forward_reverse_numbers",
		"reason": "强化工作记忆",
	}

	normalized := applyTrainingGameDefaults(item, planGameSpec{
		Game:            "forward_reverse_numbers",
		DisplayName:     "正倒序数字",
		StartDifficulty: 3,
		TargetLow:       2,
		TargetHigh:      4,
		DailySessions:   2,
	})

	if normalized["start_difficulty"] != 3 || normalized["suggested_difficulty"] != 3 {
		t.Fatalf("expected start and compatibility difficulty fields, got %#v", normalized)
	}
	if normalized["target_difficulty_low"] != 2 || normalized["target_difficulty_high"] != 4 {
		t.Fatalf("expected target difficulty band, got %#v", normalized)
	}
	if normalized["daily_sessions"] != 2 {
		t.Fatalf("expected daily_sessions=2, got %#v", normalized["daily_sessions"])
	}
}

func TestAssembleCognitiveProfileMMSEOnlyKeepsHundredScaleFields(t *testing.T) {
	now := time.Now()
	profile := assembleCognitiveProfile(
		map[string]domainMMSEEvidence{
			"orientation": freshMMSEEvidence(82, 2),
			"memory":      freshMMSEEvidence(70, 2),
			"calculation": freshMMSEEvidence(60, 2),
			"recall":      freshMMSEEvidence(75, 2),
			"language":    freshMMSEEvidence(88, 2),
		},
		2,
		cloneTimePtr(now),
		nil,
		0,
		nil,
	)

	if !profile.MmseSource || profile.GameSource {
		t.Fatalf("expected MMSE-only profile sources, got mmse=%v game=%v", profile.MmseSource, profile.GameSource)
	}
	if profile.ProfileVersion != cognitiveProfileVersion || profile.ScoringModel != cognitiveProfileModel {
		t.Fatalf("expected version/model fields, got version=%q model=%q", profile.ProfileVersion, profile.ScoringModel)
	}
	if profile.OverallScore < 70 || profile.OverallScore > 80 {
		t.Fatalf("expected overall score to stay in 0-100 range with MMSE-only evidence, got %.1f", profile.OverallScore)
	}
	if profile.OverallConfidence != 95 {
		t.Fatalf("expected overall confidence=95, got %.1f", profile.OverallConfidence)
	}
	if len(profile.NeedsAssessmentDomains) != 0 {
		t.Fatalf("expected no needs_assessment domains, got %#v", profile.NeedsAssessmentDomains)
	}
	if len(profile.PriorityDomains) == 0 || profile.PriorityDomains[0] != "calculation" {
		t.Fatalf("expected calculation to be highest priority, got %#v", profile.PriorityDomains)
	}

	calculation := profile.DomainDetails["calculation"]
	if calculation.EvidenceStatus != "sufficient" || calculation.MMSECount != 2 || calculation.GameSessions21d != 0 {
		t.Fatalf("unexpected calculation detail: %#v", calculation)
	}
	if calculation.MmseScore != 60 || calculation.GameScore != 0 {
		t.Fatalf("expected MMSE/game source scores to remain visible, got %#v", calculation)
	}
}

func TestAssembleCognitiveProfileSparseGamesShrinkTowardNeutralPrior(t *testing.T) {
	now := time.Now()
	profile := assembleCognitiveProfile(
		nil,
		0,
		nil,
		map[string]domainGameEvidence{
			"memory": freshGameEvidence(20, 1, 0, -12),
		},
		1,
		cloneTimePtr(now),
	)

	if profile.MmseSource || !profile.GameSource {
		t.Fatalf("expected game-only profile sources, got mmse=%v game=%v", profile.MmseSource, profile.GameSource)
	}
	if profile.OverallScore <= 50 || profile.OverallScore >= 55 {
		t.Fatalf("expected sparse game evidence to shrink toward prior instead of collapsing, got %.1f", profile.OverallScore)
	}
	if profile.OverallConfidence >= 5 {
		t.Fatalf("expected very low overall confidence for sparse game evidence, got %.1f", profile.OverallConfidence)
	}
	if len(profile.PriorityDomains) != 0 {
		t.Fatalf("expected no priority domains when all evidence is missing, got %#v", profile.PriorityDomains)
	}
	if len(profile.NeedsAssessmentDomains) != len(cognitiveProfileDomains) {
		t.Fatalf("expected all domains to need assessment, got %#v", profile.NeedsAssessmentDomains)
	}

	memory := profile.DomainDetails["memory"]
	if memory.Score <= 45 || memory.Score >= 55 {
		t.Fatalf("expected sparse game score to stay near the 55 prior, got %.1f", memory.Score)
	}
	if memory.EvidenceStatus != "missing" || memory.GameSessions21d != 1 {
		t.Fatalf("unexpected sparse game detail: %#v", memory)
	}
}

func TestAssembleCognitiveProfileAlignedEvidenceBuildsPriorityDomains(t *testing.T) {
	now := time.Now()
	profile := assembleCognitiveProfile(
		map[string]domainMMSEEvidence{
			"orientation": freshMMSEEvidence(82, 2),
			"memory":      freshMMSEEvidence(66, 2),
			"calculation": freshMMSEEvidence(52, 2),
			"recall":      freshMMSEEvidence(75, 2),
			"language":    freshMMSEEvidence(88, 2),
		},
		2,
		cloneTimePtr(now),
		map[string]domainGameEvidence{
			"orientation": freshGameEvidence(80, 6, 1, 2),
			"memory":      freshGameEvidence(68, 6, 1, 1),
			"calculation": freshGameEvidence(50, 6, 1, -8),
			"recall":      freshGameEvidence(74, 6, 1, 3),
			"language":    freshGameEvidence(86, 6, 1, 2),
		},
		30,
		cloneTimePtr(now),
	)

	if profile.OverallScore < 65 || profile.OverallScore > 85 {
		t.Fatalf("expected aligned evidence to produce a stable 0-100 score, got %.1f", profile.OverallScore)
	}
	if profile.OverallConfidence < 90 {
		t.Fatalf("expected high confidence for aligned dual-source evidence, got %.1f", profile.OverallConfidence)
	}
	if len(profile.NeedsAssessmentDomains) != 0 {
		t.Fatalf("expected no needs_assessment domains, got %#v", profile.NeedsAssessmentDomains)
	}
	if len(profile.PriorityDomains) == 0 || profile.PriorityDomains[0] != "calculation" {
		t.Fatalf("expected weak declining calculation domain to rank first, got %#v", profile.PriorityDomains)
	}

	calculation := profile.DomainDetails["calculation"]
	if calculation.Level != "weak" || calculation.DisagreementFlag {
		t.Fatalf("unexpected aligned calculation detail: %#v", calculation)
	}
	if calculation.EvidenceStatus != "sufficient" {
		t.Fatalf("expected aligned evidence to be sufficient, got %#v", calculation)
	}
}

func TestAssembleCognitiveProfileWithoutEvidenceReturnsNeutralProfile(t *testing.T) {
	profile := assembleCognitiveProfile(nil, 0, nil, nil, 0, nil)

	if profile.OverallScore != 55 || profile.OverallConfidence != 0 {
		t.Fatalf("expected neutral profile for missing evidence, got score=%.1f confidence=%.1f", profile.OverallScore, profile.OverallConfidence)
	}
	if len(profile.PriorityDomains) != 0 {
		t.Fatalf("expected no priority domains, got %#v", profile.PriorityDomains)
	}
	if len(profile.NeedsAssessmentDomains) != len(cognitiveProfileDomains) {
		t.Fatalf("expected all domains to need assessment, got %#v", profile.NeedsAssessmentDomains)
	}
	for _, domain := range cognitiveProfileDomains {
		detail := profile.DomainDetails[domain]
		if detail.Score != 55 || detail.EvidenceStatus != "missing" {
			t.Fatalf("expected neutral missing detail for %s, got %#v", domain, detail)
		}
	}
}

func TestFuseDomainDetailFlagsSevereDisagreement(t *testing.T) {
	detail := fuseDomainDetail(
		"orientation",
		freshMMSEEvidence(90, 2),
		freshGameEvidence(40, 6, 1, 0),
	)

	if !detail.DisagreementFlag {
		t.Fatalf("expected disagreement flag, got %#v", detail)
	}
	if detail.EvidenceStatus != "sufficient" {
		t.Fatalf("expected disagreement to keep a usable but penalized evidence status, got %#v", detail)
	}
	if detail.Confidence < 68 || detail.Confidence > 70 {
		t.Fatalf("expected confidence penalty after disagreement, got %.1f", detail.Confidence)
	}
	if detail.Score < 58 || detail.Score > 65 {
		t.Fatalf("expected disagreement score to be shrunk toward the prior, got %.1f", detail.Score)
	}
}

func TestFuseDomainDetailAssistedGamesReduceConfidence(t *testing.T) {
	independent := fuseDomainDetail(
		"memory",
		domainMMSEEvidence{},
		freshGameEvidence(85, 6, 1, 0),
	)
	assisted := fuseDomainDetail(
		"memory",
		domainMMSEEvidence{},
		freshGameEvidence(85, 6, 0, 0),
	)

	if assisted.Confidence >= independent.Confidence {
		t.Fatalf("expected assisted sessions to reduce confidence, got assisted=%.1f independent=%.1f", assisted.Confidence, independent.Confidence)
	}
	if assisted.Score <= 55 || assisted.Score >= 85 {
		t.Fatalf("expected assisted game score to remain in shrunk 0-100 range, got %.1f", assisted.Score)
	}
	if assisted.GameSessions21d != 6 || independent.GameSessions21d != 6 {
		t.Fatalf("expected game session counts to be preserved, got assisted=%#v independent=%#v", assisted, independent)
	}
}

func findGameSummaryRow(rows []map[string]any, gameName string) map[string]any {
	for _, row := range rows {
		if row["game_name"] == gameName {
			return row
		}
	}
	return nil
}

func mustJSONMeta(t *testing.T, meta map[string]any) string {
	t.Helper()
	bytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	return string(bytes)
}

func freshMMSEEvidence(score float64, count int) domainMMSEEvidence {
	now := time.Now()
	return domainMMSEEvidence{
		Score:    score,
		Count:    count,
		LatestAt: cloneTimePtr(now),
	}
}

func freshGameEvidence(score float64, sessions int, independentRate, trendDelta float64) domainGameEvidence {
	now := time.Now()
	return domainGameEvidence{
		Score:           score,
		SessionCount:    sessions,
		LatestAt:        cloneTimePtr(now),
		IndependentRate: independentRate,
		TrendDelta14d:   trendDelta,
	}
}
