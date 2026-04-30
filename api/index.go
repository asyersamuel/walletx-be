package api

import (
	"net/http"
	"os"
	"sync"

	"walletx-be/internal/bootstrap"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var (
	app     http.Handler
	appOnce sync.Once
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

	cfg := bootstrap.LoadConfig()

	appInstance, err := bootstrap.BuildApp(cfg)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to build application")
		return
	}

	app = appInstance.Handler
}

func Handler(w http.ResponseWriter, r *http.Request) {
	appOnce.Do(initializeApp)
	app.ServeHTTP(w, r)
}
