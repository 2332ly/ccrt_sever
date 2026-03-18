package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

type idiomRequest struct {
	Temperature float64 `json:"temperature"`
}

type lifeAssistantRequest struct {
	Query       string  `json:"query"`
	Temperature float64 `json:"temperature"`
}

type ttsRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

type asrOptions struct {
	Format      string
	SampleRate  int
	Punctuation bool
	InverseText bool
	VoiceDetect bool
}

type lifeAssistantAIResponse struct {
	Topic              string   `json:"topic"`
	Tip                string   `json:"tip"`
	Actions            []string `json:"actions"`
	Answer             string   `json:"answer"`
	QuickActions       []string `json:"quick_actions"`
	SuggestedQuestions []string `json:"suggested_questions"`
}

type voiceControlPlanRequest struct {
	Text             string                     `json:"text"`
	CurrentTab       string                     `json:"current_tab"`
	CurrentRoute     string                     `json:"current_route"`
	CanPop           bool                       `json:"can_pop"`
	AvailableActions []string                   `json:"available_actions"`
	AvailableTabs    []voiceControlTargetOption `json:"available_tabs"`
	AvailablePages   []voiceControlTargetOption `json:"available_pages"`
}

type voiceControlTargetOption struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Aliases []string `json:"aliases"`
}

const (
	voiceNotConfiguredCode      = "VOICE_NOT_CONFIGURED"
	voiceServiceUnavailableCode = "VOICE_SERVICE_UNAVAILABLE"
	maxAudioPayloadBytes        = 10 * 1024 * 1024

	voiceControlActionNone               = "no_action"
	voiceControlActionGoHome             = "go_home"
	voiceControlActionSwitchTab          = "switch_tab"
	voiceControlActionGoBack             = "go_back"
	voiceControlActionOpenPage           = "open_page"
	voiceControlActionAskLifeAssistant   = "ask_life_assistant"
	voiceControlActionCreateReminder     = "create_reminder"
	voiceControlActionOpenRemindersToday = "open_reminders_today"
	voiceControlActionCompleteReminder   = "complete_reminder"
)

var (
	dashScopeTTSBytesFn          = utils.DashScopeTTSBytes
	dashScopeTTSBytesWithModelFn = utils.DashScopeTTSBytesWithModel
	aliyunNLSASRFn               = utils.AliyunNLSASR
	lifeAssistantOpenAIChatFn    = utils.OpenAIChat
	openAIChatWithToolsFn        = utils.OpenAIChatWithTools
	isDashScopeConfiguredFn      = utils.IsDashScopeConfigured
	isAliyunNLSConfiguredFn      = utils.IsAliyunNLSConfigured
	getCurrentTTSUserFn          = getCurrentUser
	findDefaultVoiceProfileFn    = findDefaultVoiceProfile
	findVoiceProfileByVoiceIDFn  = findVoiceProfileByVendorVoiceID
	markVoiceProfileFailedFn     = markVoiceProfileFailed
	supportedVoiceControlTabs    = []voiceControlTargetOption{
		{ID: "home", Label: "首页", Aliases: []string{"首页", "主页", "home"}},
		{ID: "reminders", Label: "提醒", Aliases: []string{"提醒", "提醒页", "服药提醒", "用药提醒", "日常提醒", "训练提醒", "事项提醒", "reminders", "reminder"}},
		{ID: "mmse", Label: "MMSE", Aliases: []string{"mmse", "评估", "测评", "认知评估"}},
		{ID: "profile", Label: "我的", Aliases: []string{"我的", "个人中心", "profile"}},
	}
	supportedVoiceControlPages = []voiceControlTargetOption{
		{ID: "reminders", Label: "提醒中心", Aliases: []string{"提醒", "提醒页", "服药提醒", "用药提醒", "日常提醒", "训练提醒", "事项提醒", "reminders", "reminder", "medication_reminder", "medication_schedule"}},
		{ID: "assessment", Label: "去做评估", Aliases: []string{"评估", "测评", "认知评估", "去做评估", "assessment", "mmse", "mmse_assessment", "mmse_page"}},
		{ID: "life_assistant", Label: "日常助手", Aliases: []string{"日常助手", "生活助手", "life_assistant"}},
		{ID: "cognitive_profile", Label: "认知画像", Aliases: []string{"认知画像", "认知图像", "cognitive_profile"}},
		{ID: "training_effect", Label: "训练效果", Aliases: []string{"训练效果", "训练成果", "training_effect"}},
		{ID: "mmse_daily_home", Label: "做做游戏", Aliases: []string{"做做游戏", "评估大厅", "游戏大厅", "认知游戏", "mmse_daily_home"}},
	}
	defaultVoiceControlPageIDs = []string{
		"life_assistant",
		"cognitive_profile",
		"training_effect",
		"mmse_daily_home",
	}
)

// GenerateIdiom generates a four-character Chinese idiom via configured AI provider.
func GenerateIdiom(ctx *gin.Context) {
	var req idiomRequest
	_ = ctx.ShouldBindJSON(&req)

	temp := req.Temperature
	if temp <= 0 || temp > 2 {
		temp = 0.9
	}

	messages := []utils.OpenAIMessage{
		{
			Role:    "system",
			Content: "你是一名中文助手。请只输出一个四字成语，要求意象丰富、有画面感，优先自然/生活/四季/山水/光影等主题；避免生僻字、网络热词或英文；不要任何标点、解释或多余文字。",
		},
		{
			Role:    "user",
			Content: "请给我一个意象丰富、朗朗上口的四字成语。",
		},
	}

	content, model, err := lifeAssistantOpenAIChatFn(messages, temp)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", err.Error())
		return
	}

	idiom := pickFirstChinese(content, 4)
	if len([]rune(idiom)) < 4 {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", "invalid ai response")
		return
	}

	chars := splitRunes(idiom)
	utils.RespondOK(ctx, gin.H{
		"idiom": idiom,
		"chars": chars,
		"model": model,
	})
}

// GenerateLifeAssistantTip generates AI guidance for daily-life assistance.
func GenerateLifeAssistantTip(ctx *gin.Context) {
	var req lifeAssistantRequest
	_ = ctx.ShouldBindJSON(&req)

	temp := req.Temperature
	if temp <= 0 || temp > 2 {
		temp = 0.7
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = "daily habits"
	}

	messages := []utils.OpenAIMessage{
		{
			Role: "system",
			Content: "You are a helpful daily-life assistant for seniors. Respond in Simplified Chinese. " +
				"Return ONLY a valid JSON object with keys: answer, quick_actions, suggested_questions. " +
				"answer: <=80 Chinese characters, plain language, no markdown. " +
				"quick_actions: array of 2-3 short action items (<=8 Chinese chars each). " +
				"suggested_questions: array of 2-3 short follow-up questions (<=18 Chinese chars each). " +
				"No extra text.",
		},
		{
			Role:    "user",
			Content: "User request: " + query,
		},
	}

	content, model, err := lifeAssistantOpenAIChatFn(messages, temp)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", err.Error())
		return
	}

	res, err := parseLifeAssistantResponse(content)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", "invalid ai response")
		return
	}

	answer := strings.TrimSpace(res.Answer)
	if answer == "" {
		answer = strings.TrimSpace(res.Tip)
	}
	quickActions := normalizeActions(res.QuickActions)
	if len(quickActions) == 0 {
		quickActions = normalizeActions(res.Actions)
	}
	if len(quickActions) == 0 {
		quickActions = fallbackLifeAssistantQuickActions(query)
	}
	suggestedQuestions := normalizeQuestionSuggestions(res.SuggestedQuestions)
	if len(suggestedQuestions) == 0 {
		suggestedQuestions = fallbackLifeAssistantSuggestedQuestions(query)
	}
	if answer == "" {
		answer = buildFallbackLifeAssistantAnswer(query, quickActions)
	}

	topic := strings.TrimSpace(res.Topic)
	if topic == "" {
		topic = inferLifeAssistantTopic(query)
	}
	tip := strings.TrimSpace(res.Tip)
	if tip == "" {
		tip = answer
	}
	actions := normalizeActions(res.Actions)
	if len(actions) == 0 {
		actions = quickActions
	}

	utils.RespondOK(ctx, gin.H{
		"topic":               topic,
		"tip":                 tip,
		"actions":             actions,
		"answer":              answer,
		"quick_actions":       quickActions,
		"suggested_questions": suggestedQuestions,
		"model":               model,
	})
}

// GenerateTTS proxies DashScope TTS and returns an audio URL.
func GenerateTTS(ctx *gin.Context) {
	var req ttsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_PARAMS", "invalid payload")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_PARAMS", "text is required")
		return
	}
	if !isDashScopeConfiguredFn() {
		utils.RespondError(ctx, http.StatusServiceUnavailable, voiceNotConfiguredCode, "tts service is not configured")
		return
	}

	user, userErr := getCurrentTTSUserFn(ctx)
	selection := resolveTTSVoiceSelection(req.Voice, user, userErr == nil)

	data, contentType, model, voice, source, err := synthesizeWithFallback(text, selection)
	if err != nil {
		log.Printf("tts failed: %v", err)
		utils.RespondError(ctx, http.StatusBadGateway, voiceServiceUnavailableCode, "voice synthesis is unavailable")
		return
	}
	ctx.Header("X-TTS-Model", model)
	ctx.Header("X-TTS-Voice", voice)
	ctx.Header("X-TTS-Source", source)
	ctx.Data(http.StatusOK, contentType, data)
}

// GenerateASR proxies Aliyun NLS RESTful ASR and returns recognized text.
func GenerateASR(ctx *gin.Context) {
	opts := parseASROptions(ctx)
	if err := validateASROptions(opts); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_PARAMS", err.Error())
		return
	}
	audioData, err := readAudioPayload(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_PARAMS", err.Error())
		return
	}
	if !isAliyunNLSConfiguredFn() {
		utils.RespondError(ctx, http.StatusServiceUnavailable, voiceNotConfiguredCode, "asr service is not configured")
		return
	}

	text, _, err := aliyunNLSASRFn(audioData, opts.Format, opts.SampleRate, utils.NLSASROptions{
		EnablePunctuationPrediction:    opts.Punctuation,
		EnableInverseTextNormalization: opts.InverseText,
		EnableVoiceDetection:           opts.VoiceDetect,
	})
	if err != nil {
		log.Printf("asr failed: %v", err)
		utils.RespondError(ctx, http.StatusBadGateway, voiceServiceUnavailableCode, "voice recognition is unavailable")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"text":        text,
		"vendor":      "aliyun_nls",
		"format":      opts.Format,
		"sample_rate": opts.SampleRate,
	})
}

// PlanVoiceControl turns a recognized utterance into a normalized in-app action.
func PlanVoiceControl(ctx *gin.Context) {
	var req voiceControlPlanRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_PARAMS", "invalid payload")
		return
	}

	providedActions := len(req.AvailableActions) > 0
	req.Text = strings.TrimSpace(req.Text)
	req.CurrentTab = normalizeVoiceControlToken(req.CurrentTab)
	req.CurrentRoute = strings.TrimSpace(req.CurrentRoute)
	req.AvailableActions = normalizeVoiceControlActionList(req.AvailableActions)
	availableTabs := normalizeVoiceControlTargetOptions(
		req.AvailableTabs,
		supportedVoiceControlTabs,
		nil,
	)
	availablePages := normalizeVoiceControlTargetOptions(
		req.AvailablePages,
		supportedVoiceControlPages,
		defaultVoiceControlPageIDs,
	)

	if req.Text == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_PARAMS", "text is required")
		return
	}

	if providedActions && len(req.AvailableActions) == 0 {
		utils.RespondOK(ctx, gin.H{
			"reply_text":  "当前没有可执行的语音控制动作。",
			"action":      voiceControlActionNone,
			"action_args": gin.H{},
			"confidence":  0.0,
			"model":       "",
		})
		return
	}

	tools := buildVoiceControlToolsV2(req.AvailableActions, availableTabs, availablePages)
	if len(tools) == 0 {
		utils.RespondOK(ctx, gin.H{
			"reply_text":  "当前没有可执行的语音控制动作。",
			"action":      voiceControlActionNone,
			"action_args": gin.H{},
			"confidence":  0.0,
			"model":       "",
		})
		return
	}

	contextPayload, err := json.Marshal(gin.H{
		"text":              req.Text,
		"current_tab":       req.CurrentTab,
		"current_route":     req.CurrentRoute,
		"can_pop":           req.CanPop,
		"available_actions": req.AvailableActions,
		"available_tabs":    availableTabs,
		"available_pages":   availablePages,
	})
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "SERVER_ERROR", "could not build planner context")
		return
	}

	messages := []utils.OpenAIMessage{
		{
			Role:    "system",
			Content: buildVoiceControlSystemPrompt(availableTabs, availablePages),
		},
		{
			Role:    "user",
			Content: string(contextPayload),
		},
	}

	result, err := openAIChatWithToolsFn(messages, tools, 0.1)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", err.Error())
		return
	}

	replyText, action, actionArgs, confidence := normalizeVoiceControlResultV2(
		result,
		req.AvailableActions,
		availableTabs,
		availablePages,
	)
	utils.RespondOK(ctx, gin.H{
		"reply_text":  replyText,
		"action":      action,
		"action_args": actionArgs,
		"confidence":  confidence,
		"model":       result.Model,
	})
}

func pickFirstChinese(text string, n int) string {
	if n <= 0 {
		return ""
	}
	var out []rune
	for _, r := range strings.TrimSpace(text) {
		if r >= '\u4e00' && r <= '\u9fa5' {
			out = append(out, r)
			if len(out) >= n {
				break
			}
		}
	}
	return string(out)
}

func splitRunes(s string) []string {
	r := []rune(s)
	out := make([]string, 0, len(r))
	for _, ch := range r {
		out = append(out, string(ch))
	}
	return out
}

func parseLifeAssistantResponse(content string) (lifeAssistantAIResponse, error) {
	var res lifeAssistantAIResponse
	payload := strings.TrimSpace(content)
	if err := json.Unmarshal([]byte(payload), &res); err == nil {
		return res, nil
	}

	start := strings.Index(payload, "{")
	end := strings.LastIndex(payload, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(payload[start:end+1]), &res); err == nil {
			return res, nil
		}
	}

	return lifeAssistantAIResponse{}, errors.New("invalid ai response")
}

func parseASROptions(ctx *gin.Context) asrOptions {
	format := strings.TrimSpace(ctx.PostForm("format"))
	if format == "" {
		format = strings.TrimSpace(ctx.Query("format"))
	}
	if format == "" {
		format = "wav"
	}
	format = strings.ToLower(format)
	sampleRate := parseInt(ctx.PostForm("sample_rate"))
	if sampleRate <= 0 {
		sampleRate = parseInt(ctx.Query("sample_rate"))
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}

	return asrOptions{
		Format:      format,
		SampleRate:  sampleRate,
		Punctuation: parseBoolDefault(ctx, "enable_punctuation_prediction", true),
		InverseText: parseBoolDefault(ctx, "enable_inverse_text_normalization", true),
		VoiceDetect: parseBoolDefault(ctx, "enable_voice_detection", true),
	}
}

func validateASROptions(opts asrOptions) error {
	switch opts.Format {
	case "wav", "pcm", "mp3", "aac", "amr", "opus", "speex":
	default:
		return errors.New("unsupported audio format")
	}
	switch opts.SampleRate {
	case 8000, 16000:
	default:
		return errors.New("unsupported sample rate")
	}
	return nil
}

func parseInt(raw string) int {
	val := strings.TrimSpace(raw)
	if val == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(val)
	return parsed
}

func parseBoolDefault(ctx *gin.Context, key string, def bool) bool {
	raw := strings.TrimSpace(ctx.PostForm(key))
	if raw == "" {
		raw = strings.TrimSpace(ctx.Query(key))
	}
	if raw == "" {
		return def
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return parsed
}

func readAudioPayload(ctx *gin.Context) ([]byte, error) {
	if file, err := ctx.FormFile("audio"); err == nil && file != nil {
		if file.Size > maxAudioPayloadBytes {
			return nil, errors.New("audio is too large")
		}
		f, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxAudioPayloadBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxAudioPayloadBytes {
			return nil, errors.New("audio is too large")
		}
		if len(data) == 0 {
			return nil, errors.New("audio is empty")
		}
		return data, nil
	}

	if ctx.Request.Body == nil {
		return nil, errors.New("audio is required")
	}
	data, err := io.ReadAll(io.LimitReader(ctx.Request.Body, maxAudioPayloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAudioPayloadBytes {
		return nil, errors.New("audio is too large")
	}
	if len(data) == 0 {
		return nil, errors.New("audio is empty")
	}
	return data, nil
}

func normalizeActions(actions []string) []string {
	out := make([]string, 0, len(actions))
	for _, item := range actions {
		val := strings.TrimSpace(item)
		if val == "" {
			continue
		}
		out = append(out, val)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func normalizeQuestionSuggestions(items []string) []string {
	out := make([]string, 0, 3)
	for _, item := range items {
		val := strings.TrimSpace(item)
		if val == "" {
			continue
		}
		if !strings.HasSuffix(val, "？") && !strings.HasSuffix(val, "?") {
			val += "？"
		}
		out = append(out, val)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func inferLifeAssistantTopic(query string) string {
	switch {
	case strings.Contains(query, "睡") || strings.Contains(query, "失眠") || strings.Contains(query, "早醒"):
		return "睡眠建议"
	case strings.Contains(query, "吃") || strings.Contains(query, "饮食") || strings.Contains(query, "饭"):
		return "饮食安排"
	case strings.Contains(query, "记") || strings.Contains(query, "忘") || strings.Contains(query, "提醒"):
		return "记事提醒"
	case strings.Contains(query, "出门") || strings.Contains(query, "外出") || strings.Contains(query, "散步"):
		return "外出准备"
	default:
		return "生活助手"
	}
}

func fallbackLifeAssistantQuickActions(query string) []string {
	switch {
	case strings.Contains(query, "睡") || strings.Contains(query, "失眠") || strings.Contains(query, "早醒"):
		return []string{"固定入睡时间", "睡前放松", "减少午后浓茶"}
	case strings.Contains(query, "吃") || strings.Contains(query, "饮食") || strings.Contains(query, "饭"):
		return []string{"规律三餐", "少量多次", "记得喝水"}
	case strings.Contains(query, "记") || strings.Contains(query, "忘") || strings.Contains(query, "提醒"):
		return []string{"先写下来", "设置提醒", "和家人确认"}
	case strings.Contains(query, "出门") || strings.Contains(query, "外出") || strings.Contains(query, "散步"):
		return []string{"检查随身物品", "先喝几口水", "活动别太久"}
	default:
		return []string{"先喝点水", "慢一点做", "需要时找家人"}
	}
}

func fallbackLifeAssistantSuggestedQuestions(query string) []string {
	switch {
	case strings.Contains(query, "睡") || strings.Contains(query, "失眠") || strings.Contains(query, "早醒"):
		return []string{"午休多久比较合适？", "睡前可以做什么放松？"}
	case strings.Contains(query, "吃") || strings.Contains(query, "饮食") || strings.Contains(query, "饭"):
		return []string{"早餐吃什么更稳妥？", "喝水怎么分配更容易坚持？"}
	case strings.Contains(query, "记") || strings.Contains(query, "忘") || strings.Contains(query, "提醒"):
		return []string{"今天最重要的事怎么记？", "提醒要设在几点更合适？"}
	default:
		return []string{"今天适合安排什么活动？", "我现在先做哪一步比较好？"}
	}
}

func buildFallbackLifeAssistantAnswer(query string, quickActions []string) string {
	switch {
	case strings.Contains(query, "睡") || strings.Contains(query, "失眠") || strings.Contains(query, "早醒"):
		return "今晚先把入睡时间固定下来，睡前半小时少看屏幕，再做一次慢呼吸放松。"
	case strings.Contains(query, "吃") || strings.Contains(query, "饮食") || strings.Contains(query, "饭") || strings.Contains(query, "胃口"):
		return "先按清淡、规律、少量多次来安排，今天把喝水和三餐时间尽量固定。"
	case strings.Contains(query, "记") || strings.Contains(query, "忘") || strings.Contains(query, "提醒"):
		return "先把最重要的一件事写下来，再配一个固定时间提醒，会更容易记住。"
	case strings.Contains(query, "出门") || strings.Contains(query, "外出") || strings.Contains(query, "散步"):
		return "出门前先确认钥匙、手机、纸巾和水，再按体力安排短时间活动。"
	default:
		steps := normalizeActions(quickActions)
		if len(steps) == 0 {
			steps = fallbackLifeAssistantQuickActions(query)
		}
		if len(steps) > 2 {
			steps = steps[:2]
		}
		return "先从这几步开始：" + strings.Join(steps, "、") + "。一步一步做，比一次做很多更轻松。"
	}
}

func buildVoiceControlTools(availableActions []string) []utils.OpenAITool {
	allowed := make(map[string]struct{}, len(availableActions))
	for _, action := range availableActions {
		allowed[action] = struct{}{}
	}
	isAllowed := func(action string) bool {
		if len(allowed) == 0 {
			return true
		}
		_, ok := allowed[action]
		return ok
	}

	tools := make([]utils.OpenAITool, 0, 7)
	if isAllowed(voiceControlActionGoHome) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionGoHome,
				Description: "返回应用主页",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		})
	}
	if isAllowed(voiceControlActionSwitchTab) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionSwitchTab,
				Description: "切换到底部标签页",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tab": map[string]any{
							"type": "string",
							"enum": []string{"home", "reminders", "mmse", "profile"},
						},
					},
					"required": []string{"tab"},
				},
			},
		})
	}
	if isAllowed(voiceControlActionGoBack) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionGoBack,
				Description: "返回上一页",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		})
	}
	if isAllowed(voiceControlActionOpenPage) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionOpenPage,
				Description: "打开应用内的常用页面",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"page": map[string]any{
							"type": "string",
							"enum": []string{
								"life_assistant",
								"cognitive_profile",
								"training_effect",
								"mmse_daily_home",
							},
						},
					},
					"required": []string{"page"},
				},
			},
		})
	}
	return tools
}

func normalizeVoiceControlResult(
	result utils.OpenAIChatResult,
	availableActions []string,
) (string, string, gin.H, float64) {
	allowed := make(map[string]struct{}, len(availableActions))
	for _, action := range availableActions {
		allowed[action] = struct{}{}
	}
	isAllowed := func(action string) bool {
		if len(allowed) == 0 {
			return true
		}
		_, ok := allowed[action]
		return ok
	}

	replyText := strings.TrimSpace(result.Content)
	for _, call := range result.ToolCalls {
		action, actionArgs, ok := normalizeVoiceControlToolCall(call, isAllowed)
		if !ok {
			continue
		}
		if replyText == "" {
			replyText = defaultVoiceControlReply(action, actionArgs)
		}
		return replyText, action, actionArgs, 0.92
	}

	if replyText == "" {
		replyText = "我暂时不能执行这个语音指令。"
	}
	return replyText, voiceControlActionNone, gin.H{}, 0.18
}

func normalizeVoiceControlToolCall(
	call utils.OpenAIToolCall,
	isAllowed func(string) bool,
) (string, gin.H, bool) {
	name := normalizeVoiceControlToken(call.Function.Name)
	if name == "" || !isAllowed(name) {
		return "", nil, false
	}

	rawArgs := strings.TrimSpace(call.Function.Arguments)
	if rawArgs == "" {
		rawArgs = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", nil, false
	}

	switch name {
	case voiceControlActionGoHome, voiceControlActionGoBack:
		return name, gin.H{}, true
	case voiceControlActionSwitchTab:
		tab := normalizeVoiceControlToken(toString(args["tab"]))
		switch tab {
		case "home", "reminders", "mmse", "profile":
			return name, gin.H{"tab": tab}, true
		default:
			return "", nil, false
		}
	case voiceControlActionOpenPage:
		page := normalizeVoiceControlToken(toString(args["page"]))
		switch page {
		case "life_assistant", "cognitive_profile", "training_effect", "mmse_daily_home":
			return name, gin.H{"page": page}, true
		default:
			return "", nil, false
		}
	default:
		return "", nil, false
	}
}

func normalizeVoiceControlActionList(actions []string) []string {
	if len(actions) == 0 {
		return []string{
			voiceControlActionGoHome,
			voiceControlActionSwitchTab,
			voiceControlActionGoBack,
			voiceControlActionOpenPage,
			voiceControlActionAskLifeAssistant,
			voiceControlActionCreateReminder,
			voiceControlActionOpenRemindersToday,
			voiceControlActionCompleteReminder,
		}
	}

	out := make([]string, 0, len(actions))
	seen := map[string]struct{}{}
	for _, action := range actions {
		normalized := normalizeVoiceControlToken(action)
		switch normalized {
		case voiceControlActionGoHome,
			voiceControlActionSwitchTab,
			voiceControlActionGoBack,
			voiceControlActionOpenPage,
			voiceControlActionAskLifeAssistant,
			voiceControlActionCreateReminder,
			voiceControlActionOpenRemindersToday,
			voiceControlActionCompleteReminder:
		default:
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeVoiceControlToken(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func defaultVoiceControlReply(action string, actionArgs gin.H) string {
	switch action {
	case voiceControlActionGoHome:
		return "正在返回主页。"
	case voiceControlActionGoBack:
		return "正在返回上一页。"
	case voiceControlActionSwitchTab:
		return "正在切换到" + voiceControlTabLabel(toString(actionArgs["tab"])) + "。"
	case voiceControlActionOpenPage:
		return "正在打开" + voiceControlPageLabel(toString(actionArgs["page"])) + "。"
	default:
		return "我暂时不能执行这个语音指令。"
	}
}

func voiceControlTabLabel(tab string) string {
	switch normalizeVoiceControlToken(tab) {
	case "home":
		return "主页"
	case "reminders":
		return "服药提醒"
	case "mmse":
		return "评估"
	case "profile":
		return "我的"
	default:
		return "目标页面"
	}
}

func voiceControlPageLabel(page string) string {
	switch normalizeVoiceControlToken(page) {
	case "life_assistant":
		return "生活助手"
	case "cognitive_profile":
		return "认知画像"
	case "training_effect":
		return "训练效果"
	case "mmse_daily_home":
		return "评估大厅"
	default:
		return "页面"
	}
}

func normalizeVoiceControlTargetOptions(
	items []voiceControlTargetOption,
	supported []voiceControlTargetOption,
	defaultIDs []string,
) []voiceControlTargetOption {
	if len(items) == 0 {
		return pickDefaultVoiceControlTargets(supported, defaultIDs)
	}

	supportedByID := make(map[string]voiceControlTargetOption, len(supported))
	for _, item := range supported {
		supportedByID[item.ID] = item
	}

	out := make([]voiceControlTargetOption, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := normalizeVoiceControlToken(item.ID)
		base, ok := supportedByID[id]
		if !ok {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		label := strings.TrimSpace(item.Label)
		if label == "" {
			label = base.Label
		}
		aliases := mergeVoiceControlAliases(label, id, item.Aliases, base.Aliases)
		out = append(out, voiceControlTargetOption{
			ID:      id,
			Label:   label,
			Aliases: aliases,
		})
	}

	if len(out) == 0 {
		return pickDefaultVoiceControlTargets(supported, defaultIDs)
	}
	return out
}

func pickDefaultVoiceControlTargets(
	supported []voiceControlTargetOption,
	defaultIDs []string,
) []voiceControlTargetOption {
	if len(defaultIDs) == 0 {
		out := make([]voiceControlTargetOption, 0, len(supported))
		for _, item := range supported {
			out = append(out, voiceControlTargetOption{
				ID:      item.ID,
				Label:   item.Label,
				Aliases: append([]string(nil), item.Aliases...),
			})
		}
		return out
	}

	supportedByID := make(map[string]voiceControlTargetOption, len(supported))
	for _, item := range supported {
		supportedByID[item.ID] = item
	}

	out := make([]voiceControlTargetOption, 0, len(defaultIDs))
	for _, id := range defaultIDs {
		item, ok := supportedByID[id]
		if !ok {
			continue
		}
		out = append(out, voiceControlTargetOption{
			ID:      item.ID,
			Label:   item.Label,
			Aliases: append([]string(nil), item.Aliases...),
		})
	}
	return out
}

func mergeVoiceControlAliases(label, id string, aliasGroups ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	appendAlias := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		normalized := normalizeVoiceControlToken(trimmed)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, trimmed)
	}

	appendAlias(label)
	appendAlias(id)
	for _, group := range aliasGroups {
		for _, alias := range group {
			appendAlias(alias)
		}
	}
	return out
}

func buildVoiceControlSystemPrompt(
	availableTabs []voiceControlTargetOption,
	availablePages []voiceControlTargetOption,
) string {
	var builder strings.Builder
	builder.WriteString("你是一个内置在移动应用中的语音控制代理。")
	builder.WriteString("只在用户意图明确且属于导航或提醒管理能力时调用一个工具。")
	builder.WriteString("你只能执行返回首页、切换底部标签、返回上一页、打开前端提供的页面目标，以及创建提醒、查看今日提醒、完成提醒。")
	if summary := summarizeVoiceControlTargets("当前可切换的标签", availableTabs); summary != "" {
		builder.WriteString(summary)
	}
	if summary := summarizeVoiceControlTargets("当前可打开的页面", availablePages); summary != "" {
		builder.WriteString(summary)
	}
	builder.WriteString("优先使用前端提供的 label、aliases 和 canonical id。")
	builder.WriteString("如果用户说法接近某个 alias，就映射到对应 canonical id，不要臆造未提供的标签或页面。")
	builder.WriteString("创建提醒时请提取 reminder_type、title、time、date、repeat_rule、note；time 需要规范成 HH:MM，date 使用 YYYY-MM-DD。")
	builder.WriteString("完成提醒时优先提取 ordinal、title 或 time；如果用户说的是跳过，status 设为 skipped，否则设为 done。")
	builder.WriteString("如果用户意图不清、超出导航和提醒范围、涉及危险操作，就不要调用工具，并给出一句简短中文说明。")
	return builder.String()
}

func summarizeVoiceControlTargets(
	title string,
	items []voiceControlTargetOption,
) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf(
			"%s（id=%s，alias=%s）",
			item.Label,
			item.ID,
			strings.Join(previewVoiceControlAliases(item.Aliases, item.Label, item.ID), " / "),
		))
	}
	return title + "：" + strings.Join(parts, "；") + "。"
}

func previewVoiceControlAliases(
	aliases []string,
	label string,
	id string,
) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	appendAlias := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		normalized := normalizeVoiceControlToken(trimmed)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, trimmed)
	}
	appendAlias(label)
	appendAlias(id)
	for _, alias := range aliases {
		if len(out) >= 4 {
			break
		}
		appendAlias(alias)
	}
	return out
}

func voiceControlOptionIDs(items []voiceControlTargetOption) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func canonicalVoiceControlTargetID(raw string, items []voiceControlTargetOption) string {
	normalized := normalizeVoiceControlToken(raw)
	if normalized == "" {
		return ""
	}
	for _, item := range items {
		if normalized == item.ID {
			return item.ID
		}
		if normalizeVoiceControlToken(item.Label) == normalized {
			return item.ID
		}
		for _, alias := range item.Aliases {
			if normalizeVoiceControlToken(alias) == normalized {
				return item.ID
			}
		}
	}
	return ""
}

func voiceControlTargetLabelFromOptions(
	id string,
	items []voiceControlTargetOption,
	fallback string,
) string {
	normalized := normalizeVoiceControlToken(id)
	for _, item := range items {
		if item.ID == normalized {
			return item.Label
		}
	}
	return fallback
}

func buildVoiceControlToolsV2(
	availableActions []string,
	availableTabs []voiceControlTargetOption,
	availablePages []voiceControlTargetOption,
) []utils.OpenAITool {
	allowed := make(map[string]struct{}, len(availableActions))
	for _, action := range availableActions {
		allowed[action] = struct{}{}
	}
	isAllowed := func(action string) bool {
		if len(allowed) == 0 {
			return true
		}
		_, ok := allowed[action]
		return ok
	}

	tools := make([]utils.OpenAITool, 0, 4)
	if isAllowed(voiceControlActionGoHome) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionGoHome,
				Description: "返回应用首页",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		})
	}
	if isAllowed(voiceControlActionSwitchTab) && len(availableTabs) > 0 {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionSwitchTab,
				Description: "切换到底部标签页",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tab": map[string]any{
							"type": "string",
							"enum": voiceControlOptionIDs(availableTabs),
						},
					},
					"required": []string{"tab"},
				},
			},
		})
	}
	if isAllowed(voiceControlActionGoBack) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionGoBack,
				Description: "返回上一页",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		})
	}
	if isAllowed(voiceControlActionOpenPage) && len(availablePages) > 0 {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionOpenPage,
				Description: "打开应用内的常用页面",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"page": map[string]any{
							"type": "string",
							"enum": voiceControlOptionIDs(availablePages),
						},
					},
					"required": []string{"page"},
				},
			},
		})
	}
	if isAllowed(voiceControlActionAskLifeAssistant) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionAskLifeAssistant,
				Description: "把用户的自然语言问题转给生活助手页面并自动提问",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "用户要提问给生活助手的完整问题",
						},
					},
					"required": []string{"query"},
				},
			},
		})
	}
	if isAllowed(voiceControlActionCreateReminder) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionCreateReminder,
				Description: "创建一个新的提醒草稿",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"reminder_type": map[string]any{
							"type": "string",
							"enum": []string{"medication", "habit", "training", "task"},
						},
						"title": map[string]any{
							"type": "string",
						},
						"time": map[string]any{
							"type":        "string",
							"description": "24小时制 HH:MM",
						},
						"date": map[string]any{
							"type":        "string",
							"description": "可选，格式 YYYY-MM-DD",
						},
						"repeat_rule": map[string]any{
							"type": "string",
							"enum": []string{"once", "daily"},
						},
						"note": map[string]any{
							"type": "string",
						},
					},
					"required": []string{"reminder_type", "title", "time"},
				},
			},
		})
	}
	if isAllowed(voiceControlActionOpenRemindersToday) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionOpenRemindersToday,
				Description: "打开提醒页并查看今天的提醒",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		})
	}
	if isAllowed(voiceControlActionCompleteReminder) {
		tools = append(tools, utils.OpenAITool{
			Type: "function",
			Function: utils.OpenAIFunctionSpec{
				Name:        voiceControlActionCompleteReminder,
				Description: "完成或跳过一条今天的提醒",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ordinal": map[string]any{
							"type": "integer",
						},
						"title": map[string]any{
							"type": "string",
						},
						"time": map[string]any{
							"type":        "string",
							"description": "可选，格式 HH:MM",
						},
						"status": map[string]any{
							"type": "string",
							"enum": []string{"done", "skipped"},
						},
					},
				},
			},
		})
	}
	return tools
}

func normalizeVoiceControlResultV2(
	result utils.OpenAIChatResult,
	availableActions []string,
	availableTabs []voiceControlTargetOption,
	availablePages []voiceControlTargetOption,
) (string, string, gin.H, float64) {
	allowed := make(map[string]struct{}, len(availableActions))
	for _, action := range availableActions {
		allowed[action] = struct{}{}
	}
	isAllowed := func(action string) bool {
		if len(allowed) == 0 {
			return true
		}
		_, ok := allowed[action]
		return ok
	}

	replyText := strings.TrimSpace(result.Content)
	for _, call := range result.ToolCalls {
		action, actionArgs, ok := normalizeVoiceControlToolCallV2(
			call,
			isAllowed,
			availableTabs,
			availablePages,
		)
		if !ok {
			continue
		}
		if replyText == "" {
			replyText = defaultVoiceControlReplyV2(
				action,
				actionArgs,
				availableTabs,
				availablePages,
			)
		}
		return replyText, action, actionArgs, 0.92
	}

	if replyText == "" {
		replyText = "我暂时不能执行这个语音指令。"
	}
	return replyText, voiceControlActionNone, gin.H{}, 0.18
}

func normalizeVoiceControlToolCallV2(
	call utils.OpenAIToolCall,
	isAllowed func(string) bool,
	availableTabs []voiceControlTargetOption,
	availablePages []voiceControlTargetOption,
) (string, gin.H, bool) {
	name := normalizeVoiceControlToken(call.Function.Name)
	if name == "" || !isAllowed(name) {
		return "", nil, false
	}

	rawArgs := strings.TrimSpace(call.Function.Arguments)
	if rawArgs == "" {
		rawArgs = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", nil, false
	}

	switch name {
	case voiceControlActionGoHome, voiceControlActionGoBack:
		return name, gin.H{}, true
	case voiceControlActionSwitchTab:
		tab := canonicalVoiceControlTargetID(toString(args["tab"]), availableTabs)
		if tab == "" {
			return "", nil, false
		}
		return name, gin.H{"tab": tab}, true
	case voiceControlActionOpenPage:
		page := canonicalVoiceControlTargetID(toString(args["page"]), availablePages)
		if page == "" {
			return "", nil, false
		}
		return name, gin.H{"page": page}, true
	case voiceControlActionAskLifeAssistant:
		query := strings.TrimSpace(toString(args["query"]))
		if query == "" {
			return "", nil, false
		}
		return name, gin.H{"query": query}, true
	case voiceControlActionCreateReminder:
		reminderType := normalizeReminderType(toString(args["reminder_type"]))
		title := strings.TrimSpace(toString(args["title"]))
		timeText := normalizeVoiceReminderTime(toString(args["time"]))
		if title == "" || timeText == "" {
			return "", nil, false
		}
		dateText := normalizeVoiceReminderDate(toString(args["date"]))
		repeatRule := strings.ToLower(strings.TrimSpace(toString(args["repeat_rule"])))
		if repeatRule == "" {
			if dateText != "" {
				repeatRule = reminderRepeatOnce
			} else {
				repeatRule = reminderRepeatDaily
			}
		}
		if repeatRule != reminderRepeatOnce && repeatRule != reminderRepeatDaily {
			return "", nil, false
		}
		actionArgs := gin.H{
			"reminder_type": reminderType,
			"title":         title,
			"time":          timeText,
			"repeat_rule":   repeatRule,
		}
		if dateText != "" {
			actionArgs["date"] = dateText
		}
		note := strings.TrimSpace(toString(args["note"]))
		if note != "" {
			actionArgs["note"] = note
		}
		return name, actionArgs, true
	case voiceControlActionOpenRemindersToday:
		return name, gin.H{}, true
	case voiceControlActionCompleteReminder:
		actionArgs := gin.H{}
		if ordinal := parseVoiceReminderOrdinal(args["ordinal"]); ordinal > 0 {
			actionArgs["ordinal"] = ordinal
		}
		if title := strings.TrimSpace(toString(args["title"])); title != "" {
			actionArgs["title"] = title
		}
		if timeText := normalizeVoiceReminderTime(toString(args["time"])); timeText != "" {
			actionArgs["time"] = timeText
		}
		status := strings.ToLower(strings.TrimSpace(toString(args["status"])))
		if status == "" {
			status = reminderStatusDone
		}
		if status != reminderStatusDone && status != reminderStatusSkipped {
			return "", nil, false
		}
		if len(actionArgs) == 0 {
			return "", nil, false
		}
		actionArgs["status"] = status
		return name, actionArgs, true
	default:
		return "", nil, false
	}
}

func defaultVoiceControlReplyV2(
	action string,
	actionArgs gin.H,
	availableTabs []voiceControlTargetOption,
	availablePages []voiceControlTargetOption,
) string {
	switch action {
	case voiceControlActionGoHome:
		return "正在返回首页。"
	case voiceControlActionGoBack:
		return "正在返回上一页。"
	case voiceControlActionSwitchTab:
		return "正在切换到" + voiceControlTargetLabelFromOptions(
			toString(actionArgs["tab"]),
			availableTabs,
			"目标标签",
		) + "。"
	case voiceControlActionOpenPage:
		return "正在打开" + voiceControlTargetLabelFromOptions(
			toString(actionArgs["page"]),
			availablePages,
			"目标页面",
		) + "。"
	case voiceControlActionAskLifeAssistant:
		return "正在为您打开生活助手。"
	case voiceControlActionCreateReminder:
		return "正在打开提醒页，请确认新的提醒。"
	case voiceControlActionOpenRemindersToday:
		return "正在打开今天的提醒。"
	case voiceControlActionCompleteReminder:
		if strings.ToLower(strings.TrimSpace(toString(actionArgs["status"]))) == reminderStatusSkipped {
			return "正在跳过这条提醒。"
		}
		return "正在完成这条提醒。"
	default:
		return "我暂时不能执行这个语音指令。"
	}
}

func normalizeVoiceReminderTime(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return ""
	}
	return parsed.Format("15:04")
}

func normalizeVoiceReminderDate(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return ""
	}
	return parsed.Format("2006-01-02")
}

func parseVoiceReminderOrdinal(value any) int {
	switch current := value.(type) {
	case int:
		return current
	case int32:
		return int(current)
	case int64:
		return int(current)
	case float64:
		return int(current)
	case json.Number:
		parsed, err := current.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		parsed, _ := strconv.Atoi(strings.TrimSpace(toString(value)))
		return parsed
	}
}

func toString(value any) string {
	switch val := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(val)
	case []byte:
		return strings.TrimSpace(string(val))
	case interface{ String() string }:
		return strings.TrimSpace(val.String())
	default:
		return strings.TrimSpace(fmt.Sprint(val))
	}
}

type ttsVoiceSelection struct {
	Voice           string
	Model           string
	Source          string
	ProfileID       uint
	NeedsFallback   bool
	HasVoiceProfile bool
}

func resolveTTSVoiceSelection(requestedVoice string, user models.User, hasUser bool) ttsVoiceSelection {
	cleanedVoice := strings.TrimSpace(requestedVoice)
	if cleanedVoice != "" {
		if hasUser {
			if profile, found, err := findVoiceProfileByVoiceIDFn(user.ID, cleanedVoice); err == nil && found {
				if profile.Status == voiceStatusReady && strings.TrimSpace(profile.VendorVoiceID) != "" {
					return ttsVoiceSelection{
						Voice:           profile.VendorVoiceID,
						Model:           utils.DashScopeVoiceCloneModel(),
						Source:          "custom",
						ProfileID:       profile.ID,
						HasVoiceProfile: true,
					}
				}
				return ttsVoiceSelection{
					Source:          "fallback",
					ProfileID:       profile.ID,
					NeedsFallback:   true,
					HasVoiceProfile: true,
				}
			}
		}
		return ttsVoiceSelection{Voice: cleanedVoice, Source: "system"}
	}
	if hasUser {
		if profile, found, err := findDefaultVoiceProfileFn(user.ID); err == nil && found {
			if profile.Status == voiceStatusReady && strings.TrimSpace(profile.VendorVoiceID) != "" {
				return ttsVoiceSelection{
					Voice:           profile.VendorVoiceID,
					Model:           utils.DashScopeVoiceCloneModel(),
					Source:          "custom",
					ProfileID:       profile.ID,
					HasVoiceProfile: true,
				}
			}
			return ttsVoiceSelection{
				Source:          "fallback",
				ProfileID:       profile.ID,
				NeedsFallback:   true,
				HasVoiceProfile: true,
			}
		}
	}
	return ttsVoiceSelection{Source: "system"}
}

func synthesizeWithFallback(text string, selection ttsVoiceSelection) ([]byte, string, string, string, string, error) {
	if selection.Source == "custom" && !selection.NeedsFallback {
		data, contentType, model, voice, err := dashScopeTTSBytesWithModelFn(text, selection.Voice, selection.Model)
		if err == nil {
			return data, contentType, model, voice, "custom", nil
		}
		if selection.ProfileID > 0 && isDashScopeVoiceUnavailableError(err) {
			_ = markVoiceProfileFailedFn(selection.ProfileID, err.Error())
		}
		data, contentType, model, voice, fallbackErr := dashScopeTTSBytesFn(text, "")
		if fallbackErr != nil {
			return nil, "", "", "", "", fallbackErr
		}
		return data, contentType, model, voice, "fallback", nil
	}

	if selection.NeedsFallback {
		data, contentType, model, voice, err := dashScopeTTSBytesFn(text, "")
		if err != nil {
			return nil, "", "", "", "", err
		}
		return data, contentType, model, voice, "fallback", nil
	}

	data, contentType, model, voice, err := dashScopeTTSBytesFn(text, selection.Voice)
	if err != nil {
		return nil, "", "", "", "", err
	}
	return data, contentType, model, voice, "system", nil
}

func isDashScopeVoiceUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "voice not exists") ||
		strings.Contains(msg, "voice does not exist") ||
		strings.Contains(msg, "permission")
}
