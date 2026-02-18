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
	switch ruleType {
	case "fields_correct":
		fields := buildFieldOptions(rule["fields"])
		if len(fields) == 0 {
			return nil
		}
		return gin.H{
			"fields":          fields,
			"score_per_field": toInt(rule["score_per_field"], 1),
		}
	case "set_match":
		items := toStringSlice(rule["correct_set"])
		if len(items) == 0 {
			return nil
		}
		return gin.H{
			"items":          items,
			"score_per_item": toInt(rule["score_per_item"], 1),
		}
	default:
		return nil
	}
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

	// Validate answers completeness and duplicates
	seen := map[uint]bool{}
	for _, a := range req.Answers {
		if _, ok := questionMap[a.QuestionID]; !ok {
			utils.RespondError(ctx, http.StatusBadRequest, "INVALID_QUESTION", "question not found")
			return
		}
		if seen[a.QuestionID] {
			utils.RespondError(ctx, http.StatusBadRequest, "DUPLICATE_QUESTION", "duplicate question answer")
			return
		}
		seen[a.QuestionID] = true
	}
	if len(seen) != len(questionIDs) {
		utils.RespondError(ctx, http.StatusBadRequest, "INCOMPLETE_ANSWER", "all questions must be answered")
		return
	}

	answers := make([]models.ScaleAnswer, 0, len(req.Answers))
	moduleScores := map[uint]int{}
	totalScore := 0

	for _, a := range req.Answers {
		q := questionMap[a.QuestionID]
		score, err := scoreByRule(q, a)
		if err != nil {
			utils.RespondError(ctx, http.StatusBadRequest, "SCORE_ERROR", err.Error())
			return
		}
		if score < 0 {
			score = 0
		}
		if score > q.MaxScore {
			score = q.MaxScore
		}

		totalScore += score
		moduleScores[q.ModuleID] += score

		answers = append(answers, models.ScaleAnswer{
			UserID:     user.ID,
			QuestionID: q.ID,
			UserAnswer: strings.TrimSpace(string(a.UserAnswer)),
			Score:      score,
		})
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

		moduleScoreRows := make([]models.AssessmentModuleScore, 0, len(moduleScores))
		for moduleID, s := range moduleScores {
			moduleScoreRows = append(moduleScoreRows, models.AssessmentModuleScore{
				AssessmentID: assessment.ID,
				ModuleID:     moduleID,
				Score:        s,
			})
		}
		if err := tx.Create(&moduleScoreRows).Error; err != nil {
			return err
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

	switch ruleType {
	case "fields_correct":
		fields := toStringSlice(rule["fields"])
		per := toInt(rule["score_per_field"], 1)
		correctCount := countFieldsCorrect(fields, a.UserAnswer)
		return correctCount * per, nil
	case "set_match":
		correct := toStringSlice(rule["correct_set"])
		per := toInt(rule["score_per_item"], 1)
		items := extractStringSlice(a.UserAnswer, "items")
		return setMatchScore(correct, items) * per, nil
	case "sequence_match":
		correct := toIntSlice(rule["correct_sequence"])
		per := toInt(rule["score_per_step"], 1)
		seq := extractIntSlice(a.UserAnswer, "sequence")
		return sequenceMatchScore(correct, seq) * per, nil
	case "multi_step":
		steps := toStringSlice(rule["steps"])
		per := toInt(rule["score_per_step"], 1)
		done := extractStringSlice(a.UserAnswer, "steps_done")
		return setMatchScore(steps, done) * per, nil
	case "exact_text":
		expected, _ := rule["expected"].(string)
		scoreValue := toInt(rule["score_value"], 1)
		text := extractText(a.UserAnswer)
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
	var obj map[string]any
	if err := json.Unmarshal(a.UserAnswer, &obj); err == nil {
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
