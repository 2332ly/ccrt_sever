package utils

import (
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(pwd string) (string, error) {
	hush, err := bcrypt.GenerateFromPassword([]byte(pwd), 12)
	return string(hush), err
}

// CheckPassword 验证明文密码是否与哈希匹配
// 返回 true 表示密码正确，false 表示不匹配或发生错误
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
