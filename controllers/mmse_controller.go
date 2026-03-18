package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var openAIChatFn = utils.OpenAIChat

// GetMMSEScale 获取当前激活量表版本与题目
func GetMMSEScale(ctx *gin.Context) {
	var version models.ScaleVersion
	if err := global.Db.Preload("Scale").
		Preload("Modules", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order asc")
		}).
		Preload("Modules.Questions", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order asc")
		}).
		Where("is_active = ?", true).
		First(&version).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "active scale version not found")
		return
	}

	type questionResp struct {
		ID       uint   `json:"id"`
		Type     string `json:"type"`
		Content  string `json:"content"`
		MaxScore int    `json:"max_score"`
		Options  any    `json:"options,omitempty"`
	}
	type moduleResp struct {
		ID        uint           `json:"id"`
		Name      string         `json:"name"`
		MaxScore  int            `json:"max_score"`
		SortOrder int            `json:"sort_order"`
		Questions []questionResp `json:"questions"`
	}

	modules := make([]moduleResp, 0, len(version.Modules))
	for _, m := range version.Modules {
		qr := make([]questionResp, 0, len(m.Questions))
		for _, q := range m.Questions {
			options := buildQuestionOptions(q)
			qr = append(qr, questionResp{
				ID:       q.ID,
				Type:     q.Type,
				Content:  q.Content,
				MaxScore: q.MaxScore,
				Options:  options,
			})
		}
		modules = append(modules, moduleResp{
			ID:        m.ID,
			Name:      m.Name,
			MaxScore:  m.MaxScore,
			SortOrder: m.SortOrder,
			Questions: qr,
		})
	}

	utils.RespondOK(ctx, gin.H{
		"scale": gin.H{
			"id":          version.ScaleID,
			"name":        version.Scale.Name,
			"total_score": version.TotalScore,
		},
		"version": gin.H{
			"id":          version.ID,
			"name":        version.Name,
			"version":     version.Version,
			"total_score": version.TotalScore,
			"is_active":   version.IsActive,
		},
		"modules": modules,
	})
}

func buildQuestionOptions(q models.ScaleQuestion) any {
	if strings.TrimSpace(q.AnswerRule) == "" {
		return nil
	}
	var rule map[string]any
	if err := json.Unmarshal([]byte(q.AnswerRule), &rule); err != nil {
		return nil
	}
	ruleType, _ := rule["type"].(string)
	ruleType = strings.TrimSpace(ruleType)
	options := gin.H{}
	switch ruleType {
	case "fields_correct":
		fields := buildFieldOptions(rule["fields"])
		if len(fields) > 0 {
			options["fields"] = fields
			options["score_per_field"] = toInt(rule["score_per_field"], 1)
		}
	case "set_match":
		items := toStringSlice(rule["correct_set"])
		if len(items) > 0 {
			options["items"] = items
			options["score_per_item"] = toInt(rule["score_per_item"], 1)
		}
	case "sequence_match":
		sequence := toIntSlice(rule["correct_sequence"])
		if len(sequence) > 0 {
			options["correct_sequence"] = sequence
			options["score_per_step"] = toInt(rule["score_per_step"], 1)
			options["allow_partial"] = toBool(rule["allow_partial"])
		}
	case "multi_step":
		steps := toStringSlice(rule["steps"])
		if len(steps) > 0 {
			options["steps"] = steps
			options["score_per_step"] = toInt(rule["score_per_step"], 1)
		}
	case "exact_text":
		expected, _ := rule["expected"].(string)
		if strings.TrimSpace(expected) != "" {
			options["expected"] = strings.TrimSpace(expected)
			options["score_value"] = toInt(rule["score_value"], 1)
		}
	}

	for key, value := range buildCapabilityOptions(q, rule) {
		if value == nil {
			continue
		}
		options[key] = value
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

type mmseAnswerEvaluateRequest struct {
	QuestionKey     string          `json:"question_key" binding:"required"`
	QuestionPrompt  string          `json:"question_prompt" binding:"required"`
	QuestionType    string          `json:"question_type" binding:"required"`
	MaxScore        int             `json:"max_score"`
	Transcript      string          `json:"transcript" binding:"required"`
	SessionMode     string          `json:"session_mode,omitempty"`
	Capability      json.RawMessage `json:"capability,omitempty"`
	LocationContext json.RawMessage `json:"location_context,omitempty"`
	ExpectedValues  json.RawMessage `json:"expected_values,omitempty"`
}

type mmseAnswerEvaluateResponse struct {
	Score         int             `json:"score"`
	Confidence    float64         `json:"confidence"`
	Status        string          `json:"status"`
	Reason        string          `json:"reason"`
	Model         string          `json:"model,omitempty"`
	AnswerPayload json.RawMessage `json:"answer_payload,omitempty"`
}

func EvaluateMMSEAnswer(ctx *gin.Context) {
	if _, err := getCurrentUser(ctx); err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var req mmseAnswerEvaluateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	req.QuestionKey = strings.TrimSpace(req.QuestionKey)
	req.QuestionPrompt = strings.TrimSpace(req.QuestionPrompt)
	req.QuestionType = strings.TrimSpace(req.QuestionType)
	req.Transcript = strings.TrimSpace(req.Transcript)
	req.SessionMode = strings.TrimSpace(req.SessionMode)
	if req.Transcript == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "transcript is required")
		return
	}

	messages, err := buildMMSEAnswerEvaluationMessages(req)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	content, model, err := openAIChatFn(messages, 0.1)
	if err != nil {
		utils.RespondOK(ctx, failedMMSEAnswerEvaluateResponse(
			req.Transcript,
			"failed",
			"AI 判分暂时不可用，已回退到自动失败收口。",
			"",
		))
		return
	}

	parsed, err := parseMMSEAnswerEvaluateResponse(content)
	if err != nil {
		utils.RespondOK(ctx, failedMMSEAnswerEvaluateResponse(
			req.Transcript,
			"failed",
			"AI 判分结果无法解析，已回退到自动失败收口。",
			model,
		))
		return
	}

	response := normalizeMMSEAnswerEvaluateResponse(req, parsed)
	response.Model = model
	utils.RespondOK(ctx, gin.H{
		"score":          response.Score,
		"confidence":     response.Confidence,
		"status":         response.Status,
		"reason":         response.Reason,
		"model":          response.Model,
		"answer_payload": rawJSONToAny(response.AnswerPayload),
	})
}

func buildFieldOptions(raw any) []gin.H {
	if raw == nil {
		return nil
	}
	out := make([]gin.H, 0)
	switch v := raw.(type) {
	case []any:
		for _, it := range v {
			switch t := it.(type) {
			case map[string]any:
				key, _ := t["key"].(string)
				label, _ := t["label"].(string)
				key = strings.TrimSpace(key)
				label = strings.TrimSpace(label)
				if key == "" {
					continue
				}
				if label == "" {
					label = fieldLabel(key)
				}
				out = append(out, gin.H{"key": key, "label": label})
			case string:
				key := strings.TrimSpace(t)
				if key == "" {
					continue
				}
				out = append(out, gin.H{"key": key, "label": fieldLabel(key)})
			}
		}
	case []string:
		for _, s := range v {
			key := strings.TrimSpace(s)
			if key == "" {
				continue
			}
			out = append(out, gin.H{"key": key, "label": fieldLabel(key)})
		}
	}
	return out
}

func fieldLabel(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "year":
		return "\u5e74\u4efd"
	case "month":
		return "\u6708\u4efd"
	case "day":
		return "\u65e5\u671f"
	case "weekday":
		return "\u661f\u671f"
	case "season":
		return "\u5b63\u8282"
	case "province":
		return "\u7701\u5e02"
	case "city":
		return "\u57ce\u5e02"
	case "district":
		return "\u533a\u53bf"
	case "street":
		return "\u8857\u9053"
	case "location", "place":
		return "\u5177\u4f53\u5730\u70b9"
	case "floor":
		return "\u697c\u5c42"
	default:
		return key
	}
}

// SubmitMMSEAssessment 提交MMSE作答并评分
func SubmitMMSEAssessment(ctx *gin.Context) {
	var req MMSESubmitRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var version models.ScaleVersion
	versionQuery := global.Db.Preload("Modules").Preload("Modules.Questions")
	if req.ScaleVersionID > 0 {
		versionQuery = versionQuery.Where("id = ?", req.ScaleVersionID)
	} else {
		versionQuery = versionQuery.Where("is_active = ?", true)
	}
	if err := versionQuery.First(&version).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "scale version not found")
		return
	}

	questionMap := map[uint]models.ScaleQuestion{}
	var questionIDs []uint
	for _, m := range version.Modules {
		for _, q := range m.Questions {
			questionMap[q.ID] = q
			questionIDs = append(questionIDs, q.ID)
		}
	}
	if len(questionMap) == 0 {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_SCALE", "no questions configured")
		return
	}

	seen := map[uint]bool{}
	for _, answer := range req.Answers {
		if _, ok := questionMap[answer.QuestionID]; !ok {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_QUESTION", "question not found")
			return
		}
		if seen[answer.QuestionID] {
			utils.RespondError(ctx, http.StatusBadRequest, "DUPLICATE_QUESTION", "duplicate question answer")
			return
		}
		if err := validateArtifactRefs(answer.ArtifactRefs); err != nil {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_ARTIFACT", err.Error())
			return
		}
		seen[answer.QuestionID] = true
	}
	if len(seen) != len(questionIDs) {
		utils.RespondError(ctx, http.StatusBadRequest, "INCOMPLETE_ANSWER", "all questions must be answered")
		return
	}

	answers := make([]models.ScaleAnswer, 0, len(req.Answers))
	pendingArtifacts := make([][]models.ScaleAnswerArtifact, 0, len(req.Answers))
	moduleScores := map[uint]int{}
	totalScore := 0

	for _, input := range req.Answers {
		question := questionMap[input.QuestionID]
		score, err := scoreByRule(question, input)
		if err != nil {
			utils.RespondError(ctx, http.StatusBadRequest, "SCORE_ERROR", err.Error())
			return
		}
		if score < 0 {
			score = 0
		}
		if score > question.MaxScore {
			score = question.MaxScore
		}

		totalScore += score
		moduleScores[question.ModuleID] += score

		answers = append(answers, models.ScaleAnswer{
			UserID:         user.ID,
			QuestionID:     question.ID,
			UserAnswer:     legacyUserAnswerText(input),
			AnswerPayload:  compactJSONText(answerPayloadForScoring(input)),
			DeviceMetrics:  compactJSONText(input.DeviceMetrics),
			ManualOverride: input.ManualOverride || input.ManualScore != nil,
			Score:          score,
		})
		pendingArtifacts = append(
			pendingArtifacts,
			buildScaleAnswerArtifacts(user.ID, question.ID, input),
		)
	}

	level := classifyMMSE(totalScore)
	now := time.Now()
	var assessmentID uint

	err = global.Db.Transaction(func(tx *gorm.DB) error {
		assessment := models.ScaleAssessment{
			UserID:         user.ID,
			ScaleVersionID: version.ID,
			TotalScore:     totalScore,
			Level:          level,
			CompletedAt:    &now,
		}
		if err := tx.Create(&assessment).Error; err != nil {
			return err
		}
		assessmentID = assessment.ID

		for i := range answers {
			answers[i].AssessmentID = assessment.ID
		}
		if err := tx.Create(&answers).Error; err != nil {
			return err
		}

		for i := range pendingArtifacts {
			for j := range pendingArtifacts[i] {
				pendingArtifacts[i][j].AssessmentID = assessment.ID
				pendingArtifacts[i][j].AnswerID = answers[i].ID
			}
			if len(pendingArtifacts[i]) == 0 {
				continue
			}
			if err := tx.Create(&pendingArtifacts[i]).Error; err != nil {
				return err
			}
		}

		moduleScoreRows := make([]models.AssessmentModuleScore, 0, len(moduleScores))
		for moduleID, score := range moduleScores {
			moduleScoreRows = append(moduleScoreRows, models.AssessmentModuleScore{
				AssessmentID: assessment.ID,
				ModuleID:     moduleID,
				Score:        score,
			})
		}
		if len(moduleScoreRows) > 0 {
			if err := tx.Create(&moduleScoreRows).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not save assessment")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"assessment_id": assessmentID,
		"total_score":   totalScore,
		"level":         level,
	})
}

// GetMMSEAssessment 获取评估详情
func GetMMSEAssessment(ctx *gin.Context) {
	id := ctx.Param("id")
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var assessment models.ScaleAssessment
	if err := global.Db.Where("id = ? AND user_id = ?", id, user.ID).First(&assessment).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "assessment not found")
		return
	}

	var answers []models.ScaleAnswer
	if err := global.Db.Where("assessment_id = ?", assessment.ID).Find(&answers).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not load answers")
		return
	}

	var moduleScores []models.AssessmentModuleScore
	if err := global.Db.Where("assessment_id = ?", assessment.ID).Find(&moduleScores).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not load module scores")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"assessment": gin.H{
			"id":               assessment.ID,
			"scale_version_id": assessment.ScaleVersionID,
			"total_score":      assessment.TotalScore,
			"level":            assessment.Level,
			"completed_at":     assessment.CompletedAt,
		},
		"answers":       answers,
		"module_scores": moduleScores,
	})
}

// ListMMSEAssessments 获取当前用户评估列表（分页）
func ListMMSEAssessments(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	page := parsePositiveInt(ctx.Query("page"), 1)
	size := parsePositiveInt(ctx.Query("size"), 20)
	if size > 100 {
		size = 100
	}
	offset := (page - 1) * size

	var total int64
	if err := global.Db.Model(&models.ScaleAssessment{}).
		Where("user_id = ?", user.ID).
		Count(&total).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not count assessments")
		return
	}

	var items []models.ScaleAssessment
	if err := global.Db.Where("user_id = ?", user.ID).
		Order("id desc").
		Limit(size).
		Offset(offset).
		Find(&items).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not load assessments")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"data":  items,
		"page":  page,
		"size":  size,
		"total": total,
	})
}

func parsePositiveInt(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return def
	}
	return val
}

// EvaluateMMSEWithAI 调用 OpenAI API 生成 AI 评分与评价
func EvaluateMMSEWithAI(ctx *gin.Context) {
	id := ctx.Param("id")
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	_ = ctx.ShouldBindJSON(&req)

	var assessment models.ScaleAssessment
	if err := global.Db.Where("id = ? AND user_id = ?", id, user.ID).First(&assessment).Error; err != nil {
		utils.RespondError(ctx, http.StatusNotFound, "NOT_FOUND", "assessment not found")
		return
	}

	if assessment.AIAt != nil && !req.Force {
		utils.RespondOK(ctx, gin.H{
			"assessment_id": assessment.ID,
			"ai_score":      assessment.AIScore,
			"ai_level":      assessment.AILevel,
			"ai_comment":    assessment.AIComment,
			"ai_result":     assessment.AIResult,
			"ai_model":      assessment.AIModel,
			"ai_at":         assessment.AIAt,
		})
		return
	}

	var answers []models.ScaleAnswer
	if err := global.Db.Where("assessment_id = ?", assessment.ID).Find(&answers).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not load answers")
		return
	}

	questionIDs := make([]uint, 0, len(answers))
	for _, a := range answers {
		questionIDs = append(questionIDs, a.QuestionID)
	}

	var questions []models.ScaleQuestion
	if err := global.Db.Where("id IN ?", questionIDs).Find(&questions).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not load questions")
		return
	}
	qMap := map[uint]models.ScaleQuestion{}
	for _, q := range questions {
		qMap[q.ID] = q
	}

	aiPayload := buildAIPayload(assessment, answers, qMap)
	payloadJSON, _ := json.Marshal(aiPayload)

	messages := []utils.OpenAIMessage{
		{
			Role: "system",
			Content: `你是临床评估助手。请以温暖、鼓励、有人情味的语气，直接对受测者使用“您/您的”来给出评价，同时保持专业客观，必须基于提供的 MMSE 数据。只返回严格 JSON，不要输出多余文本或说明。
必须使用中文；语气积极但不过度夸张，强调理解、肯定和可执行的建议；主语固定用“您”。
输出字段与类型：ai_score(整数0-30)、ai_level(字符串 normal|mild|moderate|severe)、comment(字符串<=200字)、suggestion(字符串<=200字)。
严格 JSON 示例：{"ai_score":26,"ai_level":"mild","comment":"...","suggestion":"..."}`,
		},
		{
			Role:    "user",
			Content: string(payloadJSON),
		},
	}
	content, model, err := utils.OpenAIChat(messages, 0.2)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", err.Error())
		return
	}

	aiRes, err := parseAIResponse(content)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", "invalid ai response")
		return
	}

	now := time.Now()
	assessment.AIScore = aiRes.AIScore
	assessment.AILevel = strings.TrimSpace(aiRes.AILevel)
	assessment.AIComment = strings.TrimSpace(strings.TrimSpace(aiRes.Comment) + " " + strings.TrimSpace(aiRes.Suggestion))
	assessment.AIResult = strings.TrimSpace(content)
	assessment.AIModel = model
	assessment.AIAt = &now

	if err := global.Db.Save(&assessment).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "could not save ai result")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"assessment_id": assessment.ID,
		"ai_score":      assessment.AIScore,
		"ai_level":      assessment.AILevel,
		"ai_comment":    assessment.AIComment,
		"ai_result":     assessment.AIResult,
		"ai_model":      assessment.AIModel,
		"ai_at":         assessment.AIAt,
	})
}

type aiAnswerItem struct {
	QuestionID uint   `json:"question_id"`
	Content    string `json:"content"`
	MaxScore   int    `json:"max_score"`
	Score      int    `json:"score"`
	UserAnswer string `json:"user_answer"`
}

type aiPayload struct {
	AssessmentID uint           `json:"assessment_id"`
	TotalScore   int            `json:"total_score"`
	Level        string         `json:"level"`
	Answers      []aiAnswerItem `json:"answers"`
}

func buildAIPayload(assessment models.ScaleAssessment, answers []models.ScaleAnswer, qMap map[uint]models.ScaleQuestion) aiPayload {
	items := make([]aiAnswerItem, 0, len(answers))
	for _, a := range answers {
		q := qMap[a.QuestionID]
		items = append(items, aiAnswerItem{
			QuestionID: a.QuestionID,
			Content:    q.Content,
			MaxScore:   q.MaxScore,
			Score:      a.Score,
			UserAnswer: a.UserAnswer,
		})
	}
	return aiPayload{
		AssessmentID: assessment.ID,
		TotalScore:   assessment.TotalScore,
		Level:        assessment.Level,
		Answers:      items,
	}
}

type aiResponse struct {
	AIScore    int    `json:"ai_score"`
	AILevel    string `json:"ai_level"`
	Comment    string `json:"comment"`
	Suggestion string `json:"suggestion"`
}

// parseAIResponse tolerates surrounding text/code fences and extracts the JSON block.
func parseAIResponse(content string) (aiResponse, error) {
	trimmed := strings.TrimSpace(content)
	var res aiResponse

	if err := json.Unmarshal([]byte(trimmed), &res); err == nil {
		return res, nil
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		snippet := trimmed[start : end+1]
		if err := json.Unmarshal([]byte(snippet), &res); err == nil {
			return res, nil
		}
	}

	return aiResponse{}, errors.New("invalid ai response")
}

func buildMMSEAnswerEvaluationMessages(
	req mmseAnswerEvaluateRequest,
) ([]utils.OpenAIMessage, error) {
	const systemPrompt = `你是 CCRT/MMSE 单题判分器。你的唯一任务是根据题目、识别文本和参考信息判断当前题目是否达标。
只返回严格 JSON，不要输出任何额外说明、代码块或自然语言。
输出字段固定为：
- score: 整数，范围 0..max_score
- confidence: 0..1 的小数
- status: "matched" | "unmatched" | "low_confidence" | "failed"
- reason: 简短中文说明，不超过 60 字
- answer_payload: JSON 对象，至少包含 text 字段
规则：
1. 如果无法稳定判断，返回 status="low_confidence"，score=0。
2. 如果回答明显不符合题意，返回 status="unmatched"，score=0。
3. 只有证据充分时才返回 status="matched"。
4. 不要编造上下文，也不要依赖 GPS 或未提供的信息。`

	userPayload := map[string]any{
		"question_key":     req.QuestionKey,
		"question_prompt":  req.QuestionPrompt,
		"question_type":    req.QuestionType,
		"max_score":        req.MaxScore,
		"transcript":       req.Transcript,
		"session_mode":     req.SessionMode,
		"question_profile": mmseAnswerEvaluationInstruction(req.QuestionKey),
	}
	if capability := rawJSONToAny(req.Capability); capability != nil {
		userPayload["capability"] = capability
	}
	if locationContext := rawJSONToAny(req.LocationContext); locationContext != nil {
		userPayload["location_context"] = locationContext
	}
	if expectedValues := rawJSONToAny(req.ExpectedValues); expectedValues != nil {
		userPayload["expected_values"] = expectedValues
	}

	body, err := json.Marshal(userPayload)
	if err != nil {
		return nil, err
	}
	return []utils.OpenAIMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(body)},
	}, nil
}

func mmseAnswerEvaluationInstruction(questionKey string) string {
	switch strings.TrimSpace(questionKey) {
	case "orientation_year", "orientation_season", "orientation_month", "orientation_day", "orientation_weekday":
		return "时间定向题。只判断识别文本是否与当前日期时间答案一致。"
	case "location_city", "location_district", "location_street", "location_place", "location_floor":
		return "地点定向题。优先依据 location_context 和 expected_values 判断是否匹配，不允许自己猜测位置。"
	case "memory_immediate", "memory_delayed":
		return "词语记忆题。请识别命中的词项数量，并在 answer_payload.items 中返回命中的词项列表。"
	case "language_watch", "language_pen":
		return "命名题。只判断是否命中了目标词或常见同义表达，不要发散。"
	case "language_sentence":
		return "完整句子题。只判断文本是否像一句完整、通顺、符合题意的句子。"
	default:
		return "单题判分。严格根据 transcript 与 expected_values 判断是否达标。"
	}
}

func parseMMSEAnswerEvaluateResponse(content string) (mmseAnswerEvaluateResponse, error) {
	trimmed := strings.TrimSpace(content)
	var res mmseAnswerEvaluateResponse

	if err := json.Unmarshal([]byte(trimmed), &res); err == nil {
		return res, nil
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		snippet := trimmed[start : end+1]
		if err := json.Unmarshal([]byte(snippet), &res); err == nil {
			return res, nil
		}
	}

	return mmseAnswerEvaluateResponse{}, errors.New("invalid ai response")
}

func normalizeMMSEAnswerEvaluateResponse(
	req mmseAnswerEvaluateRequest,
	raw mmseAnswerEvaluateResponse,
) mmseAnswerEvaluateResponse {
	status := strings.ToLower(strings.TrimSpace(raw.Status))
	if status == "" {
		switch {
		case raw.Confidence > 0 && raw.Confidence < 0.75:
			status = "low_confidence"
		case raw.Score > 0:
			status = "matched"
		default:
			status = "unmatched"
		}
	}
	switch status {
	case "matched", "unmatched", "low_confidence", "failed":
	default:
		status = "failed"
	}

	if raw.Confidence < 0 {
		raw.Confidence = 0
	}
	if raw.Confidence > 1 {
		raw.Confidence = 1
	}
	if raw.Score < 0 {
		raw.Score = 0
	}
	if req.MaxScore > 0 && raw.Score > req.MaxScore {
		raw.Score = req.MaxScore
	}
	if status != "matched" {
		raw.Score = 0
	}
	if len(raw.AnswerPayload) == 0 {
		raw.AnswerPayload = mustCompactJSON(map[string]any{"text": req.Transcript})
	}
	if strings.TrimSpace(raw.Reason) == "" {
		raw.Reason = "已完成自动判分。"
	}

	return mmseAnswerEvaluateResponse{
		Score:         raw.Score,
		Confidence:    raw.Confidence,
		Status:        status,
		Reason:        strings.TrimSpace(raw.Reason),
		AnswerPayload: raw.AnswerPayload,
	}
}

func failedMMSEAnswerEvaluateResponse(
	transcript string,
	status string,
	reason string,
	model string,
) gin.H {
	return gin.H{
		"score":      0,
		"confidence": 0,
		"status":     status,
		"reason":     reason,
		"model":      model,
		"answer_payload": map[string]any{
			"text":              transcript,
			"evaluation_status": status,
		},
	}
}

func rawJSONToAny(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}

	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	return payload
}

func mustCompactJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(payload)
}

func classifyMMSE(total int) string {
	switch {
	case total >= 27:
		return "normal"
	case total >= 21:
		return "mild"
	case total >= 10:
		return "moderate"
	default:
		return "severe"
	}
}

func scoreByRule(q models.ScaleQuestion, a MMSEAnswerInput) (int, error) {
	if strings.TrimSpace(q.AnswerRule) == "" {
		return scoreManual(q, a)
	}

	var rule map[string]any
	if err := json.Unmarshal([]byte(q.AnswerRule), &rule); err != nil {
		return 0, errors.New("invalid answer rule")
	}
	ruleType, _ := rule["type"].(string)
	ruleType = strings.TrimSpace(ruleType)
	answerPayload := answerPayloadForScoring(a)

	switch ruleType {
	case "fields_correct":
		fields := toStringSlice(rule["fields"])
		per := toInt(rule["score_per_field"], 1)
		correctCount := countFieldsCorrect(fields, answerPayload)
		return correctCount * per, nil
	case "set_match":
		correct := toStringSlice(rule["correct_set"])
		per := toInt(rule["score_per_item"], 1)
		items := extractStringSlice(answerPayload, "items")
		return setMatchScore(correct, items) * per, nil
	case "sequence_match":
		correct := toIntSlice(rule["correct_sequence"])
		per := toInt(rule["score_per_step"], 1)
		seq := extractIntSlice(answerPayload, "sequence")
		return sequenceMatchScore(correct, seq) * per, nil
	case "multi_step":
		steps := toStringSlice(rule["steps"])
		per := toInt(rule["score_per_step"], 1)
		done := extractStringSlice(answerPayload, "steps_done")
		return setMatchScore(steps, done) * per, nil
	case "exact_text":
		expected, _ := rule["expected"].(string)
		scoreValue := toInt(rule["score_value"], 1)
		text := extractText(answerPayload)
		if normalizeText(text) == normalizeText(expected) {
			return scoreValue, nil
		}
		return 0, nil
	case "manual":
		return scoreManual(q, a)
	default:
		return scoreManual(q, a)
	}
}

func scoreManual(q models.ScaleQuestion, a MMSEAnswerInput) (int, error) {
	if a.ManualScore != nil {
		return *a.ManualScore, nil
	}
	answerPayload := answerPayloadForScoring(a)
	var obj map[string]any
	if err := json.Unmarshal(answerPayload, &obj); err == nil {
		if v, ok := obj["score"]; ok {
			return toInt(v, 0), nil
		}
	}
	return 0, errors.New("manual score required")
}

func countFieldsCorrect(fields []string, raw json.RawMessage) int {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return 0
	}
	if v, ok := obj["correct_fields"]; ok {
		list := toStringSlice(v)
		return setMatchScore(fields, list)
	}
	if v, ok := obj["fields"]; ok {
		switch v.(type) {
		case []any, []string:
			list := toStringSlice(v)
			return setMatchScore(fields, list)
		case map[string]any:
			return countFieldsByValue(fields, v.(map[string]any))
		}
	}
	if v, ok := obj["correct_map"]; ok {
		if m, ok := v.(map[string]any); ok {
			count := 0
			for _, f := range fields {
				if bv, ok := m[f]; ok && toBool(bv) {
					count++
				}
			}
			return count
		}
	}
	if v, ok := obj["fields_map"]; ok {
		if m, ok := v.(map[string]any); ok {
			return countFieldsByValue(fields, m)
		}
	}
	return 0
}

func countFieldsByValue(fields []string, m map[string]any) int {
	if len(fields) == 0 || len(m) == 0 {
		return 0
	}
	// If these look like datetime fields, validate against current time.
	if isTimeFields(fields) {
		return countDatetimeFields(fields, m)
	}
	// Otherwise, accept non-empty value as correct (no reference data).
	count := 0
	for _, f := range fields {
		if v, ok := m[f]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				count++
			}
		}
	}
	return count
}

func isTimeFields(fields []string) bool {
	set := map[string]bool{}
	for _, f := range fields {
		set[strings.ToLower(strings.TrimSpace(f))] = true
	}
	return set["year"] || set["month"] || set["day"] || set["weekday"] || set["season"]
}

func countDatetimeFields(fields []string, m map[string]any) int {
	now := time.Now()
	count := 0
	for _, f := range fields {
		key := strings.ToLower(strings.TrimSpace(f))
		val, ok := m[f]
		if !ok {
			val, ok = m[key]
		}
		if !ok {
			continue
		}
		text, _ := val.(string)
		if text == "" {
			continue
		}
		if matchTimeField(key, text, now) {
			count++
		}
	}
	return count
}

func matchTimeField(key, input string, now time.Time) bool {
	input = strings.TrimSpace(input)
	switch key {
	case "year":
		return normalizeInt(input) == now.Year()
	case "month":
		return normalizeInt(input) == int(now.Month())
	case "day":
		return normalizeInt(input) == now.Day()
	case "weekday":
		return normalizeWeekday(input) == weekdayNumber(now.Weekday())
	case "season":
		return normalizeSeason(input) == seasonByMonth(int(now.Month()))
	default:
		return false
	}
}

func normalizeInt(raw string) int {
	raw = strings.TrimSpace(raw)
	val, err := strconv.Atoi(raw)
	if err == nil {
		return val
	}
	return 0
}

func weekdayNumber(w time.Weekday) int {
	if w == time.Sunday {
		return 7
	}
	return int(w)
}

func normalizeWeekday(raw string) int {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\u661f\u671f", "")
	raw = strings.ReplaceAll(raw, "\u5468", "")
	raw = strings.ReplaceAll(raw, "\u793c\u62dc", "")
	switch raw {
	case "\u4e00":
		return 1
	case "\u4e8c":
		return 2
	case "\u4e09":
		return 3
	case "\u56db":
		return 4
	case "\u4e94":
		return 5
	case "\u516d":
		return 6
	case "\u65e5", "\u5929":
		return 7
	}
	if n, err := strconv.Atoi(raw); err == nil {
		if n == 0 {
			return 7
		}
		return n
	}
	return 0
}

func seasonByMonth(month int) int {
	switch month {
	case 3, 4, 5:
		return 1
	case 6, 7, 8:
		return 2
	case 9, 10, 11:
		return 3
	default:
		return 4
	}
}

func normalizeSeason(raw string) int {
	raw = strings.TrimSpace(raw)
	switch raw {
	case "\u6625", "\u6625\u5b63", "\u6625\u5929":
		return 1
	case "\u590f", "\u590f\u5b63", "\u590f\u5929":
		return 2
	case "\u79cb", "\u79cb\u5b63", "\u79cb\u5929":
		return 3
	case "\u51ac", "\u51ac\u5b63", "\u51ac\u5929":
		return 4
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return 0
}

func extractStringSlice(raw json.RawMessage, key string) []string {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v, ok := obj[key]; ok {
			return toStringSlice(v)
		}
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	return nil
}

func extractIntSlice(raw json.RawMessage, key string) []int {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v, ok := obj[key]; ok {
			return toIntSlice(v)
		}
	}
	var arr []int
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	return nil
}

func extractText(raw json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v, ok := obj["text"].(string); ok {
			return v
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

func normalizeText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, it := range t {
			switch v := it.(type) {
			case string:
				out = append(out, strings.TrimSpace(v))
			case map[string]any:
				if key, ok := v["key"].(string); ok {
					key = strings.TrimSpace(key)
					if key != "" {
						out = append(out, key)
					}
				}
			}
		}
		return out
	default:
		return nil
	}
}

func toIntSlice(v any) []int {
	switch t := v.(type) {
	case []int:
		return t
	case []any:
		out := make([]int, 0, len(t))
		for _, it := range t {
			out = append(out, toInt(it, 0))
		}
		return out
	default:
		return nil
	}
}

func toInt(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return int(n)
		}
	}
	return def
}

func toBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return false
}

func setMatchScore(correct, items []string) int {
	if len(correct) == 0 || len(items) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, c := range correct {
		key := strings.ToLower(strings.TrimSpace(c))
		if key != "" {
			set[key] = true
		}
	}
	score := 0
	used := map[string]bool{}
	for _, it := range items {
		key := strings.ToLower(strings.TrimSpace(it))
		if key == "" || used[key] {
			continue
		}
		if set[key] {
			score++
			used[key] = true
		}
	}
	return score
}

func sequenceMatchScore(correct, seq []int) int {
	n := len(correct)
	if len(seq) < n {
		n = len(seq)
	}
	score := 0
	for i := 0; i < n; i++ {
		if seq[i] == correct[i] {
			score++
		}
	}
	return score
}
