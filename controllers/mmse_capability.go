package controllers

import (
	"strings"

	"ccrt_sever/models"

	"github.com/gin-gonic/gin"
)

type mmseCapabilityDefaults struct {
	InputModes          []string
	RequiredPermissions []string
	AutoScore           any
	WarmupTags          []string
	FallbackMode        string
	Stimulus            any
	SupportsGlobalVoice bool
	CameraProfile       string
	PreferredLens       string
	ROIPreset           string
	ExpectedLocation    []string
}

func buildCapabilityOptions(q models.ScaleQuestion, rule map[string]any) gin.H {
	ruleType, _ := rule["type"].(string)
	defaults := inferMMSECapabilityDefaults(q, strings.TrimSpace(ruleType))

	options := gin.H{
		"input_modes":          coalesceStringSlice(rule["input_modes"], defaults.InputModes),
		"required_permissions": coalesceStringSlice(rule["required_permissions"], defaults.RequiredPermissions),
		"warmup_tags":          coalesceStringSlice(rule["warmup_tags"], defaults.WarmupTags),
		"fallback_mode":        coalesceString(rule["fallback_mode"], defaults.FallbackMode),
		"supports_global_voice": coalesceBool(
			rule["supports_global_voice"],
			defaults.SupportsGlobalVoice,
		),
	}

	if cameraProfile := coalesceString(rule["camera_profile"], defaults.CameraProfile); cameraProfile != "" {
		options["camera_profile"] = cameraProfile
	}
	if preferredLens := coalesceString(rule["preferred_lens"], defaults.PreferredLens); preferredLens != "" {
		options["preferred_lens"] = preferredLens
	}
	if roiPreset := coalesceString(rule["roi_preset"], defaults.ROIPreset); roiPreset != "" {
		options["roi_preset"] = roiPreset
	}
	if expectedLocation := coalesceStringSlice(
		rule["expected_location_context_fields"],
		defaults.ExpectedLocation,
	); len(expectedLocation) > 0 {
		options["expected_location_context_fields"] = expectedLocation
	}

	if stimulus, ok := rule["stimulus"]; ok && stimulus != nil {
		options["stimulus"] = stimulus
	} else if defaults.Stimulus != nil {
		options["stimulus"] = defaults.Stimulus
	}

	if autoScore := normalizeAutoScore(rule["auto_score"]); autoScore != nil {
		options["auto_score"] = autoScore
	} else if defaults.AutoScore != nil {
		options["auto_score"] = defaults.AutoScore
	}

	return options
}

func inferMMSECapabilityDefaults(
	q models.ScaleQuestion,
	ruleType string,
) mmseCapabilityDefaults {
	content := strings.TrimSpace(q.Content)
	defaults := mmseCapabilityDefaults{
		InputModes:          []string{"manual"},
		RequiredPermissions: nil,
		AutoScore:           gin.H{"kind": "manual_review"},
		WarmupTags:          []string{"standard_assessment"},
		FallbackMode:        "caregiver_confirm",
		SupportsGlobalVoice: false,
	}

	switch ruleType {
	case "fields_correct":
		autoKind := "location_orientation"
		stimulusLabel := "地点定向"
		if strings.Contains(content, "年份") || strings.Contains(content, "季节") {
			autoKind = "date_time_orientation"
			stimulusLabel = "时间定向"
		}
		defaults.InputModes = []string{"voice"}
		defaults.RequiredPermissions = []string{"microphone"}
		defaults.AutoScore = gin.H{"kind": autoKind}
		defaults.Stimulus = gin.H{"label": stimulusLabel}
		defaults.WarmupTags = []string{"daily_warmup", "standard_assessment"}
		defaults.SupportsGlobalVoice = true
		if autoKind == "location_orientation" {
			defaults.ExpectedLocation = []string{"city", "district", "street", "place", "floor"}
		}
	case "set_match":
		defaults.InputModes = []string{"voice"}
		defaults.RequiredPermissions = []string{"microphone"}
		defaults.AutoScore = gin.H{"kind": "set_match"}
		defaults.Stimulus = gin.H{"label": "词语记忆"}
		defaults.WarmupTags = []string{"daily_warmup", "standard_assessment"}
		defaults.SupportsGlobalVoice = true
	case "sequence_match":
		defaults.InputModes = []string{"voice", "keypad"}
		defaults.RequiredPermissions = []string{"microphone"}
		defaults.AutoScore = gin.H{"kind": "sequence_match"}
		defaults.Stimulus = gin.H{"label": "连减 7"}
		defaults.WarmupTags = []string{"daily_warmup", "standard_assessment"}
		defaults.SupportsGlobalVoice = true
	case "exact_text":
		defaults.InputModes = []string{"voice"}
		defaults.RequiredPermissions = []string{"microphone"}
		defaults.AutoScore = gin.H{"kind": "exact_text"}
		defaults.Stimulus = gin.H{"label": "复述句子"}
		defaults.WarmupTags = []string{"daily_warmup", "standard_assessment"}
		defaults.SupportsGlobalVoice = true
	case "multi_step":
		defaults.InputModes = []string{"camera_video"}
		defaults.RequiredPermissions = []string{"camera"}
		defaults.AutoScore = gin.H{"kind": "action_sequence"}
		defaults.Stimulus = gin.H{"label": "三步动作"}
		defaults.CameraProfile = "action_sequence"
		defaults.PreferredLens = "front"
		defaults.ROIPreset = "upper_body"
	default:
		switch {
		case strings.Contains(content, "闭上你的眼睛"):
			defaults.InputModes = []string{"camera_video"}
			defaults.RequiredPermissions = []string{"camera"}
			defaults.AutoScore = gin.H{"kind": "eye_closure"}
			defaults.Stimulus = gin.H{"label": "阅读并闭眼"}
			defaults.CameraProfile = "eye_closure"
			defaults.PreferredLens = "front"
			defaults.ROIPreset = "face"
		case strings.Contains(content, "写一个完整句子"):
			defaults.InputModes = []string{"touch_stroke", "voice"}
			defaults.RequiredPermissions = []string{"microphone"}
			defaults.AutoScore = gin.H{"kind": "sentence_meaning", "requires_review": true}
			defaults.Stimulus = gin.H{"label": "写句子"}
			defaults.CameraProfile = "paper_sentence_capture"
			defaults.PreferredLens = "back"
			defaults.ROIPreset = "paper"
			defaults.SupportsGlobalVoice = true
		case strings.Contains(content, "临摹图形"):
			defaults.InputModes = []string{"touch_stroke", "camera_still"}
			defaults.AutoScore = gin.H{"kind": "shape_drawing", "requires_review": true}
			defaults.Stimulus = gin.H{"label": "临摹图形"}
			defaults.CameraProfile = "paper_shape_capture"
			defaults.PreferredLens = "back"
			defaults.ROIPreset = "paper"
		case strings.Contains(content, "命名"):
			defaults.InputModes = []string{"voice"}
			defaults.RequiredPermissions = []string{"microphone"}
			defaults.AutoScore = gin.H{"kind": "naming"}
			defaults.Stimulus = gin.H{"label": "命名"}
			defaults.WarmupTags = []string{"daily_warmup", "standard_assessment"}
			defaults.SupportsGlobalVoice = true
		}
	}

	return defaults
}

func normalizeAutoScore(raw any) any {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return gin.H{"kind": strings.TrimSpace(v)}
	case map[string]any:
		if len(v) == 0 {
			return nil
		}
		return v
	default:
		return raw
	}
}

func coalesceStringSlice(raw any, fallback []string) []string {
	if values := toStringSlice(raw); len(values) > 0 {
		return values
	}
	if len(fallback) == 0 {
		return nil
	}
	return append([]string(nil), fallback...)
}

func coalesceString(raw any, fallback string) string {
	if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func coalesceBool(raw any, fallback bool) bool {
	switch value := raw.(type) {
	case bool:
		return value
	default:
		return fallback
	}
}
