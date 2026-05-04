package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"walletx-be/internal/bootstrap"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
}

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Warn(".env file not found, using default system variables")
	}

	cfg := bootstrap.LoadConfig()

	app, err := bootstrap.BuildApp(cfg)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to build application")
	}
	defer app.Cleanup()

	port := app.Config.Server.Port
	if port == "" {
		port = "8080"
	}

	logrus.WithFields(logrus.Fields{
		"port": port,
		"env":  os.Getenv("GIN_MODE"),
		"url":  "http://localhost:" + port,
	}).Info("WalletX REST API Server is starting to listen")

	go func() {
		if err := app.Handler.(*gin.Engine).Run(":" + port); err != nil {
			logrus.WithError(err).Fatal("Failed to start server")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logrus.Info("Shutting down server...")
	ctx := context.Background()
	if err := app.ShutdownFunc(ctx); err != nil {
		logrus.WithError(err).Error("Error during shutdown")
	}
}
