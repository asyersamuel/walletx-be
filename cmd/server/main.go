package main

import (
	"os"

	"walletx-be/internal/workers"
	"github.com/sirupsen/logrus"
)


func init() {
	// Standarisasi Logrus Global
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
}

func main() {
    logrus.Info("🚀 Memulai Aplikasi WalletX...")

    // 1. Load Configuration
    cfg := config.Load()

    // 2. Inisialisasi Service
    txService := services.NewTransactionService()

    // 3. Menjalankan Worker menggunakan data dari Config
    logrus.WithFields(logrus.Fields{
        "bot_email": cfg.IMAP.Email,
    }).Info("Menginisialisasi IMAP Worker")

    // Ambil kredensial langsung dari object cfg
    worker := workers.NewIMAPWorker(cfg.IMAP.Email, cfg.IMAP.Password, txService)
    
    err := worker.ProcessUnseenEmails()
    
    if err != nil {
        logrus.WithError(err).Error("❌ IMAP Worker berhenti karena error")
    } else {
        logrus.Info("✅ IMAP Worker berhasil dieksekusi")
    }
}