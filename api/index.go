package api

import (
	"net/http"
	"os"
	"sync"

	"walletx-be/configs"
	"walletx-be/internal/app"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var (
	httpHandler http.Handler
	appOnce     sync.Once
)

func init() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
}

func initializeApp() {
	if err := godotenv.Load(); err != nil {
		logrus.Warn(".env file not found, using default system variables")
	}

	cfg := configs.Load()

	appInstance, err := app.Run(cfg)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to build application")
		return
	}

	httpHandler = appInstance.Handler
}

func Handler(w http.ResponseWriter, r *http.Request) {
	appOnce.Do(initializeApp)
	httpHandler.ServeHTTP(w, r)
}
