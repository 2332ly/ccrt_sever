package config

import (
	"log"

	"github.com/spf13/viper"
)

type config struct {
	App struct {
		Name string
		Port string
	}
	Database struct {
		Dsn          string
		MaxIdleConns int
		MaxOpenConns int
	}
}

var AppConfig *config

func InitConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config/")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Error reading config file: ", err)
	}
	AppConfig = &config{}
	err := viper.Unmarshal(&AppConfig)
	if err != nil {
		log.Fatal("Error unmarshaling config file: ", err)
	}
	initDB()
}
