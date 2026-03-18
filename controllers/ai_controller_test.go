package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestGenerateASRRejectsInvalidFormat(t *testing.T) {
	recorder, ctx := newVoiceTestContext(http.MethodPost, "/api/ai/asr?format=flac&sample_rate=16000", bytes.NewReader([]byte("audio")))

	GenerateASR(ctx)

	assertErrorResponse(t, recorder, http.StatusBadRequest, "INVALID_PARAMS")
}

func TestGenerateASRRejectsInvalidSampleRate(t *testing.T) {
	recorder, ctx := newVoiceTestContext(http.MethodPost, "/api/ai/asr?format=wav&sample_rate=44100", bytes.NewReader([]byte("audio")))

	GenerateASR(ctx)

	assertErrorResponse(t, recorder, http.StatusBadRequest, "INVALID_PARAMS")
}

func TestGenerateASRRequiresAudio(t *testing.T) {
	recorder, ctx := newVoiceTestContext(http.MethodPost, "/api/ai/asr?format=wav&sample_rate=16000", http.NoBody)

	GenerateASR(ctx)

	assertErrorResponse(t, recorder, http.StatusBadRequest, "INVALID_PARAMS")
}

func TestGenerateASRReturnsConfiguredError(t *testing.T) {
	restoreASR := patchASRDependencies(t, false, nil)
	defer restoreASR()

	body, contentType := newMultipartAudioBody(t, "sample.wav", []byte("voice"))
	recorder, ctx := newVoiceTestContext(http.MethodPost, "/api/ai/asr?format=wav&sample_rate=16000", body)
	ctx.Request.Header.Set("Content-Type", contentType)

	GenerateASR(ctx)

	assertErrorResponse(t, recorder, http.StatusServiceUnavailable, voiceNotConfiguredCode)
}

func TestGenerateASRReturnsUpstreamError(t *testing.T) {
	restoreASR := patchASRDependencies(t, true, func(data []byte, format string, sampleRate int, opts utils.NLSASROptions) (string, map[string]any, error) {
		return "", nil, errors.New("boom")
	})
	defer restoreASR()

	body, contentType := newMultipartAudioBody(t, "sample.wav", []byte("voice"))
	recorder, ctx := newVoiceTestContext(http.MethodPost, "/api/ai/asr?format=wav&sample_rate=16000", body)
	ctx.Request.Header.Set("Content-Type", contentType)

	GenerateASR(ctx)

	assertErrorResponse(t, recorder, http.StatusBadGateway, voiceServiceUnavailableCode)
}

func TestGenerateASRSuccess(t *testing.T) {
	restoreASR := patchASRDependencies(t, true, func(data []byte, format string, sampleRate int, opts utils.NLSASROptions) (string, map[string]any, error) {
		if string(data) != "voice" {
			t.Fatalf("unexpected audio payload: %q", string(data))
		}
		if format != "wav" || sampleRate != 16000 {
			t.Fatalf("unexpected asr options: format=%s sampleRate=%d", format, sampleRate)
		}
		if !opts.EnablePunctuationPrediction || !opts.EnableInverseTextNormalization || !opts.EnableVoiceDetection {
			t.Fatalf("expected default asr flags enabled, got %#v", opts)
		}
		return "2026", map[string]any{"status": 0}, nil
	})
	defer restoreASR()

	body, contentType := newMultipartAudioBody(t, "sample.wav", []byte("voice"))
	recorder, ctx := newVoiceTestContext(http.MethodPost, "/api/ai/asr?format=wav&sample_rate=16000", body)
	ctx.Request.Header.Set("Content-Type", contentType)

	GenerateASR(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["text"] != "2026" || payload["vendor"] != "aliyun_nls" {
		t.Fatalf("unexpected success payload: %#v", payload)
	}
}

func TestGenerateTTSReturnsConfiguredError(t *testing.T) {
	restoreTTS := patchTTSDependencies(t, false, nil, nil)
	defer restoreTTS()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/tts", map[string]any{"text": "你好"})

	GenerateTTS(ctx)

	assertErrorResponse(t, recorder, http.StatusServiceUnavailable, voiceNotConfiguredCode)
}

func TestGenerateTTSSuccess(t *testing.T) {
	restoreTTS := patchTTSDependencies(t, true, func(text, voice string) ([]byte, string, string, string, error) {
		if text != "你好" {
			t.Fatalf("unexpected text: %q", text)
		}
		return []byte("audio-bytes"), "audio/mpeg", "qwen3-tts-flash", "longxiaochun", nil
	}, nil)
	defer restoreTTS()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/tts", map[string]any{"text": "你好"})

	GenerateTTS(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-TTS-Model") != "qwen3-tts-flash" || recorder.Header().Get("X-TTS-Voice") != "longxiaochun" {
		t.Fatalf("unexpected tts headers: %#v", recorder.Header())
	}
	if recorder.Header().Get("X-TTS-Source") != "system" {
		t.Fatalf("unexpected tts source: %#v", recorder.Header())
	}
	if recorder.Body.String() != "audio-bytes" {
		t.Fatalf("unexpected tts body: %q", recorder.Body.String())
	}
}

func TestGenerateTTSFallsBackWhenDefaultCloneFails(t *testing.T) {
	restoreTTS := patchTTSDependencies(
		t,
		true,
		func(text, voice string) ([]byte, string, string, string, error) {
			if voice != "" {
				t.Fatalf("expected fallback system synthesis, got voice=%q", voice)
			}
			return []byte("fallback-audio"), "audio/mpeg", "qwen3-tts-flash", "longxiaochun", nil
		},
		func(text, voice, model string) ([]byte, string, string, string, error) {
			if voice != "clone-voice-1" || model != "qwen-tts" {
				t.Fatalf("unexpected custom synthesis options: voice=%s model=%s", voice, model)
			}
			return nil, "", "", "", errors.New("voice not found")
		},
	)
	defer restoreTTS()

	origCurrentUser := getCurrentTTSUserFn
	origDefaultVoice := findDefaultVoiceProfileFn
	origMarkFailed := markVoiceProfileFailedFn
	defer func() {
		getCurrentTTSUserFn = origCurrentUser
		findDefaultVoiceProfileFn = origDefaultVoice
		markVoiceProfileFailedFn = origMarkFailed
	}()

	getCurrentTTSUserFn = func(ctx *gin.Context) (models.User, error) {
		return models.User{Model: gorm.Model{ID: 7}, Username: "tester"}, nil
	}
	findDefaultVoiceProfileFn = func(userID uint) (models.VoiceProfile, bool, error) {
		return models.VoiceProfile{
			Model:         gorm.Model{ID: 11},
			VendorVoiceID: "clone-voice-1",
			Status:        voiceStatusReady,
			IsDefault:     true,
		}, true, nil
	}
	markVoiceProfileFailedFn = func(profileID uint, lastError string) error {
		if profileID != 11 {
			t.Fatalf("unexpected failed profile id: %d", profileID)
		}
		return nil
	}

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/tts", map[string]any{"text": "你好"})

	GenerateTTS(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-TTS-Source") != "fallback" {
		t.Fatalf("expected fallback source, got headers=%#v", recorder.Header())
	}
	if recorder.Body.String() != "fallback-audio" {
		t.Fatalf("unexpected fallback tts body: %q", recorder.Body.String())
	}
}

func TestGenerateLifeAssistantTipReturnsStructuredAnswer(t *testing.T) {
	restoreOpenAI := patchOpenAIChat(t, func(messages []utils.OpenAIMessage, temperature float64) (string, string, error) {
		if len(messages) != 2 {
			t.Fatalf("unexpected message count: %d", len(messages))
		}
		return `{"answer":"今晚先按固定时间休息。","quick_actions":["早点洗漱","睡前放松"],"suggested_questions":["午休多久合适？","晚饭后适合散步吗？"]}`, "gpt-test", nil
	})
	defer restoreOpenAI()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/life-assistant", map[string]any{
		"query": "晚上睡不好怎么办",
	})

	GenerateLifeAssistantTip(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["answer"] != "今晚先按固定时间休息。" {
		t.Fatalf("unexpected answer payload: %#v", payload)
	}
	if payload["tip"] != "今晚先按固定时间休息。" {
		t.Fatalf("expected legacy tip fallback, got %#v", payload)
	}
	quickActions, ok := payload["quick_actions"].([]any)
	if !ok || len(quickActions) != 2 {
		t.Fatalf("unexpected quick_actions payload: %#v", payload)
	}
	actions, ok := payload["actions"].([]any)
	if !ok || len(actions) != 2 {
		t.Fatalf("unexpected legacy actions payload: %#v", payload)
	}
	suggestedQuestions, ok := payload["suggested_questions"].([]any)
	if !ok || len(suggestedQuestions) != 2 {
		t.Fatalf("unexpected suggested_questions payload: %#v", payload)
	}
}

func TestPlanVoiceControlMapsGoHome(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		if len(messages) != 2 {
			t.Fatalf("unexpected message count: %d", len(messages))
		}
		if len(tools) != 4 {
			t.Fatalf("expected 4 tools, got %d", len(tools))
		}
		return utils.OpenAIChatResult{
			Content: "好的，正在返回主页。",
			Model:   "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionGoHome,
						Arguments: `{}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "返回主页",
		"current_tab":       "mmse",
		"current_route":     "main_shell/mmse",
		"can_pop":           false,
		"available_actions": []string{"go_home", "switch_tab", "go_back", "open_page"},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionGoHome {
		t.Fatalf("unexpected action payload: %#v", payload)
	}
	if payload["model"] != "gpt-test" {
		t.Fatalf("unexpected model payload: %#v", payload)
	}
}

func TestPlanVoiceControlMapsAskLifeAssistant(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		foundAskTool := false
		for _, tool := range tools {
			if tool.Function.Name == voiceControlActionAskLifeAssistant {
				foundAskTool = true
				break
			}
		}
		if !foundAskTool {
			t.Fatalf("expected ask_life_assistant tool, got %#v", tools)
		}
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionAskLifeAssistant,
						Arguments: `{"query":"晚上睡不好怎么办"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "晚上睡不好怎么办",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"go_home", "switch_tab", "go_back", "open_page", "ask_life_assistant"},
		"available_pages": []map[string]any{
			{"id": "life_assistant", "label": "生活助手", "aliases": []string{"生活助手", "日常助手"}},
		},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionAskLifeAssistant {
		t.Fatalf("unexpected action payload: %#v", payload)
	}
	actionArgs, ok := payload["action_args"].(map[string]any)
	if !ok || actionArgs["query"] != "晚上睡不好怎么办" {
		t.Fatalf("unexpected action args payload: %#v", payload)
	}
}

func TestPlanVoiceControlMapsSwitchTab(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionSwitchTab,
						Arguments: `{"tab":"mmse"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "去做评估",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"switch_tab"},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionSwitchTab {
		t.Fatalf("unexpected action payload: %#v", payload)
	}
	args, ok := payload["action_args"].(map[string]any)
	if !ok || args["tab"] != "mmse" {
		t.Fatalf("unexpected action args: %#v", payload["action_args"])
	}
}

func TestPlanVoiceControlMapsCreateReminder(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionCreateReminder,
						Arguments: `{"reminder_type":"training","title":"做脑力热身","time":"09:30","date":"2026-03-18","repeat_rule":"once","note":"早餐后"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "明天早上九点半提醒我做脑力热身",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"create_reminder"},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionCreateReminder {
		t.Fatalf("unexpected action payload: %#v", payload)
	}
	args, ok := payload["action_args"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected action args: %#v", payload["action_args"])
	}
	if args["reminder_type"] != "training" || args["title"] != "做脑力热身" || args["time"] != "09:30" || args["date"] != "2026-03-18" || args["repeat_rule"] != "once" {
		t.Fatalf("unexpected reminder draft args: %#v", args)
	}
}

func TestPlanVoiceControlMapsCompleteReminder(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionCompleteReminder,
						Arguments: `{"ordinal":1,"status":"done"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "完成第一条提醒",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"complete_reminder"},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionCompleteReminder {
		t.Fatalf("unexpected action payload: %#v", payload)
	}
	args, ok := payload["action_args"].(map[string]any)
	if !ok || int(args["ordinal"].(float64)) != 1 || args["status"] != "done" {
		t.Fatalf("unexpected action args: %#v", payload["action_args"])
	}
}

func TestPlanVoiceControlFallsBackToDefaultTargetsWithoutFrontendContext(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		tabEnum := extractToolEnum(t, tools, voiceControlActionSwitchTab, "tab")
		pageEnum := extractToolEnum(t, tools, voiceControlActionOpenPage, "page")
		if !reflect.DeepEqual(tabEnum, []string{"home", "reminders", "mmse", "profile"}) {
			t.Fatalf("unexpected fallback tab enum: %#v", tabEnum)
		}
		if !reflect.DeepEqual(pageEnum, []string{"life_assistant", "cognitive_profile", "training_effect", "mmse_daily_home"}) {
			t.Fatalf("unexpected fallback page enum: %#v", pageEnum)
		}
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionOpenPage,
						Arguments: `{"page":"life_assistant"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "打开日常助手",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"go_home", "switch_tab", "go_back", "open_page"},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	args, ok := payload["action_args"].(map[string]any)
	if !ok || args["page"] != "life_assistant" {
		t.Fatalf("unexpected action args: %#v", payload["action_args"])
	}
}

func TestPlanVoiceControlUsesProvidedTargetsForToolEnums(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		tabEnum := extractToolEnum(t, tools, voiceControlActionSwitchTab, "tab")
		pageEnum := extractToolEnum(t, tools, voiceControlActionOpenPage, "page")
		if !reflect.DeepEqual(tabEnum, []string{"home", "reminders", "mmse", "profile"}) {
			t.Fatalf("unexpected tab enum: %#v", tabEnum)
		}
		if !reflect.DeepEqual(pageEnum, []string{"reminders", "assessment", "life_assistant"}) {
			t.Fatalf("unexpected page enum: %#v", pageEnum)
		}
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionOpenPage,
						Arguments: `{"page":"assessment"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "打开评估",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"go_home", "switch_tab", "go_back", "open_page"},
		"available_tabs": []map[string]any{
			{"id": "home", "label": "首页", "aliases": []string{"首页", "主页"}},
			{"id": "reminders", "label": "提醒", "aliases": []string{"提醒", "服药提醒"}},
			{"id": "mmse", "label": "MMSE", "aliases": []string{"评估", "mmse"}},
			{"id": "profile", "label": "我的", "aliases": []string{"我的"}},
		},
		"available_pages": []map[string]any{
			{"id": "reminders", "label": "用药提醒", "aliases": []string{"用药提醒", "medication_schedule"}},
			{"id": "assessment", "label": "去做评估", "aliases": []string{"评估", "mmse_page"}},
			{"id": "life_assistant", "label": "日常助手", "aliases": []string{"日常助手"}},
			{"id": "family_contact", "label": "亲友联系", "aliases": []string{"亲友联系"}},
		},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionOpenPage {
		t.Fatalf("unexpected action payload: %#v", payload)
	}
	args, ok := payload["action_args"].(map[string]any)
	if !ok || args["page"] != "assessment" {
		t.Fatalf("unexpected action args: %#v", payload["action_args"])
	}
}

func TestPlanVoiceControlPromptUsesProvidedLabelsAndAliases(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		if len(messages) != 2 {
			t.Fatalf("unexpected message count: %d", len(messages))
		}
		systemPrompt := messages[0].Content
		if !strings.Contains(systemPrompt, "当前可切换的标签") {
			t.Fatalf("system prompt missing tab summary: %s", systemPrompt)
		}
		if !strings.Contains(systemPrompt, "服药提醒（id=reminders") {
			t.Fatalf("system prompt missing reminders label/id: %s", systemPrompt)
		}
		if !strings.Contains(systemPrompt, "medication_schedule") {
			t.Fatalf("system prompt missing legacy alias preview: %s", systemPrompt)
		}
		if strings.Contains(systemPrompt, "family_contact") {
			t.Fatalf("system prompt should not mention filtered family_contact target: %s", systemPrompt)
		}
		return utils.OpenAIChatResult{
			Content: "好的",
			Model:   "gpt-test",
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "打开提醒",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"go_home", "switch_tab", "go_back", "open_page"},
		"available_tabs": []map[string]any{
			{"id": "home", "label": "首页", "aliases": []string{"首页", "主页"}},
			{"id": "reminders", "label": "提醒", "aliases": []string{"提醒", "服药提醒"}},
		},
		"available_pages": []map[string]any{
			{"id": "reminders", "label": "服药提醒", "aliases": []string{"提醒", "medication_schedule"}},
			{"id": "family_contact", "label": "亲友联系", "aliases": []string{"亲友联系"}},
		},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionNone {
		t.Fatalf("expected no_action, got %#v", payload)
	}
}

func TestPlanVoiceControlNormalizesLegacyPageAliasFromProvidedTargets(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionOpenPage,
						Arguments: `{"page":"medication_schedule"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "打开提醒",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"open_page"},
		"available_pages": []map[string]any{
			{"id": "reminders", "label": "用药提醒", "aliases": []string{"提醒", "medication_schedule"}},
		},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	args, ok := payload["action_args"].(map[string]any)
	if !ok || args["page"] != "reminders" {
		t.Fatalf("unexpected action args: %#v", payload["action_args"])
	}
}

func TestPlanVoiceControlRejectsUnsupportedFamilyContactTarget(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		pageEnum := extractToolEnum(t, tools, voiceControlActionOpenPage, "page")
		for _, item := range pageEnum {
			if item == "family_contact" {
				t.Fatalf("family_contact should not be exposed: %#v", pageEnum)
			}
		}
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionOpenPage,
						Arguments: `{"page":"family_contact"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "联系家人",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"open_page"},
		"available_pages": []map[string]any{
			{"id": "family_contact", "label": "亲友联系", "aliases": []string{"亲友联系"}},
			{"id": "reminders", "label": "用药提醒", "aliases": []string{"提醒"}},
		},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionNone {
		t.Fatalf("expected no_action, got %#v", payload)
	}
}

func TestPlanVoiceControlRejectsInvalidToolName(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		return utils.OpenAIChatResult{
			Content: "这个操作不支持。",
			Model:   "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      "delete_account",
						Arguments: `{}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "删除账号",
		"current_tab":       "profile",
		"current_route":     "main_shell/profile",
		"can_pop":           false,
		"available_actions": []string{"go_home", "switch_tab", "go_back", "open_page"},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionNone {
		t.Fatalf("expected no_action, got %#v", payload)
	}
}

func TestPlanVoiceControlRejectsInvalidToolArgs(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		return utils.OpenAIChatResult{
			Model: "gpt-test",
			ToolCalls: []utils.OpenAIToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: utils.OpenAIFunctionCall{
						Name:      voiceControlActionSwitchTab,
						Arguments: `{"tab":"settings"}`,
					},
				},
			},
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":              "切到设置",
		"current_tab":       "home",
		"current_route":     "main_shell/home",
		"can_pop":           false,
		"available_actions": []string{"switch_tab"},
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionNone {
		t.Fatalf("expected no_action, got %#v", payload)
	}
}

func TestPlanVoiceControlAllowsReplyWithoutTool(t *testing.T) {
	restorePlanner := patchOpenAIPlanner(t, func(messages []utils.OpenAIMessage, tools []utils.OpenAITool, temperature float64) (utils.OpenAIChatResult, error) {
		return utils.OpenAIChatResult{
			Content: "请再具体一点，比如说返回主页。",
			Model:   "gpt-test",
		}, nil
	})
	defer restorePlanner()

	recorder, ctx := newJSONVoiceTestContext(t, "/api/ai/voice-control/plan", map[string]any{
		"text":          "帮我处理一下",
		"current_tab":   "home",
		"current_route": "main_shell/home",
		"can_pop":       false,
	})

	PlanVoiceControl(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["action"] != voiceControlActionNone {
		t.Fatalf("expected no_action, got %#v", payload)
	}
	if payload["reply_text"] != "请再具体一点，比如说返回主页。" {
		t.Fatalf("unexpected reply payload: %#v", payload)
	}
}

func extractToolEnum(t *testing.T, tools []utils.OpenAITool, toolName, field string) []string {
	t.Helper()
	for _, tool := range tools {
		if tool.Function.Name != toolName {
			continue
		}
		properties, ok := tool.Function.Parameters["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %s missing properties: %#v", toolName, tool.Function.Parameters)
		}
		fieldValue, ok := properties[field].(map[string]any)
		if !ok {
			t.Fatalf("tool %s missing field %s: %#v", toolName, field, properties)
		}
		if enumValues, ok := fieldValue["enum"].([]string); ok {
			return enumValues
		}
		rawEnum, ok := fieldValue["enum"].([]any)
		if !ok {
			t.Fatalf("tool %s missing enum for %s: %#v", toolName, field, fieldValue)
		}
		out := make([]string, 0, len(rawEnum))
		for _, item := range rawEnum {
			value, ok := item.(string)
			if !ok {
				t.Fatalf("tool %s enum item has unexpected type: %#v", toolName, item)
			}
			out = append(out, value)
		}
		return out
	}
	t.Fatalf("tool %s not found", toolName)
	return nil
}

func newVoiceTestContext(method, target string, body io.Reader) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, body)
	return recorder, ctx
}

func newJSONVoiceTestContext(t *testing.T, target string, payload map[string]any) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	recorder, ctx := newVoiceTestContext(http.MethodPost, target, bytes.NewReader(bodyBytes))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return recorder, ctx
}

func newMultipartAudioBody(t *testing.T, filename string, content []byte) (*bytes.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return bytes.NewReader(body.Bytes()), writer.FormDataContentType()
}

func patchASRDependencies(t *testing.T, configured bool, fn func([]byte, string, int, utils.NLSASROptions) (string, map[string]any, error)) func() {
	t.Helper()
	origConfigured := isAliyunNLSConfiguredFn
	origASR := aliyunNLSASRFn
	isAliyunNLSConfiguredFn = func() bool { return configured }
	if fn != nil {
		aliyunNLSASRFn = fn
	}
	return func() {
		isAliyunNLSConfiguredFn = origConfigured
		aliyunNLSASRFn = origASR
	}
}

func patchTTSDependencies(t *testing.T, configured bool, fn func(string, string) ([]byte, string, string, string, error), customFn func(string, string, string) ([]byte, string, string, string, error)) func() {
	t.Helper()
	origConfigured := isDashScopeConfiguredFn
	origTTS := dashScopeTTSBytesFn
	origCustomTTS := dashScopeTTSBytesWithModelFn
	origCurrentUser := getCurrentTTSUserFn
	origDefaultVoice := findDefaultVoiceProfileFn
	origVoiceByID := findVoiceProfileByVoiceIDFn
	origMarkFailed := markVoiceProfileFailedFn
	isDashScopeConfiguredFn = func() bool { return configured }
	getCurrentTTSUserFn = func(ctx *gin.Context) (models.User, error) {
		return models.User{}, errors.New("no user in test")
	}
	findDefaultVoiceProfileFn = func(userID uint) (models.VoiceProfile, bool, error) {
		return models.VoiceProfile{}, false, nil
	}
	findVoiceProfileByVoiceIDFn = func(userID uint, vendorVoiceID string) (models.VoiceProfile, bool, error) {
		return models.VoiceProfile{}, false, nil
	}
	markVoiceProfileFailedFn = func(profileID uint, lastError string) error { return nil }
	if fn != nil {
		dashScopeTTSBytesFn = fn
	}
	if customFn != nil {
		dashScopeTTSBytesWithModelFn = customFn
	} else {
		dashScopeTTSBytesWithModelFn = func(text, voice, model string) ([]byte, string, string, string, error) {
			return dashScopeTTSBytesFn(text, voice)
		}
	}
	return func() {
		isDashScopeConfiguredFn = origConfigured
		dashScopeTTSBytesFn = origTTS
		dashScopeTTSBytesWithModelFn = origCustomTTS
		getCurrentTTSUserFn = origCurrentUser
		findDefaultVoiceProfileFn = origDefaultVoice
		findVoiceProfileByVoiceIDFn = origVoiceByID
		markVoiceProfileFailedFn = origMarkFailed
	}
}

func patchOpenAIPlanner(t *testing.T, fn func([]utils.OpenAIMessage, []utils.OpenAITool, float64) (utils.OpenAIChatResult, error)) func() {
	t.Helper()
	orig := openAIChatWithToolsFn
	openAIChatWithToolsFn = fn
	return func() {
		openAIChatWithToolsFn = orig
	}
}

func patchOpenAIChat(t *testing.T, fn func([]utils.OpenAIMessage, float64) (string, string, error)) func() {
	t.Helper()
	orig := lifeAssistantOpenAIChatFn
	lifeAssistantOpenAIChatFn = fn
	return func() {
		lifeAssistantOpenAIChatFn = orig
	}
}

func assertErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("expected status=%d, got %d body=%s", status, recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	errorPayload, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %#v", payload)
	}
	if errorPayload["code"] != code {
		t.Fatalf("expected error code=%s, got %#v", code, errorPayload)
	}
}

func decodeJSONBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, recorder.Body.String())
	}
	return payload
}
