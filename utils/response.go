package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIError 统一错误响应结构
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RespondError 返回统一格式的错误
func RespondError(ctx *gin.Context, status int, code, message string) {
	ctx.AbortWithStatusJSON(status, gin.H{
		"error": APIError{
			Code:    code,
			Message: message,
		},
	})
}

// RespondOK 返回统一格式的成功响应（data 可为空）
func RespondOK(ctx *gin.Context, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	ctx.JSON(http.StatusOK, data)
}
