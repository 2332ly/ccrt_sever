package utils

import "strings"

var (
	smsAccessKeyID     string
	smsAccessKeySecret string
	smsSecurityToken   string
)

// SetSMSCredentialsFromConfig 允许从配置注入 AK/SK，方便多人联调。
// 强烈建议仅在开发环境使用，并确保 config.yml 不提交到仓库。
func SetSMSCredentialsFromConfig(accessKeyID, accessKeySecret, securityToken string) {
	smsAccessKeyID = strings.TrimSpace(accessKeyID)
	smsAccessKeySecret = strings.TrimSpace(accessKeySecret)
	smsSecurityToken = strings.TrimSpace(securityToken)
}

func getSMSCredentials() (ak, sk, token string) {
	return smsAccessKeyID, smsAccessKeySecret, smsSecurityToken
}
