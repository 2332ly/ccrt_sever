package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetMMSEScaleReturnsCapabilityMetadata(t *testing.T) {
	db := setupMMSEControllerTestDB(t)
	_, version, questions := seedMMSEScaleTestData(t, db)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/mmse/scale", http.NoBody)

	GetMMSEScale(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	payload := decodeMMSEJSONBody(t, recorder)
	versionPayload := payload["version"].(map[string]any)
	if int(versionPayload["id"].(float64)) != int(version.ID) {
		t.Fatalf("unexpected version payload: %#v", versionPayload)
	}

	modules := payload["modules"].([]any)
	firstModule := modules[0].(map[string]any)
	firstQuestion := firstModule["questions"].([]any)[0].(map[string]any)
	options := firstQuestion["options"].(map[string]any)

	if got := firstQuestion["id"].(float64); int(got) != int(questions[0].ID) {
		t.Fatalf("unexpected first question payload: %#v", firstQuestion)
	}
	assertStringSliceJSON(t, options["input_modes"], []string{"voice"})
	assertStringSliceJSON(t, options["required_permissions"], []string{"microphone"})
	if options["fallback_mode"] != "caregiver_confirm" {
		t.Fatalf("expected caregiver_confirm fallback, got %#v", options["fallback_mode"])
	}
	if options["supports_global_voice"] != true {
		t.Fatalf("expected supports_global_voice=true, got %#v", options["supports_global_voice"])
	}
	autoScore := options["auto_score"].(map[string]any)
	if autoScore["kind"] != "date_time_orientation" {
		t.Fatalf("unexpected auto_score payload: %#v", autoScore)
	}
}

func TestBuildCapabilityOptionsIncludesLocationContextDefaults(t *testing.T) {
	options := buildCapabilityOptions(
		models.ScaleQuestion{
			Type:    "fields_correct",
			Content: "璇锋偍璇村嚭褰撳墠鐨勫煄甯傘€佸尯鍘垮拰妤煎眰",
		},
		map[string]any{
			"type": "fields_correct",
		},
	)

	assertStringSliceNative(
		t,
		options["expected_location_context_fields"],
		[]string{"city", "district", "street", "place", "floor"},
	)
	if options["supports_global_voice"] != true {
		t.Fatalf("expected supports_global_voice=true, got %#v", options["supports_global_voice"])
	}
	autoScore := options["auto_score"].(gin.H)
	if autoScore["kind"] != "location_orientation" {
		t.Fatalf("unexpected auto_score payload: %#v", autoScore)
	}
}

func TestBuildCapabilityOptionsIncludesGlobalVoiceDefaultsForTextAnswers(t *testing.T) {
	memoryOptions := buildCapabilityOptions(
		models.ScaleQuestion{
			Type:    "set_match",
			Content: "请重复这三个词：花园、冰箱、国旗。",
		},
		map[string]any{
			"type": "set_match",
		},
	)

	assertStringSliceNative(t, memoryOptions["input_modes"], []string{"voice"})
	if memoryOptions["supports_global_voice"] != true {
		t.Fatalf("expected memory supports_global_voice=true, got %#v", memoryOptions["supports_global_voice"])
	}

	namingOptions := buildCapabilityOptions(
		models.ScaleQuestion{
			Type:    "naming",
			Content: "命名：请说出这个物品的名称",
		},
		map[string]any{
			"type": "manual",
		},
	)

	assertStringSliceNative(t, namingOptions["input_modes"], []string{"voice"})
	if namingOptions["supports_global_voice"] != true {
		t.Fatalf("expected naming supports_global_voice=true, got %#v", namingOptions["supports_global_voice"])
	}

	sentenceOptions := buildCapabilityOptions(
		models.ScaleQuestion{
			Type:    "write_or_voice",
			Content: "请写一个完整句子",
		},
		map[string]any{
			"type": "manual",
		},
	)

	assertStringSliceNative(t, sentenceOptions["input_modes"], []string{"touch_stroke", "voice"})
	if sentenceOptions["supports_global_voice"] != true {
		t.Fatalf("expected sentence supports_global_voice=true, got %#v", sentenceOptions["supports_global_voice"])
	}
}

func TestBuildCapabilityOptionsIncludesCameraDefaults(t *testing.T) {
	options := buildCapabilityOptions(
		models.ScaleQuestion{
			Type:    "multi_step",
			Content: "璇风敤鍙虫墜鎷胯捣绾稿紶",
		},
		map[string]any{
			"type": "multi_step",
		},
	)

	assertStringSliceNative(t, options["input_modes"], []string{"camera_video"})
	assertStringSliceNative(t, options["required_permissions"], []string{"camera"})
	if options["camera_profile"] != "action_sequence" {
		t.Fatalf("unexpected camera_profile: %#v", options["camera_profile"])
	}
	if options["preferred_lens"] != "front" {
		t.Fatalf("unexpected preferred_lens: %#v", options["preferred_lens"])
	}
	if options["roi_preset"] != "upper_body" {
		t.Fatalf("unexpected roi_preset: %#v", options["roi_preset"])
	}
}

func TestEvaluateMMSEAnswerReturnsStructuredResult(t *testing.T) {
	db := setupMMSEControllerTestDB(t)
	user, _, _ := seedMMSEScaleTestData(t, db)

	original := openAIChatFn
	openAIChatFn = func(messages []utils.OpenAIMessage, temperature float64) (string, string, error) {
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}
		return `{"score":1,"confidence":0.93,"status":"matched","reason":"地点匹配","answer_payload":{"text":"上海市","matched_fields":["city"]}}`, "gpt-test", nil
	}
	t.Cleanup(func() {
		openAIChatFn = original
	})

	body := mustMarshalJSON(t, map[string]any{
		"question_key":    "location_city",
		"question_prompt": "我们现在在哪个省或城市？",
		"question_type":   "voice",
		"max_score":       1,
		"transcript":      "上海市",
		"session_mode":    "warmup",
		"location_context": map[string]any{
			"city": []string{"上海市", "上海"},
		},
		"expected_values": map[string]any{
			"city": []string{"上海市", "上海"},
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/mmse/answers/evaluate", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", user.Username)

	EvaluateMMSEAnswer(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	payload := decodeMMSEJSONBody(t, recorder)
	if payload["status"] != "matched" || int(payload["score"].(float64)) != 1 {
		t.Fatalf("unexpected evaluation payload: %#v", payload)
	}
	answerPayload := payload["answer_payload"].(map[string]any)
	if answerPayload["text"] != "上海市" {
		t.Fatalf("unexpected answer_payload: %#v", answerPayload)
	}
}

func TestEvaluateMMSEAnswerReturnsControlledFailureWhenAIUnavailable(t *testing.T) {
	db := setupMMSEControllerTestDB(t)
	user, _, _ := seedMMSEScaleTestData(t, db)

	original := openAIChatFn
	openAIChatFn = func(messages []utils.OpenAIMessage, temperature float64) (string, string, error) {
		return "", "", errors.New("timeout")
	}
	t.Cleanup(func() {
		openAIChatFn = original
	})

	body := mustMarshalJSON(t, map[string]any{
		"question_key":    "memory_immediate",
		"question_prompt": "请重复一遍这三个词：花园、冰箱、国旗。",
		"question_type":   "teach",
		"max_score":       3,
		"transcript":      "花园 冰箱 国旗",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/mmse/answers/evaluate", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", user.Username)

	EvaluateMMSEAnswer(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	payload := decodeMMSEJSONBody(t, recorder)
	if payload["status"] != "failed" || int(payload["score"].(float64)) != 0 {
		t.Fatalf("unexpected failure payload: %#v", payload)
	}
}

func TestMeIncludesLocationFields(t *testing.T) {
	db := setupMMSEControllerTestDB(t)
	user, _, _ := seedMMSEScaleTestData(t, db)

	user.City = "上海市"
	user.Address = "浦东新区张江路88号3层康复中心"
	if err := db.Save(&user).Error; err != nil {
		t.Fatalf("save user location: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/me", http.NoBody)
	ctx.Set("username", user.Username)

	Me(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	payload := decodeMMSEJSONBody(t, recorder)
	if payload["city"] != "上海市" || payload["address"] != "浦东新区张江路88号3层康复中心" {
		t.Fatalf("unexpected me payload: %#v", payload)
	}
}

func TestSubmitMMSEAssessmentStoresStructuredAnswersAndArtifacts(t *testing.T) {
	db := setupMMSEControllerTestDB(t)
	user, version, questions := seedMMSEScaleTestData(t, db)

	requestBody := map[string]any{
		"scale_version_id": version.ID,
		"answers": []map[string]any{
			{
				"question_id": questions[0].ID,
				"answer_payload": map[string]any{
					"correct_fields": []string{"year", "month"},
				},
				"device_metrics": map[string]any{
					"latency_ms": 320,
				},
			},
			{
				"question_id": questions[1].ID,
				"answer_payload": map[string]any{
					"score": 1,
					"text":  "今天天气不错",
				},
				"manual_override": true,
				"artifact_refs": []map[string]any{
					{
						"kind":         "audio",
						"artifact_key": "audio/mmse/q2-answer.wav",
						"confidence":   0.92,
						"meta": map[string]any{
							"duration_ms": 1234,
						},
					},
				},
			},
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/mmse/assessments",
		bytes.NewReader(mustMarshalJSON(t, requestBody)),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", user.Username)

	SubmitMMSEAssessment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var assessments []models.ScaleAssessment
	if err := db.Find(&assessments).Error; err != nil {
		t.Fatalf("query assessments: %v", err)
	}
	if len(assessments) != 1 {
		t.Fatalf("expected 1 assessment, got %d", len(assessments))
	}
	if assessments[0].TotalScore != 3 {
		t.Fatalf("expected total score 3, got %#v", assessments[0])
	}

	var answers []models.ScaleAnswer
	if err := db.Order("question_id asc").Find(&answers).Error; err != nil {
		t.Fatalf("query answers: %v", err)
	}
	if len(answers) != 2 {
		t.Fatalf("expected 2 answers, got %d", len(answers))
	}
	if answers[0].AnswerPayload != `{"correct_fields":["year","month"]}` {
		t.Fatalf("unexpected first answer payload: %#v", answers[0])
	}
	if answers[0].DeviceMetrics != `{"latency_ms":320}` {
		t.Fatalf("unexpected first device metrics: %#v", answers[0])
	}
	if answers[1].UserAnswer != "今天天气不错" {
		t.Fatalf("expected legacy user answer text, got %#v", answers[1].UserAnswer)
	}
	if !answers[1].ManualOverride {
		t.Fatalf("expected manual override to be stored, got %#v", answers[1])
	}

	var artifacts []models.ScaleAnswerArtifact
	if err := db.Find(&artifacts).Error; err != nil {
		t.Fatalf("query artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].AssessmentID != assessments[0].ID || artifacts[0].AnswerID != answers[1].ID {
		t.Fatalf("unexpected artifact linkage: %#v", artifacts[0])
	}
	if artifacts[0].ReviewStatus != "pending" || artifacts[0].Kind != "audio" {
		t.Fatalf("unexpected artifact payload: %#v", artifacts[0])
	}
	if artifacts[0].Confidence != 0.92 {
		t.Fatalf("unexpected artifact confidence: %#v", artifacts[0])
	}
}

func TestScoreByRuleUsesAnswerPayloadWhenUserAnswerMissing(t *testing.T) {
	question := models.ScaleQuestion{
		Type: "sequence_match",
		AnswerRule: mustJSONString(t, map[string]any{
			"type":             "sequence_match",
			"correct_sequence": []int{93, 86, 79},
			"score_per_step":   1,
		}),
	}

	score, err := scoreByRule(question, MMSEAnswerInput{
		QuestionID: question.ID,
		AnswerPayload: mustJSONRaw(t, map[string]any{
			"sequence": []int{93, 86, 79},
		}),
	})
	if err != nil {
		t.Fatalf("scoreByRule returned error: %v", err)
	}
	if score != 3 {
		t.Fatalf("expected score 3, got %d", score)
	}
}

func TestPresignCCRTArtifactReturnsStubPayload(t *testing.T) {
	db := setupMMSEControllerTestDB(t)
	user, _, _ := seedMMSEScaleTestData(t, db)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/ccrt/artifacts/presign",
		bytes.NewReader(mustMarshalJSON(t, map[string]any{
			"file_name":     "answer.wav",
			"content_type":  "audio/wav",
			"kind":          "audio",
			"question_id":   3,
			"assessment_id": 7,
		})),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", user.Username)

	PresignCCRTArtifact(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeMMSEJSONBody(t, recorder)
	if payload["provider"] != "stub" || payload["method"] != "PUT" {
		t.Fatalf("unexpected presign payload: %#v", payload)
	}
	headers := payload["headers"].(map[string]any)
	if headers["Content-Type"] != "audio/wav" {
		t.Fatalf("unexpected headers payload: %#v", headers)
	}
	if objectKey := payload["object_key"].(string); objectKey == "" || objectKey[:10] != "ccrt/user-" {
		t.Fatalf("unexpected object key: %#v", payload["object_key"])
	}
}

func setupMMSEControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Scale{},
		&models.ScaleVersion{},
		&models.ScaleModule{},
		&models.ScaleQuestion{},
		&models.ScaleAssessment{},
		&models.ScaleAnswer{},
		&models.ScaleAnswerArtifact{},
		&models.AssessmentModuleScore{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	originalDB := global.Db
	global.Db = db
	t.Cleanup(func() {
		global.Db = originalDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedMMSEScaleTestData(
	t *testing.T,
	db *gorm.DB,
) (models.User, models.ScaleVersion, []models.ScaleQuestion) {
	t.Helper()

	user := models.User{Username: "tester", Phone: "13800000000", Password: "secret"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	scale := models.Scale{Name: "MMSE-Test", TotalScore: 3}
	if err := db.Create(&scale).Error; err != nil {
		t.Fatalf("create scale: %v", err)
	}
	version := models.ScaleVersion{
		ScaleID:    scale.ID,
		Name:       "MMSE-Test",
		Version:    "2026.03",
		TotalScore: 3,
		IsActive:   true,
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create version: %v", err)
	}

	modules := []models.ScaleModule{
		{ScaleVersionID: version.ID, Name: "定向力", MaxScore: 2, SortOrder: 1},
		{ScaleVersionID: version.ID, Name: "语言能力", MaxScore: 1, SortOrder: 2},
	}
	for i := range modules {
		if err := db.Create(&modules[i]).Error; err != nil {
			t.Fatalf("create module %d: %v", i, err)
		}
	}

	questions := []models.ScaleQuestion{
		{
			ModuleID: modules[0].ID,
			Type:     "fields_correct",
			Content:  "现在是？年份/月份（每项1分）",
			MaxScore: 2,
			AnswerRule: mustJSONString(t, map[string]any{
				"type":                 "fields_correct",
				"fields":               []string{"year", "month"},
				"score_per_field":      1,
				"input_modes":          []string{"voice"},
				"required_permissions": []string{"microphone"},
				"stimulus":             map[string]any{"label": "时间定向"},
				"auto_score":           map[string]any{"kind": "date_time_orientation"},
				"warmup_tags":          []string{"daily_warmup", "standard_assessment"},
				"fallback_mode":        "caregiver_confirm",
			}),
			SortOrder: 1,
		},
		{
			ModuleID: modules[1].ID,
			Type:     "manual",
			Content:  "写一个完整句子",
			MaxScore: 1,
			AnswerRule: mustJSONString(t, map[string]any{
				"type": "manual",
			}),
			SortOrder: 1,
		},
	}
	for i := range questions {
		if err := db.Create(&questions[i]).Error; err != nil {
			t.Fatalf("create question %d: %v", i, err)
		}
	}

	return user, version, questions
}

func mustJSONString(t *testing.T, value any) string {
	t.Helper()
	return string(mustMarshalJSON(t, value))
}

func mustJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	return json.RawMessage(mustMarshalJSON(t, value))
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return payload
}

func decodeMMSEJSONBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json body: %v body=%s", err, recorder.Body.String())
	}
	return payload
}

func assertStringSliceJSON(t *testing.T, raw any, expected []string) {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected []any payload, got %#v", raw)
	}
	if len(items) != len(expected) {
		t.Fatalf("expected %d items, got %#v", len(expected), items)
	}
	for index, want := range expected {
		if got, ok := items[index].(string); !ok || got != want {
			t.Fatalf("unexpected item at index %d: %#v", index, items[index])
		}
	}
}

func assertStringSliceNative(t *testing.T, raw any, expected []string) {
	t.Helper()
	items, ok := raw.([]string)
	if !ok {
		t.Fatalf("expected []string payload, got %#v", raw)
	}
	if len(items) != len(expected) {
		t.Fatalf("expected %d items, got %#v", len(expected), items)
	}
	for index, want := range expected {
		if items[index] != want {
			t.Fatalf("unexpected item at index %d: %#v", index, items[index])
		}
	}
}
