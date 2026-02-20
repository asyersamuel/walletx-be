package services

import (
	"github.com/sirupsen/logrus"
)

// TransactionService menangani logika bisnis transaksi
type TransactionService struct {
	// Nanti di sini akan ada Repository (Database)
}

func NewTransactionService() *TransactionService {
	return &TransactionService{}
}

// ProcessTransactionEmail adalah fungsi yang akan dipanggil oleh Worker
func (s *TransactionService) ProcessTransactionEmail(rawBody string, emailOwner string) error {
	logrus.WithFields(logrus.Fields{
		"user": emailOwner,
	}).Info("🧠 [Service] Memproses data transaksi dari email")

	// 1. Nanti di sini panggil Parser (Regex) untuk ambil angka
	// 2. Nanti di sini panggil Repository untuk cek idempotency & simpan
	
	return nil
}