package config

import (
	"log"
	"os"
	"strings"

	"ccrt_sever/utils"

	"github.com/spf13/viper"
)

type config struct {
	App struct {
		Name string
		Port string
	}
	Database struct {
		Dsn          string
		MaxIdleConns int
		MaxOpenConns int
	}
	JWT struct {
		Secret string
	}
	SMS struct {
		SchemeName   string
		TemplateCode string
		SignName     string
		CodeLength   int64
		ValidTimeSec int64
		IntervalSec  int64
		// ReturnVerifyCode 仅测试用，生产建议 false
		ReturnVerifyCode bool

		// 为了多人联调允许放在 yaml 里（不建议提交到仓库）
		AccessKeyId     string
		AccessKeySecret string
		SecurityToken   string
	}
	OpenAI struct {
		BaseURL string
		APIKey  string
		Model   string
		Timeout int
	}
	Notify struct {
		Enabled         bool
		PollIntervalSec int
		LeadMinutes     int
		GraceMinutes    int
		Channels        struct {
			SMS   bool
			Voice bool
			Call  bool
		}
		Webhook struct {
			SMS   string
			Voice string
			Call  string
		}
	}
}

var AppConfig *config

func InitConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config/")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Error reading config file: ", err)
	}
	AppConfig = &config{}
	err := viper.Unmarshal(&AppConfig)
	if err != nil {
		log.Fatal("Error unmarshaling config file: ", err)
	}

	// JWT secret：优先配置文件，其次环境变量（兼容不改 config.yml 也能跑）
	secret := strings.TrimSpace(AppConfig.JWT.Secret)
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if secret == "" {
		log.Fatal("JWT secret is empty. Please set jwt.secret in config.yml or env JWT_SECRET")
	}
	utils.SetJWTSecret(secret)

	// SMS 配置：用于发送/校验验证码
	utils.SetSMSConfig(
		AppConfig.SMS.SchemeName,
		AppConfig.SMS.TemplateCode,
		AppConfig.SMS.SignName,
		AppConfig.SMS.CodeLength,
		AppConfig.SMS.ValidTimeSec,
		AppConfig.SMS.IntervalSec,
	)
	// 可选：从配置注入 AK/SK（仅建议开发使用）
	utils.SetSMSCredentialsFromConfig(AppConfig.SMS.AccessKeyId, AppConfig.SMS.AccessKeySecret, AppConfig.SMS.SecurityToken)

	// OpenAI 配置
	utils.SetOpenAIConfig(AppConfig.OpenAI.BaseURL, AppConfig.OpenAI.APIKey, AppConfig.OpenAI.Model, AppConfig.OpenAI.Timeout)

	initDB()
}
