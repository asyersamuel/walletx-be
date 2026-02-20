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

	// Kredensial Bot
	emailBot := "walletxforyourfuture@gmail.com"
	appPassword := "ihytzwlhrjiapknn"

	// Inisialisasi Service
	logrus.WithFields(logrus.Fields{
		"email": emailBot,
	}).Info("Menginisialisasi Service")
	txService := services.NewTransactionService()

	// Mencatat event spesifik menggunakan WithFields
	logrus.WithFields(logrus.Fields{
		"email": emailBot,
	}).Info("Menginisialisasi IMAP Worker")

	// Menjalankan Worker
	worker := workers.NewIMAPWorker(emailBot, appPassword, txService)
	err := worker.ProcessUnseenEmails()
	
	if err != nil {
		// Menggunakan .WithError untuk standarisasi log error
		logrus.WithError(err).Error("❌ IMAP Worker berhenti karena error")
	} else {
		logrus.Info("✅ IMAP Worker berhasil dieksekusi")
	}
}