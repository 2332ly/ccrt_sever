package utils

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

// SetJWTSecret 设置 JWT 签名密钥（建议在启动时调用）
func SetJWTSecret(secret string) {
	jwtSecret = []byte(secret)
}

func GenerateJWT(username string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("jwt secret is not configured")
	}
	// 72h 有效期
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	})

	signedToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}
	// 保持兼容：仍然返回 "Bearer <token>"
	return "Bearer " + signedToken, nil
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
