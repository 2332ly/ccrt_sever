package main

import (
	"ccrt_sever/config"
	"ccrt_sever/router"
	"ccrt_sever/services"
	"fmt"
)

func main() {
	config.InitConfig()
	services.StartMedicationReminderScheduler()
	services.StartGenericReminderScheduler()
	r := router.SetupRouter()
	port := config.AppConfig.App.Port
	if port == "" {
		port = ":8080"
	}
	fmt.Printf("Server starting on %s\n", port)
	r.Run(port)
}
