package router

import (
	"ccrt_sever/controllers"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	auth := r.Group("/api/auth")
	{
		auth.POST("/login", controllers.Login)
		auth.POST("/register", controllers.Register)
		auth.POST("/sms/send", controllers.SendSMS)
		auth.POST("/sms/verify", controllers.VerifySMS)
	}

	api := r.Group("/api")
	api.Use(utils.AuthMiddleware())
	{
		api.GET("/me", controllers.Me)
	}

	return r
}
