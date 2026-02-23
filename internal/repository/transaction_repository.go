package repository

import (
	"walletx-be/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TransactionRepository mendefinisikan kontrak fungsi untuk entitas Transaction
type TransactionRepository interface {
	Create(transaction *models.Transaction) error
	GetByID(id uuid.UUID) (*models.Transaction, error)
	ListByUserID(userID uuid.UUID, limit, offset int) ([]models.Transaction, error)
}

type transactionRepository struct {
	db *gorm.DB
}

// NewTransactionRepository adalah constructor untuk membuat instance TransactionRepository
func NewTransactionRepository(db *gorm.DB) TransactionRepository {
	return &transactionRepository{db: db}
}

// Create: Akan dipanggil oleh Service setelah berhasil membedah isi email
func (r *transactionRepository) Create(transaction *models.Transaction) error {
	return r.db.Create(transaction).Error
}

func (r *transactionRepository) GetByID(id uuid.UUID) (*models.Transaction, error) {
	var transaction models.Transaction
	if err := r.db.First(&transaction, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &transaction, nil
}

// ListByUserID: Mengambil daftar transaksi milik 1 user dengan pagination
func (r *transactionRepository) ListByUserID(userID uuid.UUID, limit, offset int) ([]models.Transaction, error) {
	var items []models.Transaction
	
	// Order by transaction_date (kapan uang benar-benar keluar) menurun
	q := r.db.Where("user_id = ?", userID).Order("transaction_date DESC")
	
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	
	if err := q.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}