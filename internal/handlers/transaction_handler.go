package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
)

func GetWalletStatus(w http.ResponseWriter, r *http.Request) {
	logrus.Info("Memulai pengecekan saldo wallet di database")

	response := map[string]string{
		"status":  "success",
		"message": "WalletX API is Running!",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}