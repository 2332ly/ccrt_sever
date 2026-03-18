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
		api.GET("/me/care-contacts", controllers.ListCareContacts)
		api.POST("/me/care-contacts", controllers.CreateCareContact)
		api.PUT("/me/care-contacts/:id", controllers.UpdateCareContact)
		api.DELETE("/me/care-contacts/:id", controllers.DeleteCareContact)
		api.POST("/me/care-contacts/:id/primary", controllers.SetPrimaryCareContact)
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

		reminders := api.Group("/reminders")
		{
			reminders.POST("", controllers.CreateReminder)
			reminders.GET("", controllers.GetReminders)
			reminders.GET("/schedule", controllers.GetReminderSchedule)
			reminders.POST("/checkins", controllers.CreateReminderCheckin)
			reminders.GET("/stats", controllers.GetReminderStats)
			reminders.GET("/:id", controllers.GetReminder)
			reminders.PUT("/:id", controllers.UpdateReminder)
			reminders.DELETE("/:id", controllers.DeleteReminder)
		}

		// MMSE Routes
		mmse := api.Group("/mmse")
		{
			mmse.GET("/scale", controllers.GetMMSEScale)
			mmse.POST("/answers/evaluate", controllers.EvaluateMMSEAnswer)
			mmse.GET("/assessments", controllers.ListMMSEAssessments)
			mmse.POST("/assessments", controllers.SubmitMMSEAssessment)
			mmse.GET("/assessments/:id", controllers.GetMMSEAssessment)
			mmse.POST("/assessments/:id/ai", controllers.EvaluateMMSEWithAI)
		}

		ccrt := api.Group("/ccrt")
		{
			ccrt.POST("/artifacts/presign", controllers.PresignCCRTArtifact)
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
			ai.POST("/asr", controllers.GenerateASR)
			ai.POST("/tts", controllers.GenerateTTS)
			ai.POST("/voice-control/plan", controllers.PlanVoiceControl)
			ai.GET("/voices", controllers.ListVoiceProfiles)
			ai.POST("/voices", controllers.CreateVoiceProfile)
			ai.PUT("/voices/:id/default", controllers.SetDefaultVoiceProfile)
			ai.DELETE("/voices/:id", controllers.DeleteVoiceProfile)
		}

		// Cognitive profile & training plan
		api.GET("/cognitive-profile", controllers.GetCognitiveProfile)
		plan := api.Group("/training-plan")
		{
			plan.POST("/generate", controllers.GenerateTrainingPlan)
			plan.GET("/current", controllers.GetCurrentTrainingPlan)
		}

		// Analytics
		analytics := api.Group("/analytics")
		{
			analytics.GET("/cognitive-trend", controllers.GetCognitiveTrend)
			analytics.GET("/training-summary", controllers.GetTrainingSummary)
			analytics.POST("/ai-report", controllers.GenerateAIReport)
		}
	}

	return r
}
