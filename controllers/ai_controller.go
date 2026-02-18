package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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

type lifeAssistantAIResponse struct {
	Topic   string   `json:"topic"`
	Tip     string   `json:"tip"`
	Actions []string `json:"actions"`
}

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
			Content: "你是一名中文助手，只输出一个四字成语，不要任何标点、解释或多余文字。",
		},
		{
			Role:    "user",
			Content: "请给我一个四字成语。",
		},
	}

	content, model, err := utils.OpenAIChat(messages, temp)
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
				"Return ONLY a valid JSON object with keys: topic, tip, actions. " +
				"topic: 2-6 Chinese characters. tip: <=40 Chinese characters. " +
				"actions: array of 2-3 short items (<=8 chars). No extra text.",
		},
		{
			Role:    "user",
			Content: "User request: " + query,
		},
	}

	content, model, err := utils.OpenAIChat(messages, temp)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", err.Error())
		return
	}

	res, err := parseLifeAssistantResponse(content)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadGateway, "AI_ERROR", "invalid ai response")
		return
	}

	res.Topic = strings.TrimSpace(res.Topic)
	res.Tip = strings.TrimSpace(res.Tip)
	res.Actions = normalizeActions(res.Actions)

	utils.RespondOK(ctx, gin.H{
		"topic":   res.Topic,
		"tip":     res.Tip,
		"actions": res.Actions,
		"model":   model,
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
