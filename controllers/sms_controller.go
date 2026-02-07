package controllers

import (
	"net/http"

	"ccrt_sever/config"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

// SendSMS 发送短信验证码
func SendSMS(ctx *gin.Context) {
	var req SendSMSRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// 便于本地联调定位 JSON/body 绑定问题
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// 仅测试：是否返回验证码由配置控制
	returnCode := false
	if config.AppConfig != nil {
		returnCode = config.AppConfig.SMS.ReturnVerifyCode
	}

	verifyID, returnedCode, err := utils.SendSMSVerifyCode(req.Phone, returnCode)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "SMS_SEND_FAILED", err.Error())
		return
	}

	resp := gin.H{"verify_id": verifyID}
	if returnCode {
		resp["verify_code"] = returnedCode
	}
	utils.RespondOK(ctx, resp)
}

// VerifySMS 校验短信验证码
func VerifySMS(ctx *gin.Context) {
	var req VerifySMSRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := utils.CheckSMSVerifyCode(req.Phone, req.SMSCode); err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "SMS_VERIFY_FAILED", "Invalid sms code")
		return
	}

	utils.RespondOK(ctx, gin.H{"verified": true})
}
