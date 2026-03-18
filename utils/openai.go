package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	openaiBaseURL string
	openaiAPIKey  string
	openaiModel   string
	openaiTimeout time.Duration = 30 * time.Second
)

// SetOpenAIConfig sets OpenAI API configuration. Call during startup.
func SetOpenAIConfig(baseURL, apiKey, model string, timeoutSec int) {
	openaiBaseURL = strings.TrimSpace(baseURL)
	openaiAPIKey = strings.TrimSpace(apiKey)
	openaiModel = strings.TrimSpace(model)
	if timeoutSec > 0 {
		openaiTimeout = time.Duration(timeoutSec) * time.Second
	}
	if openaiBaseURL == "" {
		openaiBaseURL = "https://api.openai.com/v1"
	}
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIFunctionSpec `json:"function"`
}

type OpenAIFunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIChatResult struct {
	Content   string
	Model     string
	ToolCalls []OpenAIToolCall
}

type openaiChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  string          `json:"tool_choice,omitempty"`
}

type openaiChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message openAIResponseMessage `json:"message"`
	} `json:"choices"`
}

type openAIResponseMessage struct {
	Content   json.RawMessage  `json:"content"`
	ToolCalls []OpenAIToolCall `json:"tool_calls"`
}

// OpenAIChat sends a chat completion request and returns the assistant content.
func OpenAIChat(messages []OpenAIMessage, temperature float64) (string, string, error) {
	res, err := OpenAIChatWithTools(messages, nil, temperature)
	if err != nil {
		return "", "", err
	}
	return res.Content, res.Model, nil
}

// OpenAIChatWithTools sends a chat completion request with optional tools.
func OpenAIChatWithTools(messages []OpenAIMessage, tools []OpenAITool, temperature float64) (OpenAIChatResult, error) {
	if openaiAPIKey == "" {
		return OpenAIChatResult{}, errors.New("openai api key is empty")
	}
	if openaiModel == "" {
		return OpenAIChatResult{}, errors.New("openai model is empty")
	}
	if openaiBaseURL == "" {
		openaiBaseURL = "https://api.openai.com/v1"
	}

	reqBody := openaiChatRequest{
		Model:       openaiModel,
		Messages:    messages,
		Temperature: temperature,
		Tools:       tools,
	}
	if len(tools) > 0 {
		reqBody.ToolChoice = "auto"
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return OpenAIChatResult{}, err
	}

	url := strings.TrimRight(openaiBaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return OpenAIChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)

	client := &http.Client{Timeout: openaiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return OpenAIChatResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return OpenAIChatResult{}, fmt.Errorf("openai api request failed: status=%d", resp.StatusCode)
		}
		return OpenAIChatResult{}, fmt.Errorf("openai api request failed: status=%d body=%s", resp.StatusCode, bodyText)
	}

	var res openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return OpenAIChatResult{}, err
	}
	if len(res.Choices) == 0 {
		return OpenAIChatResult{}, errors.New("openai empty response")
	}

	model := strings.TrimSpace(res.Model)
	if model == "" {
		model = openaiModel
	}
	content := decodeOpenAIContent(res.Choices[0].Message.Content)
	return OpenAIChatResult{
		Content:   content,
		Model:     model,
		ToolCalls: res.Choices[0].Message.ToolCalls,
	}, nil
}

func decodeOpenAIContent(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var builder strings.Builder
		for _, part := range parts {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(strings.TrimSpace(part.Text))
		}
		return strings.TrimSpace(builder.String())
	}

	return trimmed
}
