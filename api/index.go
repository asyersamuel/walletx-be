package api

import (
	"net/http"
	"sync"

	"walletx-be/configs"
	"walletx-be/internal/app"
	platformlogger "walletx-be/internal/platform/logger"

	"github.com/joho/godotenv"
)

var (
	httpHandler       http.Handler
	initializationErr error
	appOnce           sync.Once
	appLogger         = platformlogger.NewLogger()
)

func initializeApp() {
	if err := godotenv.Load(); err != nil {
		appLogger.Warn(".env file not found, using default system variables")
	}

	cfg := configs.Load()

	appInstance, err := app.Run(cfg)
	if err != nil {
		initializationErr = err
		appLogger.WithError(err).Error("Failed to build application")
		return
	}

	httpHandler = appInstance.Handler
}

func Handler(w http.ResponseWriter, r *http.Request) {
	appOnce.Do(initializeApp)
	if initializationErr != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	httpHandler.ServeHTTP(w, r)
}
