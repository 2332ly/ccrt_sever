//go:build aliyun_sms

package utils

import (
	"errors"
	"fmt"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dypnsapi20170525 "github.com/alibabacloud-go/dypnsapi-20170525/v3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	credential "github.com/aliyun/credentials-go/credentials"
)

var (
	smsSchemeName   string
	smsTemplateCode string
	smsSignName     string
	smsCodeLength   int64 = 6
	smsValidTimeSec int64 = 300
	smsIntervalSec  int64 = 60
)

// SetSMSConfig 设置短信相关配置（启动时调用）
func SetSMSConfig(schemeName, templateCode, signName string, codeLength, validTimeSec, intervalSec int64) {
	smsSchemeName = strings.TrimSpace(schemeName)
	smsTemplateCode = strings.TrimSpace(templateCode)
	smsSignName = strings.TrimSpace(signName)
	if codeLength > 0 {
		smsCodeLength = codeLength
	}
	if validTimeSec > 0 {
		smsValidTimeSec = validTimeSec
	}
	if intervalSec > 0 {
		smsIntervalSec = intervalSec
	}
}

func createDypnsClient() (*dypnsapi20170525.Client, error) {
	// 优先使用配置注入的 AK/SK（方便多人联调）。未配置则回退到 credentials-go 自动发现。
	ak, sk, token := getSMSCredentials()

	var (
		cred credential.Credential
		err  error
	)
	if ak != "" && sk != "" {
		cfg := &credential.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(ak),
			AccessKeySecret: tea.String(sk),
		}
		if strings.TrimSpace(token) != "" {
			cfg.SecurityToken = tea.String(token)
		}
		cred, err = credential.NewCredential(cfg)
	} else {
		cred, err = credential.NewCredential(nil)
	}
	if err != nil {
		return nil, err
	}

	apiCfg := &openapi.Config{Credential: cred}
	apiCfg.Endpoint = tea.String("dypnsapi.aliyuncs.com")
	return dypnsapi20170525.NewClient(apiCfg)
}

// SendSMSVerifyCode 发送短信验证码
func SendSMSVerifyCode(phone string, returnVerifyCode bool) (verifyID string, returnedCode string, err error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "", "", errors.New("phone is empty")
	}
	if smsTemplateCode == "" || smsSignName == "" || smsSchemeName == "" {
		return "", "", errors.New("sms config is not set")
	}

	client, err := createDypnsClient()
	if err != nil {
		return "", "", err
	}

	req := &dypnsapi20170525.SendSmsVerifyCodeRequest{
		SchemeName:       tea.String(smsSchemeName),
		CountryCode:      tea.String("86"),
		PhoneNumber:      tea.String(phone),
		TemplateCode:     tea.String(smsTemplateCode),
		TemplateParam:    tea.String("{\"code\":\"##code##\",\"min\":\"5\"}"),
		Interval:         tea.Int64(smsIntervalSec),
		ValidTime:        tea.Int64(smsValidTimeSec),
		CodeLength:       tea.Int64(smsCodeLength),
		DuplicatePolicy:  tea.Int64(1),
		CodeType:         tea.Int64(1),
		ReturnVerifyCode: tea.Bool(returnVerifyCode),
		SignName:         tea.String(smsSignName),
	}

	runtime := &util.RuntimeOptions{}
	resp, err := client.SendSmsVerifyCodeWithOptions(req, runtime)
	if err != nil {
		return "", "", err
	}
	if resp == nil || resp.Body == nil {
		return "", "", errors.New("empty response")
	}
	if resp.Body.Code == nil || tea.StringValue(resp.Body.Code) != "OK" {
		msg := ""
		if resp.Body.Message != nil {
			msg = tea.StringValue(resp.Body.Message)
		}
		return "", "", fmt.Errorf("send sms failed: %s", msg)
	}

	model := resp.Body.Model
	if model != nil {
		// SDK v3.0.0: VerifyCode 位于 Model.VerifyCode
		if model.VerifyCode != nil {
			returnedCode = tea.StringValue(model.VerifyCode)
		}
		// SDK v3.0.0: 没有 VerifyId 字段，使用 BizId 作为返回的 verify_id
		if model.BizId != nil {
			verifyID = tea.StringValue(model.BizId)
		}
	}

	// 若调用方不希望返回验证码，清空
	if !returnVerifyCode {
		returnedCode = ""
	}
	if verifyID == "" {
		verifyID = "unknown"
	}
	return verifyID, returnedCode, nil
}

// CheckSMSVerifyCode 校验短信验证码
func CheckSMSVerifyCode(phone, code string) error {
	phone = strings.TrimSpace(phone)
	code = strings.TrimSpace(code)
	if phone == "" || code == "" {
		return errors.New("phone or code is empty")
	}
	if smsSchemeName == "" {
		return errors.New("sms scheme name is not configured")
	}

	client, err := createDypnsClient()
	if err != nil {
		return err
	}

	req := &dypnsapi20170525.CheckSmsVerifyCodeRequest{
		SchemeName:  tea.String(smsSchemeName),
		CountryCode: tea.String("86"),
		PhoneNumber: tea.String(phone),
		VerifyCode:  tea.String(code),
	}
	runtime := &util.RuntimeOptions{}
	resp, err := client.CheckSmsVerifyCodeWithOptions(req, runtime)
	if err != nil {
		return fmt.Errorf("check sms api error: %v", err)
	}
	if resp == nil || resp.Body == nil {
		return errors.New("empty response")
	}
	if resp.Body.Code == nil || tea.StringValue(resp.Body.Code) != "OK" {
		msg := ""
		if resp.Body.Message != nil {
			msg = tea.StringValue(resp.Body.Message)
		}
		return fmt.Errorf("verify failed: %s", msg)
	}
	return nil
}
