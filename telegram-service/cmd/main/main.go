package main

import (
	"github.com/joho/godotenv"
	"telegram-service/internal/app"
	"telegram-service/internal/config"
	"telegram-service/pkg/logging"
)

func main() {
	logger := logging.GetLogger()

	if err := godotenv.Load(); err != nil {
		logger.Panic(err)
	}

	cfg, err := config.GetConfig()
	if err != nil {
		logger.Panic(err)
	}

	if err = app.Run(cfg); err != nil {
		logger.Panic(err)
	}
}