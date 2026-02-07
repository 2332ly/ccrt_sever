package controllers

import (
	"net/http"

	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

// Me 示例受保护接口：返回当前 token 里的 username
func Me(ctx *gin.Context) {
	username, _ := ctx.Get("username")
	utils.RespondOK(ctx, gin.H{
		"username": username,
		"status":   http.StatusOK,
	})
}
