package main

import (
	"fmt"
	"net/http"

	// Sesuaikan dengan module name yang kamu buat di go mod init tadi
	"walletx-be/internal/handlers"
)

func main() {
	// 1. Tentukan Route (Jalur API)
	http.HandleFunc("/api/status", handlers.GetWalletStatus)

	// 2. Tentukan Port
	port := ":8080"
	fmt.Println("Server WalletX berjalan di http://localhost" + port)

	// 3. Jalankan Server
	err := http.ListenAndServe(port, nil)
	if err != nil {
		fmt.Printf("Gagal menjalankan server: %v\n", err)
	}
}