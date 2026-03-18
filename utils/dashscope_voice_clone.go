package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const dashScopeVoiceCloneEndpoint = "https://dashscope.aliyuncs.com/api/v1/services/audio/tts/customization"

var (
	dashscopeVoiceCloneCreateURL string
	dashscopeVoiceCloneQueryURL  string
	dashscopeVoiceCloneDeleteURL string
	dashscopeVoiceCloneModel     string
	dashscopeVoiceCloneTimeout   time.Duration = 60 * time.Second
)

type DashScopeVoiceCloneInfo struct {
	VoiceID   string `json:"voice_id"`
	Prefix    string `json:"prefix"`
	GMTCreate string `json:"gmt_create"`
}

// SetDashScopeVoiceCloneConfig sets DashScope voice clone configuration.
func SetDashScopeVoiceCloneConfig(createURL, queryURL, deleteURL, targetModel string, timeoutSec int) {
	dashscopeVoiceCloneCreateURL = strings.TrimSpace(createURL)
	dashscopeVoiceCloneQueryURL = strings.TrimSpace(queryURL)
	dashscopeVoiceCloneDeleteURL = strings.TrimSpace(deleteURL)
	dashscopeVoiceCloneModel = strings.TrimSpace(targetModel)
	if timeoutSec > 0 {
		dashscopeVoiceCloneTimeout = time.Duration(timeoutSec) * time.Second
	}
	if dashscopeVoiceCloneCreateURL == "" {
		dashscopeVoiceCloneCreateURL = dashScopeVoiceCloneEndpoint
	}
	if dashscopeVoiceCloneQueryURL == "" {
		dashscopeVoiceCloneQueryURL = dashScopeVoiceCloneEndpoint
	}
	if dashscopeVoiceCloneDeleteURL == "" {
		dashscopeVoiceCloneDeleteURL = dashScopeVoiceCloneEndpoint
	}
	if dashscopeVoiceCloneModel == "" {
		dashscopeVoiceCloneModel = "qwen-tts"
	}
}

// IsDashScopeVoiceCloneConfigured reports whether voice clone credentials are available.
func IsDashScopeVoiceCloneConfigured() bool {
	return strings.TrimSpace(dashscopeAPIKey) != ""
}

// DashScopeVoiceCloneModel returns the configured target synthesis model for cloned voices.
func DashScopeVoiceCloneModel() string {
	if strings.TrimSpace(dashscopeVoiceCloneModel) == "" {
		return "qwen-tts"
	}
	return strings.TrimSpace(dashscopeVoiceCloneModel)
}

// DashScopeCreateVoiceClone uploads an enrollment sample and returns the cloned voice id.
func DashScopeCreateVoiceClone(fileName, contentType string, data []byte, displayName, relationship string) (string, string, error) {
	if strings.TrimSpace(dashscopeAPIKey) == "" {
		return "", "", errors.New("dashscope api key is empty")
	}
	if len(data) == 0 {
		return "", "", errors.New("voice sample is empty")
	}

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)

	if err := writer.WriteField("action", "create"); err != nil {
		return "", "", err
	}
	if err := writer.WriteField("model", "qwen-voice-enrollment"); err != nil {
		return "", "", err
	}
	if err := writer.WriteField("target_model", DashScopeVoiceCloneModel()); err != nil {
		return "", "", err
	}
	customInfo, err := json.Marshal(map[string]string{
		"display_name": displayName,
		"relationship": relationship,
	})
	if err != nil {
		return "", "", err
	}
	if err := writer.WriteField("custom_info", string(customInfo)); err != nil {
		return "", "", err
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", "", err
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}

	req, err := http.NewRequest(http.MethodPost, dashscopeVoiceCloneCreateURL, bytes.NewReader(payload.Bytes()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+dashscopeAPIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("X-Sample-Content-Type", strings.TrimSpace(contentType))
	}

	bodyBytes, err := doDashScopeVoiceCloneRequest(req)
	if err != nil {
		return "", "", err
	}

	var res struct {
		RequestID string `json:"request_id"`
		Output    struct {
			VoiceID string `json:"voice_id"`
			Voice   string `json:"voice"`
		} `json:"output"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", "", err
	}
	voiceID := strings.TrimSpace(res.Output.VoiceID)
	if voiceID == "" {
		voiceID = strings.TrimSpace(res.Output.Voice)
	}
	if voiceID == "" {
		var generic map[string]any
		if err := json.Unmarshal(bodyBytes, &generic); err == nil {
			voiceID = extractVoiceID(generic)
		}
	}
	if voiceID == "" {
		return "", strings.TrimSpace(res.RequestID), errors.New("dashscope cloned voice id is empty")
	}
	return voiceID, strings.TrimSpace(res.RequestID), nil
}

// DashScopeListVoiceClones fetches voice clone metadata from DashScope.
func DashScopeListVoiceClones() ([]DashScopeVoiceCloneInfo, error) {
	if strings.TrimSpace(dashscopeAPIKey) == "" {
		return nil, errors.New("dashscope api key is empty")
	}
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	if err := writer.WriteField("action", "list"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("model", "qwen-voice-enrollment"); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, dashscopeVoiceCloneQueryURL, bytes.NewReader(payload.Bytes()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+dashscopeAPIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	bodyBytes, err := doDashScopeVoiceCloneRequest(req)
	if err != nil {
		return nil, err
	}

	var res struct {
		Output struct {
			VoiceList []DashScopeVoiceCloneInfo `json:"voice_list"`
		} `json:"output"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, err
	}
	return res.Output.VoiceList, nil
}

// DashScopeDeleteVoiceClone removes a voice clone from DashScope.
func DashScopeDeleteVoiceClone(voiceID string) error {
	if strings.TrimSpace(dashscopeAPIKey) == "" {
		return errors.New("dashscope api key is empty")
	}
	cleanedVoiceID := strings.TrimSpace(voiceID)
	if cleanedVoiceID == "" {
		return errors.New("voice id is empty")
	}

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	if err := writer.WriteField("action", "delete"); err != nil {
		return err
	}
	if err := writer.WriteField("model", "qwen-voice-enrollment"); err != nil {
		return err
	}
	if err := writer.WriteField("voice_id", cleanedVoiceID); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, dashscopeVoiceCloneDeleteURL, bytes.NewReader(payload.Bytes()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+dashscopeAPIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	_, err = doDashScopeVoiceCloneRequest(req)
	return err
}

func doDashScopeVoiceCloneRequest(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: dashscopeVoiceCloneTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return nil, fmt.Errorf("dashscope voice clone request failed: status=%d", resp.StatusCode)
		}
		return nil, fmt.Errorf("dashscope voice clone request failed: status=%d body=%s", resp.StatusCode, bodyText)
	}
	return bodyBytes, nil
}

func extractVoiceID(payload map[string]any) string {
	output, _ := payload["output"].(map[string]any)
	if output == nil {
		return ""
	}
	if voiceID, ok := output["voice_id"].(string); ok {
		return strings.TrimSpace(voiceID)
	}
	if voiceID, ok := output["voice"].(string); ok {
		return strings.TrimSpace(voiceID)
	}
	return ""
}
