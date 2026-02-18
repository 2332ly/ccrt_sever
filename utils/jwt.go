package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

const (
	accessTokenTTL  = 72 * time.Hour
	refreshTokenTTL = 30 * 24 * time.Hour
)

// SetJWTSecret 设置 JWT 签名密钥（建议在启动时调用）
func SetJWTSecret(secret string) {
	jwtSecret = []byte(secret)
}

// GenerateJWT 兼容旧逻辑：返回 "Bearer <token>"
func GenerateJWT(username string) (string, error) {
	token, _, err := GenerateAccessToken(username)
	if err != nil {
		return "", err
	}
	return FormatBearerToken(token), nil
}

// GenerateAccessToken issues a short-lived JWT access token.
func GenerateAccessToken(username string) (string, time.Time, error) {
	if len(jwtSecret) == 0 {
		return "", time.Time{}, errors.New("jwt secret is not configured")
	}
	expiresAt := time.Now().Add(accessTokenTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": username,
		"exp":      expiresAt.Unix(),
	})

	signedToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signedToken, expiresAt, nil
}

// GenerateRefreshToken issues a long-lived opaque refresh token.
func GenerateRefreshToken() (string, time.Time, error) {
	expiresAt := time.Now().Add(refreshTokenTTL)
	raw, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	return raw, expiresAt, nil
}

// FormatBearerToken returns "Bearer <token>".
func FormatBearerToken(token string) string {
	return "Bearer " + strings.TrimSpace(token)
}

// HashToken hashes a token for storage.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ParseBearerToken 从 Authorization 头/字符串中解析 bearer token
func ParseBearerToken(auth string) string {
	auth = strings.TrimSpace(auth)
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(parts[1])
	}
	// 兼容：如果直接传 token，也接受
	return auth
}

// ValidateJWT 校验 token 并返回 username
func ValidateJWT(tokenString string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("jwt secret is not configured")
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}

	u, _ := claims["username"].(string)
	if strings.TrimSpace(u) == "" {
		return "", errors.New("username claim missing")
	}
	return u, nil
}
