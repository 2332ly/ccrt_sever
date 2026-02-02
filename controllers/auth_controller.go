package controllers

import (
	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

// Register 注册新用户并返回 JWT 令牌
func Register(ctx *gin.Context) {
	var user models.User
	// 从请求体中解析 JSON 到 user 结构体，若解析失败则返回 400（请求无效）
	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// 对用户提交的明文密码进行哈希处理，防止明文存储
	hashedPwd, err := utils.HashPassword(user.Password)
	if err != nil {
		// 哈希失败通常是内部错误，返回 500
		ctx.JSON(500, gin.H{"error": "Invalid request"})
		return
	}
	// 将哈希后的密码替换原始密码字段，准备保存到数据库
	user.Password = hashedPwd

	// 根据用户名生成 JWT，用于后续认证
	token, err := utils.GenerateJWT(user.Username)
	if err != nil {
		ctx.JSON(500, gin.H{"error": "Could not generate token"})
		return
	}

	// 将新用户写入数据库，若写入失败则返回 500
	if err = global.Db.Create(&user).Error; err != nil {
		ctx.JSON(500, gin.H{"error": "Could not create user"})
		return
	}

	// 注册成功，返回生成的 JWT 给客户端
	ctx.JSON(200, gin.H{"token": token})
}

// Login 登录处理函数
func Login(ctx *gin.Context) {
	var input models.User
	// 复用 models.User 结构来接收用户名和密码字段
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// 在数据库中查找用户，按 Username 字段匹配
	var user models.User
	if err := global.Db.Where("username = ?", input.Username).First(&user).Error; err != nil {
		// 未找到用户或查询出错，返回 401（未授权）以避免泄露更多信息
		ctx.JSON(401, gin.H{"error": "Invalid credentials"})
		return
	}

	// 比对密码哈希
	if !utils.CheckPassword(input.Password, user.Password) {
		ctx.JSON(401, gin.H{"error": "Invalid credentials"})
		return
	}

	// 密码正确，生成 JWT 并返回给客户端
	token, err := utils.GenerateJWT(user.Username)
	if err != nil {
		ctx.JSON(500, gin.H{"error": "Could not generate token"})
		return
	}

	ctx.JSON(200, gin.H{"token": token})
}
