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
		auth.POST("/refresh", controllers.RefreshToken)
		auth.POST("/sms/send", controllers.SendSMS)
		auth.POST("/sms/verify", controllers.VerifySMS)
	}

	api := r.Group("/api")
	api.Use(utils.AuthMiddleware())
	{
		api.GET("/me", controllers.Me)
		api.PUT("/me/emergency", controllers.UpdateEmergencyContact)
		api.POST("/sos", controllers.CreateSOSEvent)

		// Medication Reminder Routes
		med := api.Group("/medications")
		{
			med.POST("", controllers.CreateMedication)
			med.GET("", controllers.GetMedications)
			med.GET("/schedule", controllers.GetMedicationSchedule)
			med.POST("/checkins", controllers.CreateMedicationCheckin)
			med.GET("/stats", controllers.GetMedicationStats)
			med.GET("/:id", controllers.GetMedication)
			med.PUT("/:id", controllers.UpdateMedication)
			med.DELETE("/:id", controllers.DeleteMedication)
		}

		// MMSE Routes
		mmse := api.Group("/mmse")
		{
			mmse.GET("/scale", controllers.GetMMSEScale)
			mmse.GET("/assessments", controllers.ListMMSEAssessments)
			mmse.POST("/assessments", controllers.SubmitMMSEAssessment)
			mmse.GET("/assessments/:id", controllers.GetMMSEAssessment)
			mmse.POST("/assessments/:id/ai", controllers.EvaluateMMSEWithAI)
		}

		// Rehab game results
		games := api.Group("/games")
		{
			games.POST("/results", controllers.CreateGameResult)
			games.GET("/difficulty", controllers.GetGameDifficulty)
		}

		ai := api.Group("/ai")
		{
			ai.POST("/idiom", controllers.GenerateIdiom)
			ai.POST("/life-assistant", controllers.GenerateLifeAssistantTip)
		}
	}

	return r
}
