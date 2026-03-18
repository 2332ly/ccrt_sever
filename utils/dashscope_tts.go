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
	dashscopeBaseURL string
	dashscopeAPIKey  string
	dashscopeModel   string
	dashscopeVoice   string
	dashscopeTimeout time.Duration = 30 * time.Second
)

// IsDashScopeConfigured reports whether TTS credentials are available.
func IsDashScopeConfigured() bool {
	return strings.TrimSpace(dashscopeAPIKey) != ""
}

// SetDashScopeConfig sets DashScope TTS configuration. Call during startup.
func SetDashScopeConfig(baseURL, apiKey, model, voice string, timeoutSec int) {
	dashscopeBaseURL = strings.TrimSpace(baseURL)
	dashscopeAPIKey = strings.TrimSpace(apiKey)
	dashscopeModel = strings.TrimSpace(model)
	dashscopeVoice = strings.TrimSpace(voice)
	if timeoutSec > 0 {
		dashscopeTimeout = time.Duration(timeoutSec) * time.Second
	}
	if dashscopeBaseURL == "" {
		dashscopeBaseURL = "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation"
	}
	if dashscopeModel == "" {
		dashscopeModel = "qwen3-tts-flash"
	}
	if dashscopeVoice == "" {
		dashscopeVoice = "longxiaochun"
	}
}

// DashScopeDefaultVoice returns the configured fallback system voice.
func DashScopeDefaultVoice() string {
	if strings.TrimSpace(dashscopeVoice) == "" {
		return "longxiaochun"
	}
	return strings.TrimSpace(dashscopeVoice)
}

// DashScopeDefaultModel returns the configured fallback system TTS model.
func DashScopeDefaultModel() string {
	if strings.TrimSpace(dashscopeModel) == "" {
		return "qwen3-tts-flash"
	}
	return strings.TrimSpace(dashscopeModel)
}

type dashscopeTTSRequest struct {
	Model string `json:"model"`
	Input struct {
		Text string `json:"text"`
	} `json:"input"`
	Parameters struct {
		Voice string `json:"voice"`
	} `json:"parameters"`
}

type dashscopeTTSResponse struct {
	Output struct {
		Audio struct {
			URL string `json:"url"`
		} `json:"audio"`
	} `json:"output"`
}

// DashScopeTTS requests TTS audio URL via DashScope.
func DashScopeTTS(text, voice string) (string, string, string, error) {
	return DashScopeTTSWithModel(text, voice, "")
}

// DashScopeTTSWithModel requests TTS audio URL via DashScope using an optional model override.
func DashScopeTTSWithModel(text, voice, modelOverride string) (string, string, string, error) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return "", "", "", errors.New("text is empty")
	}
	if dashscopeAPIKey == "" {
		return "", "", "", errors.New("dashscope api key is empty")
	}
	if dashscopeBaseURL == "" {
		dashscopeBaseURL = "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation"
	}

	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = DashScopeDefaultModel()
	}
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voice = DashScopeDefaultVoice()
	}

	reqBody := dashscopeTTSRequest{Model: model}
	reqBody.Input.Text = cleaned
	reqBody.Parameters.Voice = voice

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", "", err
	}

	req, err := http.NewRequest(http.MethodPost, dashscopeBaseURL, bytes.NewReader(payload))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+dashscopeAPIKey)

	client := &http.Client{Timeout: dashscopeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return "", "", "", fmt.Errorf("dashscope request failed: status=%d", resp.StatusCode)
		}
		return "", "", "", fmt.Errorf("dashscope request failed: status=%d body=%s", resp.StatusCode, bodyText)
	}

	var res dashscopeTTSResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", "", err
	}
	url := strings.TrimSpace(res.Output.Audio.URL)
	if url == "" {
		return "", "", "", errors.New("dashscope audio url is empty")
	}
	return url, model, voice, nil
}

// DashScopeTTSBytes fetches the generated audio bytes via DashScope.
func DashScopeTTSBytes(text, voice string) ([]byte, string, string, string, error) {
	return DashScopeTTSBytesWithModel(text, voice, "")
}

// DashScopeTTSBytesWithModel fetches generated audio bytes via DashScope with an optional model override.
func DashScopeTTSBytesWithModel(text, voice, modelOverride string) ([]byte, string, string, string, error) {
	url, model, voice, err := DashScopeTTSWithModel(text, voice, modelOverride)
	if err != nil {
		return nil, "", "", "", err
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", "", "", err
	}

	client := &http.Client{Timeout: dashscopeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return nil, "", "", "", fmt.Errorf("dashscope audio fetch failed: status=%d", resp.StatusCode)
		}
		return nil, "", "", "", fmt.Errorf("dashscope audio fetch failed: status=%d body=%s", resp.StatusCode, bodyText)
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return nil, "", "", "", err
	}
	if len(data) == 0 {
		return nil, "", "", "", errors.New("dashscope audio is empty")
	}
	return data, contentType, model, voice, nil
}
