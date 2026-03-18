package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAliyunNLSTokenReturnsCachedToken(t *testing.T) {
	resetNLSTestState()

	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		fmt.Fprintf(w, `{"Token":{"Id":"token-%d","ExpireTime":%d}}`, n, time.Now().Add(10*time.Minute).Unix())
	}))
	defer server.Close()

	SetAliyunNLSConfig("ak", "sk", "app", server.URL, "https://example.com/asr", 30)

	first, err := AliyunNLSToken()
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	second, err := AliyunNLSToken()
	if err != nil {
		t.Fatalf("second token: %v", err)
	}

	if first != "token-1" || second != "token-1" {
		t.Fatalf("expected cached token reuse, got first=%q second=%q", first, second)
	}
	if count.Load() != 1 {
		t.Fatalf("expected single token request, got %d", count.Load())
	}
}

func TestAliyunNLSTokenRefreshesExpiredToken(t *testing.T) {
	resetNLSTestState()

	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		fmt.Fprintf(w, `{"Token":{"Id":"token-%d","ExpireTime":%d}}`, n, time.Now().Add(10*time.Minute).Unix())
	}))
	defer server.Close()

	SetAliyunNLSConfig("ak", "sk", "app", server.URL, "https://example.com/asr", 30)

	first, err := AliyunNLSToken()
	if err != nil {
		t.Fatalf("first token: %v", err)
	}

	cachedNLSToken.mu.Lock()
	cachedNLSToken.expireAt = time.Now().Add(30 * time.Second)
	cachedNLSToken.mu.Unlock()

	second, err := AliyunNLSToken()
	if err != nil {
		t.Fatalf("second token: %v", err)
	}

	if first != "token-1" || second != "token-2" {
		t.Fatalf("expected refreshed token, got first=%q second=%q", first, second)
	}
	if count.Load() != 2 {
		t.Fatalf("expected two token requests, got %d", count.Load())
	}
}

func TestAliyunNLSTokenRequiresCredentials(t *testing.T) {
	resetNLSTestState()
	SetAliyunNLSConfig("", "", "app", "https://example.com/token", "https://example.com/asr", 30)

	if _, err := AliyunNLSToken(); err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestAliyunNLSTokenAcceptsNumericErrCodeZero(t *testing.T) {
	resetNLSTestState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"Token":{"Id":"token-ok","ExpireTime":%d},"ErrCode":0}`, time.Now().Add(10*time.Minute).Unix())
	}))
	defer server.Close()

	SetAliyunNLSConfig("ak", "sk", "app", server.URL, "https://example.com/asr", 30)

	token, err := AliyunNLSToken()
	if err != nil {
		t.Fatalf("expected numeric ErrCode to be accepted, got %v", err)
	}
	if token != "token-ok" {
		t.Fatalf("unexpected token: %q", token)
	}
}

func resetNLSTestState() {
	cachedNLSToken = nlsTokenCache{}
	aliyunNLSAccessKeyID = ""
	aliyunNLSAccessKeySecret = ""
	aliyunNLSAppKey = ""
	aliyunNLSTokenURL = ""
	aliyunNLSAsrURL = ""
	aliyunNLSTimeout = 30 * time.Second
}
