package controllers

import (
	"encoding/json"
	"strings"
	"testing"

	"ccrt_sever/models"
)

func TestApplyTrainingGameDefaultsPreservesPlanDifficulty(t *testing.T) {
	merged := applyTrainingGameDefaults(map[string]any{
		"game":                   "spot_difference",
		"start_difficulty":       3,
		"target_difficulty_low":  2,
		"target_difficulty_high": 4,
	}, planGameSpec{})

	if merged["start_difficulty"] != 3 || merged["target_difficulty_low"] != 2 || merged["target_difficulty_high"] != 4 {
		t.Fatalf("expected plan defaults to be preserved, got %#v", merged)
	}
}

func TestComputeRehabScoreFallsBackWithoutTaskMeta(t *testing.T) {
	score, breakdown := computeRehabScore("leaf_attention", models.GameResult{
		GameName:   "leaf_attention",
		Difficulty: 3,
		DurationMs: 60000,
		Accuracy:   0.8,
		Success:    true,
	}, nil)

	if breakdown.Accuracy != 80 || breakdown.Completion != 100 || breakdown.TimeEfficiency != 100 || breakdown.Stability != 94 {
		t.Fatalf("unexpected breakdown: %#v", breakdown)
	}
	if score != 88.4 {
		t.Fatalf("expected rehab score 88.4, got %#v", score)
	}
}

func TestComputeNextDifficultyFromHistoryPromotesOnHighStreak(t *testing.T) {
	history := []models.GameResult{
		buildHistoryResult(2, true, 90),
		buildHistoryResult(2, true, 88),
	}

	next, ewma, highStreak, lowStreak, reason := computeNextDifficultyFromHistory(2, history, 1, 3, 1, 5)

	if next != 3 {
		t.Fatalf("expected next difficulty 3, got %d", next)
	}
	if ewma < 85 || highStreak != 2 || lowStreak != 0 {
		t.Fatalf("unexpected stats: ewma=%v high=%d low=%d", ewma, highStreak, lowStreak)
	}
	if !strings.Contains(reason, "上调") {
		t.Fatalf("expected promotion reason, got %q", reason)
	}
}

func TestComputeNextDifficultyFromHistoryDropsOnLowScore(t *testing.T) {
	history := []models.GameResult{
		buildHistoryResult(3, true, 55),
		buildHistoryResult(3, false, 58),
	}

	next, _, _, lowStreak, reason := computeNextDifficultyFromHistory(3, history, 2, 4, 1, 5)

	if next != 2 {
		t.Fatalf("expected next difficulty 2, got %d", next)
	}
	if lowStreak != 2 {
		t.Fatalf("expected low streak 2, got %d", lowStreak)
	}
	if !strings.Contains(reason, "下调") {
		t.Fatalf("expected downgrade reason, got %q", reason)
	}
}

func TestComputeNextDifficultyFromHistoryHoldsAfterHintedSuccess(t *testing.T) {
	meta, _ := json.Marshal(map[string]any{
		"rehab_score":       92,
		"assist_triggered":  true,
		"assist_level_max":  2,
		"assist_resolution": "corrected_after_hint",
	})
	history := []models.GameResult{
		{
			GameName:   "schulte_grid",
			Difficulty: 2,
			Success:    true,
			Meta:       string(meta),
		},
		buildHistoryResult(2, true, 90),
	}

	next, _, highStreak, _, reason := computeNextDifficultyFromHistory(2, history, 1, 3, 1, 5)

	if next != 2 {
		t.Fatalf("expected hinted round to hold difficulty at 2, got %d", next)
	}
	if highStreak != 2 {
		t.Fatalf("expected high streak 2, got %d", highStreak)
	}
	if !strings.Contains(reason, "保持当前难度") {
		t.Fatalf("expected assist hold reason, got %q", reason)
	}
}

func TestComputeNextDifficultyFromHistoryDropsAfterGuidedRound(t *testing.T) {
	meta, _ := json.Marshal(map[string]any{
		"rehab_score":       72,
		"assist_triggered":  true,
		"assist_level_max":  2,
		"assist_resolution": "completed_with_guidance",
	})
	history := []models.GameResult{
		{
			GameName:   "schulte_grid",
			Difficulty: 4,
			Success:    false,
			Meta:       string(meta),
		},
		buildHistoryResult(4, true, 88),
	}

	next, _, _, _, reason := computeNextDifficultyFromHistory(4, history, 2, 5, 1, 5)

	if next != 3 {
		t.Fatalf("expected guided round to drop difficulty to 3, got %d", next)
	}
	if !strings.Contains(reason, "放缓一档") {
		t.Fatalf("expected guided downgrade reason, got %q", reason)
	}
}

func buildHistoryResult(difficulty int, success bool, rehabScore float64) models.GameResult {
	meta, _ := json.Marshal(map[string]any{"rehab_score": rehabScore})
	return models.GameResult{
		GameName:   "schulte_grid",
		Difficulty: difficulty,
		Success:    success,
		Meta:       string(meta),
	}
}
