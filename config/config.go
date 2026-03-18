package config

import (
	"log"
	"os"
	"strconv"
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
	DashScope struct {
		BaseURL             string
		APIKey              string
		Model               string
		Voice               string
		Timeout             int
		VoiceCloneCreateURL string
		VoiceCloneQueryURL  string
		VoiceCloneDeleteURL string
		VoiceCloneModel     string
		VoiceCloneTimeout   int
	}
	AliyunNLS struct {
		AccessKeyId     string
		AccessKeySecret string
		AppKey          string
		TokenURL        string
		AsrURL          string
		Timeout         int
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

	// DashScope TTS 配置
	dashBaseURL := strings.TrimSpace(AppConfig.DashScope.BaseURL)
	dashAPIKey := strings.TrimSpace(AppConfig.DashScope.APIKey)
	dashModel := strings.TrimSpace(AppConfig.DashScope.Model)
	dashVoice := strings.TrimSpace(AppConfig.DashScope.Voice)
	if dashBaseURL == "" {
		dashBaseURL = strings.TrimSpace(os.Getenv("DASHSCOPE_BASE_URL"))
	}
	if dashAPIKey == "" {
		dashAPIKey = strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	}
	if dashModel == "" {
		dashModel = strings.TrimSpace(os.Getenv("DASHSCOPE_MODEL"))
	}
	if dashVoice == "" {
		dashVoice = strings.TrimSpace(os.Getenv("DASHSCOPE_VOICE"))
	}
	utils.SetDashScopeConfig(dashBaseURL, dashAPIKey, dashModel, dashVoice, AppConfig.DashScope.Timeout)
	cloneCreateURL := strings.TrimSpace(AppConfig.DashScope.VoiceCloneCreateURL)
	cloneQueryURL := strings.TrimSpace(AppConfig.DashScope.VoiceCloneQueryURL)
	cloneDeleteURL := strings.TrimSpace(AppConfig.DashScope.VoiceCloneDeleteURL)
	cloneModel := strings.TrimSpace(AppConfig.DashScope.VoiceCloneModel)
	cloneTimeout := AppConfig.DashScope.VoiceCloneTimeout
	if cloneCreateURL == "" {
		cloneCreateURL = strings.TrimSpace(os.Getenv("DASHSCOPE_VOICE_CLONE_CREATE_URL"))
	}
	if cloneQueryURL == "" {
		cloneQueryURL = strings.TrimSpace(os.Getenv("DASHSCOPE_VOICE_CLONE_QUERY_URL"))
	}
	if cloneDeleteURL == "" {
		cloneDeleteURL = strings.TrimSpace(os.Getenv("DASHSCOPE_VOICE_CLONE_DELETE_URL"))
	}
	if cloneModel == "" {
		cloneModel = strings.TrimSpace(os.Getenv("DASHSCOPE_VOICE_CLONE_MODEL"))
	}
	if cloneTimeout <= 0 {
		if timeoutRaw := strings.TrimSpace(os.Getenv("DASHSCOPE_VOICE_CLONE_TIMEOUT")); timeoutRaw != "" {
			if parsed, err := strconv.Atoi(timeoutRaw); err == nil {
				cloneTimeout = parsed
			}
		}
	}
	utils.SetDashScopeVoiceCloneConfig(cloneCreateURL, cloneQueryURL, cloneDeleteURL, cloneModel, cloneTimeout)

	// Aliyun NLS ASR 配置
	nlsAK := strings.TrimSpace(AppConfig.AliyunNLS.AccessKeyId)
	nlsSK := strings.TrimSpace(AppConfig.AliyunNLS.AccessKeySecret)
	nlsAppKey := strings.TrimSpace(AppConfig.AliyunNLS.AppKey)
	nlsTokenURL := strings.TrimSpace(AppConfig.AliyunNLS.TokenURL)
	nlsAsrURL := strings.TrimSpace(AppConfig.AliyunNLS.AsrURL)
	if nlsAK == "" {
		nlsAK = strings.TrimSpace(os.Getenv("ALIYUN_NLS_ACCESS_KEY_ID"))
	}
	if nlsSK == "" {
		nlsSK = strings.TrimSpace(os.Getenv("ALIYUN_NLS_ACCESS_KEY_SECRET"))
	}
	if nlsAppKey == "" {
		nlsAppKey = strings.TrimSpace(os.Getenv("ALIYUN_NLS_APP_KEY"))
	}
	if nlsTokenURL == "" {
		nlsTokenURL = strings.TrimSpace(os.Getenv("ALIYUN_NLS_TOKEN_URL"))
	}
	if nlsAsrURL == "" {
		nlsAsrURL = strings.TrimSpace(os.Getenv("ALIYUN_NLS_ASR_URL"))
	}
	utils.SetAliyunNLSConfig(nlsAK, nlsSK, nlsAppKey, nlsTokenURL, nlsAsrURL, AppConfig.AliyunNLS.Timeout)

	initDB()
}
