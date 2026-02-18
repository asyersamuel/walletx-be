package handlers

import (
	"encoding/json"
	"net/http"
)

func GetWalletStatus(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status":  "success",
		"message": "WalletX API is Running!",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}