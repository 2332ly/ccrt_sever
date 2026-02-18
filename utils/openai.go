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

type openaiChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
}

type openaiChatResponse struct {
	Choices []struct {
		Message OpenAIMessage `json:"message"`
	} `json:"choices"`
}

// OpenAIChat sends a chat completion request and returns the assistant content.
func OpenAIChat(messages []OpenAIMessage, temperature float64) (string, string, error) {
	if openaiAPIKey == "" {
		return "", "", errors.New("openai api key is empty")
	}
	if openaiModel == "" {
		return "", "", errors.New("openai model is empty")
	}
	if openaiBaseURL == "" {
		openaiBaseURL = "https://api.openai.com/v1"
	}

	reqBody := openaiChatRequest{
		Model:       openaiModel,
		Messages:    messages,
		Temperature: temperature,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", err
	}

	url := strings.TrimRight(openaiBaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)

	client := &http.Client{Timeout: openaiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return "", "", fmt.Errorf("openai api request failed: status=%d", resp.StatusCode)
		}
		return "", "", fmt.Errorf("openai api request failed: status=%d body=%s", resp.StatusCode, bodyText)
	}

	var res openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", err
	}
	if len(res.Choices) == 0 {
		return "", "", errors.New("openai empty response")
	}
	return res.Choices[0].Message.Content, openaiModel, nil
}
