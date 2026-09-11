package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"walletx-be/configs"
	"walletx-be/internal/app"
	platformlogger "walletx-be/internal/platform/logger"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	appLogger := platformlogger.NewLogger()

	if err := godotenv.Load(); err != nil {
		appLogger.Warn(".env file not found, using default system variables")
	}

	cfg := configs.Load()

	appInstance, err := app.Run(cfg)
	if err != nil {
		appLogger.WithError(err).Error("Failed to build application")
		return
	}
	defer appInstance.Cleanup()

	port := cfg.Server.Port
	if port == "" {
		port = "8080"
	}

	appLogger.WithFields(map[string]interface{}{
		"port": port,
		"env":  os.Getenv("GIN_MODE"),
		"url":  "http://localhost:" + port,
	}).Info("REST API server is starting to listen")

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- appInstance.Handler.(*gin.Engine).Run(":" + port)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		appLogger.WithError(err).Error("Failed to start server")
		return
	case <-quit:
	}

	appLogger.Info("Shutting down server...")
	ctx := context.Background()
	if err := appInstance.ShutdownFunc(ctx); err != nil {
		appLogger.WithError(err).Error("Error during shutdown")
	}
}
