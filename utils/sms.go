//go:build !aliyun_sms

package utils

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

type smsMemItem struct {
	code      string
	expiresAt time.Time
}

var (
	smsStoreMu sync.Mutex
	smsStore   = map[string]smsMemItem{}
	memTTL     = 5 * time.Minute
)

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}

// SetSMSConfig 在内存实现里仅用于设置 TTL；与真实阿里云实现保持同名接口，避免改动其它代码
func SetSMSConfig(_schemeName, _templateCode, _signName string, _codeLength, validTimeSec, _intervalSec int64) {
	if validTimeSec > 0 {
		memTTL = time.Duration(validTimeSec) * time.Second
	}
}

// SendSMSVerifyCode 内存版：生成验证码并保存（returnVerifyCode=true 时返回 code，便于本地联调）
func SendSMSVerifyCode(phone string, returnVerifyCode bool) (verifyID string, returnedCode string, err error) {
	if phone == "" {
		return "", "", errors.New("phone is empty")
	}
	code := randCode(6)
	smsStoreMu.Lock()
	smsStore[phone] = smsMemItem{code: code, expiresAt: time.Now().Add(memTTL)}
	smsStoreMu.Unlock()
	if returnVerifyCode {
		returnedCode = code
	}
	return "mem", returnedCode, nil
}

// CheckSMSVerifyCode 内存版：校验验证码（一次性使用）
func CheckSMSVerifyCode(phone, code string) error {
	if phone == "" || code == "" {
		return errors.New("phone or code is empty")
	}
	smsStoreMu.Lock()
	item, ok := smsStore[phone]
	if ok {
		delete(smsStore, phone)
	}
	smsStoreMu.Unlock()

	if !ok {
		return errors.New("code not found")
	}
	if time.Now().After(item.expiresAt) {
		return errors.New("code expired")
	}
	if code != item.code {
		return errors.New("code mismatch")
	}
	return nil
}

func randCode(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}
