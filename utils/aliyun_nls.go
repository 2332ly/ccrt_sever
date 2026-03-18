package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	aliyunNLSAccessKeyID     string
	aliyunNLSAccessKeySecret string
	aliyunNLSAppKey          string
	aliyunNLSTokenURL        string
	aliyunNLSAsrURL          string
	aliyunNLSTimeout         time.Duration = 30 * time.Second
)

type nlsTokenCache struct {
	token    string
	expireAt time.Time
	mu       sync.Mutex
}

var cachedNLSToken nlsTokenCache

type NLSASROptions struct {
	EnablePunctuationPrediction    bool
	EnableInverseTextNormalization bool
	EnableVoiceDetection           bool
}

// IsAliyunNLSConfigured reports whether ASR credentials are available.
func IsAliyunNLSConfigured() bool {
	return strings.TrimSpace(aliyunNLSAccessKeyID) != "" &&
		strings.TrimSpace(aliyunNLSAccessKeySecret) != "" &&
		strings.TrimSpace(aliyunNLSAppKey) != ""
}

// SetAliyunNLSConfig sets Aliyun NLS configuration. Call during startup.
func SetAliyunNLSConfig(accessKeyID, accessKeySecret, appKey, tokenURL, asrURL string, timeoutSec int) {
	aliyunNLSAccessKeyID = strings.TrimSpace(accessKeyID)
	aliyunNLSAccessKeySecret = strings.TrimSpace(accessKeySecret)
	aliyunNLSAppKey = strings.TrimSpace(appKey)
	aliyunNLSTokenURL = strings.TrimSpace(tokenURL)
	aliyunNLSAsrURL = strings.TrimSpace(asrURL)
	if timeoutSec > 0 {
		aliyunNLSTimeout = time.Duration(timeoutSec) * time.Second
	}
	if aliyunNLSTokenURL == "" {
		aliyunNLSTokenURL = "https://nls-meta.cn-shanghai.aliyuncs.com/"
	}
	if aliyunNLSAsrURL == "" {
		aliyunNLSAsrURL = "https://nls-gateway-cn-shanghai.aliyuncs.com/stream/v1/asr"
	}
}

// AliyunNLSToken returns cached token or creates a new one.
func AliyunNLSToken() (string, error) {
	if aliyunNLSAccessKeyID == "" || aliyunNLSAccessKeySecret == "" {
		return "", errors.New("aliyun nls access key is empty")
	}

	cachedNLSToken.mu.Lock()
	if cachedNLSToken.token != "" && time.Until(cachedNLSToken.expireAt) > 90*time.Second {
		token := cachedNLSToken.token
		cachedNLSToken.mu.Unlock()
		return token, nil
	}
	cachedNLSToken.mu.Unlock()

	token, expireAt, err := createNLSToken()
	if err != nil {
		return "", err
	}

	cachedNLSToken.mu.Lock()
	cachedNLSToken.token = token
	cachedNLSToken.expireAt = expireAt
	cachedNLSToken.mu.Unlock()
	return token, nil
}

func createNLSToken() (string, time.Time, error) {
	nonce, err := randomNonce()
	if err != nil {
		return "", time.Time{}, err
	}
	params := map[string]string{
		"AccessKeyId":      aliyunNLSAccessKeyID,
		"Action":           "CreateToken",
		"Format":           "JSON",
		"RegionId":         "cn-shanghai",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   nonce,
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2019-02-28",
	}
	signature := signAliyunQuery(params, aliyunNLSAccessKeySecret)
	params["Signature"] = signature

	reqURL := aliyunNLSTokenURL
	if !strings.Contains(reqURL, "?") {
		reqURL += "?" + encodeQuery(params)
	} else {
		reqURL += "&" + encodeQuery(params)
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	client := &http.Client{Timeout: aliyunNLSTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("nls token request failed: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		Token struct {
			Id         string `json:"Id"`
			ExpireTime int64  `json:"ExpireTime"`
		} `json:"Token"`
		ErrCode   any    `json:"ErrCode"`
		ErrMsg    string `json:"ErrMsg"`
		RequestId string `json:"RequestId"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", time.Time{}, err
	}
	errCode := normalizeAliyunErrCode(res.ErrCode)
	if errCode != "" && errCode != "0" {
		return "", time.Time{}, fmt.Errorf("nls token error: %s %s", errCode, res.ErrMsg)
	}
	token := strings.TrimSpace(res.Token.Id)
	if token == "" {
		return "", time.Time{}, errors.New("nls token is empty")
	}
	expireAt := time.Now().Add(10 * time.Minute)
	if res.Token.ExpireTime > 0 {
		expireAt = time.Unix(res.Token.ExpireTime, 0)
	}
	return token, expireAt, nil
}

func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func signAliyunQuery(params map[string]string, accessKeySecret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(percentEncode(k))
		buf.WriteByte('=')
		buf.WriteString(percentEncode(params[k]))
	}
	canonical := buf.String()
	stringToSign := "GET&%2F&" + percentEncode(canonical)
	h := hmac.New(sha1.New, []byte(accessKeySecret+"&"))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func percentEncode(val string) string {
	escaped := url.QueryEscape(val)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func encodeQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(url.QueryEscape(k))
		buf.WriteByte('=')
		buf.WriteString(url.QueryEscape(params[k]))
	}
	return buf.String()
}

func normalizeAliyunErrCode(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case json.Number:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// AliyunNLSASR sends audio bytes to Aliyun NLS RESTful ASR and returns text.
func AliyunNLSASR(data []byte, format string, sampleRate int, opts NLSASROptions) (string, map[string]any, error) {
	if len(data) == 0 {
		return "", nil, errors.New("audio is empty")
	}
	if aliyunNLSAppKey == "" {
		return "", nil, errors.New("aliyun nls appkey is empty")
	}

	token, err := AliyunNLSToken()
	if err != nil {
		return "", nil, err
	}

	if format == "" {
		format = "wav"
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	u, err := url.Parse(aliyunNLSAsrURL)
	if err != nil {
		return "", nil, err
	}
	q := u.Query()
	q.Set("appkey", aliyunNLSAppKey)
	q.Set("format", format)
	q.Set("sample_rate", strconv.Itoa(sampleRate))
	q.Set("enable_punctuation_prediction", strconv.FormatBool(opts.EnablePunctuationPrediction))
	q.Set("enable_inverse_text_normalization", strconv.FormatBool(opts.EnableInverseTextNormalization))
	q.Set("enable_voice_detection", strconv.FormatBool(opts.EnableVoiceDetection))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(data))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-NLS-Token", token)

	client := &http.Client{Timeout: aliyunNLSTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("nls asr failed: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return "", nil, err
	}

	statusOK := false
	if v, ok := payload["status"]; ok {
		switch t := v.(type) {
		case float64:
			if int(t) == 0 || int(t) == 20000000 {
				statusOK = true
			}
		case int:
			if t == 0 || t == 20000000 {
				statusOK = true
			}
		case string:
			if t == "0" || t == "20000000" {
				statusOK = true
			}
		}
	}
	if !statusOK {
		msg := ""
		if m, ok := payload["message"].(string); ok {
			msg = m
		}
		if msg == "" {
			msg = string(bodyBytes)
		}
		return "", payload, fmt.Errorf("nls asr error: %s", msg)
	}

	text := ""
	if v, ok := payload["result"].(string); ok {
		text = strings.TrimSpace(v)
	}
	if text == "" {
		if v, ok := payload["text"].(string); ok {
			text = strings.TrimSpace(v)
		}
	}
	if text == "" {
		return "", payload, errors.New("nls asr result is empty")
	}
	return text, payload, nil
}
