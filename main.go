package main

import (
	"ccrt_sever/config"
	"ccrt_sever/router"
	"fmt"
)

func main() {
	config.InitConfig()
	fmt.Println(config.AppConfig.App.Port)
	r := router.SetupRouter()
	port := config.AppConfig.App.Port
	if port == "" {
		port = "8080"
	}
	r.Run(port)
}
