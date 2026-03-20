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
	Update(transaction *models.Transaction) error
	Delete(id uuid.UUID) error
	ListByUserID(userID uuid.UUID, limit, offset int, dateFilter, lastUpdated string) ([]models.Transaction, error)
	SearchByMerchant(userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error)
	GetExpensesByCategory(userID uuid.UUID, month, year int) ([]map[string]interface{}, error)
	GetForExport(userID uuid.UUID, month, year int) ([]models.Transaction, error)
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
func (r *transactionRepository) ListByUserID(userID uuid.UUID, limit, offset int, dateFilter, lastUpdated string) ([]models.Transaction, error) {
    var items []models.Transaction
    
    // Gunakan Unscoped agar data yang di-soft-delete tetap terbaca saat sinkronisasi
    q := r.db.Unscoped().Where("user_id = ?", userID)
    
    if dateFilter != "" {
        q = q.Where("DATE(transaction_date) = ?", dateFilter)
    }
    
    // DELTA SYNC LOGIC
    if lastUpdated != "" {
        q = q.Where("updated_at >= ? OR deleted_at >= ?", lastUpdated, lastUpdated)
    } else {
        // Jika tidak ada lastUpdated (first load), jangan kirim yang sudah dihapus
        q = q.Where("deleted_at IS NULL")
    }
    
    q = q.Order("transaction_date DESC")
    if limit > 0 { q = q.Limit(limit) }
    if offset > 0 { q = q.Offset(offset) }
    
    if err := q.Find(&items).Error; err != nil { return nil, err }
    return items, nil
}

// Pencarian Transaksi
func (r *transactionRepository) SearchByMerchant(userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error) {
    var items []models.Transaction
    // ILIKE untuk case-insensitive di PostgreSQL
    q := r.db.Where("user_id = ? AND merchant ILIKE ?", userID, "%"+query+"%").
              Order("transaction_date DESC")
    
    if limit > 0 { q = q.Limit(limit) }
    if offset > 0 { q = q.Offset(offset) }
    
    if err := q.Find(&items).Error; err != nil { return nil, err }
    return items, nil
}

// Analitik Laporan
func (r *transactionRepository) GetExpensesByCategory(userID uuid.UUID, month, year int) ([]map[string]interface{}, error) {
    var results []map[string]interface{}
    // Raw SQL untuk grouping
    query := `
        SELECT COALESCE(c.name, 'Lainnya') as category_name, SUM(t.amount) as total_amount
        FROM transactions t
        LEFT JOIN categories c ON t.category_id = c.id
        WHERE t.user_id = ? 
          AND EXTRACT(MONTH FROM t.transaction_date) = ? 
          AND EXTRACT(YEAR FROM t.transaction_date) = ?
          AND t.deleted_at IS NULL
        GROUP BY c.name
        ORDER BY total_amount DESC
    `
    if err := r.db.Raw(query, userID, month, year).Scan(&results).Error; err != nil {
        return nil, err
    }
    return results, nil
}

func (r *transactionRepository) GetForExport(userID uuid.UUID, month, year int) ([]models.Transaction, error) {
    var items []models.Transaction
    
    // Ambil semua transaksi di bulan dan tahun yang diminta (abaikan yang sudah dihapus)
    q := r.db.Preload("Category"). // Preload agar nama kategori ikut terambil untuk CSV
        Where("user_id = ?", userID).
        Where("EXTRACT(MONTH FROM transaction_date) = ?", month).
        Where("EXTRACT(YEAR FROM transaction_date) = ?", year).
        Where("deleted_at IS NULL"). // Jangan ikutkan data yang sudah di-soft delete
        Order("transaction_date ASC") // Urutkan dari tanggal terlama ke terbaru
        
    if err := q.Find(&items).Error; err != nil {
        return nil, err
    }
    return items, nil
}

func (r *transactionRepository) Update(transaction *models.Transaction) error {
	return r.db.Save(transaction).Error
}

func (r *transactionRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.Transaction{}, "id = ?", id).Error
}	