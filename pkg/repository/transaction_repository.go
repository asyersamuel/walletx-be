package repository

import (
	"walletx-be/pkg/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TransactionRepository mendefinisikan kontrak fungsi untuk entitas Transaction
// RESTful API compliant - supports filtering, sorting, and pagination
type TransactionRepository interface {
	Create(transaction *models.Transaction) error
	GetByID(id uuid.UUID) (*models.Transaction, error)
	Update(transaction *models.Transaction) error
	Delete(id uuid.UUID) error
	ListByUserID(userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, error)
	CountByUserID(userID uuid.UUID, filters TransactionFilters) (int64, error)
	SearchByMerchant(userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error)
	CountSearchByMerchant(userID uuid.UUID, query string) (int64, error)
	GetExpensesByCategory(userID uuid.UUID, month, year int) ([]map[string]interface{}, error)
	GetForExport(userID uuid.UUID, month, year int) ([]models.Transaction, error)
}

// TransactionFilters holds filtering options for transaction queries
type TransactionFilters struct {
	DateFrom      string  // YYYY-MM-DD format
	DateTo        string  // YYYY-MM-DD format
	AmountMin     *float64
	AmountMax     *float64
	LastUpdated   string  // For delta sync
	ExcludeDeleted bool   // Whether to exclude soft-deleted records
}

// SortOption holds sorting configuration
type SortOption struct {
	Field     string // Field to sort by (transaction_date, amount, created_at, etc.)
	Direction string // "asc" or "desc"
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

// ListByUserID: Mengambil daftar transaksi milik 1 user dengan pagination, filtering, dan sorting
// RESTful compliant - supports:
// - Date range: filters.DateFrom, filters.DateTo (YYYY-MM-DD)
// - Amount range: filters.AmountMin, filters.AmountMax
// - Delta sync: filters.LastUpdated (ISO 8601 timestamp)
// - Custom sorting: sort.Field (transaction_date, amount, created_at, etc.) + sort.Direction (ASC/DESC)
// - Default sort: transaction_date DESC
// - SQL injection protection via whitelist validation
func (r *transactionRepository) ListByUserID(userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, error) {
    var items []models.Transaction
    
    q := r.db.Where("user_id = ?", userID)
    
    // Date range filtering
    if filters.DateFrom != "" {
        q = q.Where("DATE(transaction_date) >= ?", filters.DateFrom)
    }
    if filters.DateTo != "" {
        q = q.Where("DATE(transaction_date) <= ?", filters.DateTo)
    }
    
    // Amount range filtering
    if filters.AmountMin != nil {
        q = q.Where("amount >= ?", *filters.AmountMin)
    }
    if filters.AmountMax != nil {
        q = q.Where("amount <= ?", *filters.AmountMax)
    }
    
    // Delta sync logic
    if filters.LastUpdated != "" {
        q = q.Unscoped().Where("updated_at >= ? OR deleted_at >= ?", filters.LastUpdated, filters.LastUpdated)
    } else {
        // Exclude soft-deleted on first load
        if filters.ExcludeDeleted {
            q = q.Where("deleted_at IS NULL")
        }
    }
    
    // Apply sorting
    sortField := sort.Field
    if sortField == "" {
        sortField = "transaction_date"
    }
    
    sortDirection := sort.Direction
    if sortDirection == "" {
        sortDirection = "DESC"
    }
    
    // Validate sort field to prevent SQL injection
    validFields := map[string]bool{
        "transaction_date": true,
        "amount":           true,
        "created_at":       true,
        "updated_at":       true,
        "merchant":         true,
    }
    
    if !validFields[sortField] {
        sortField = "transaction_date"
    }
    
    if sortDirection != "ASC" && sortDirection != "DESC" {
        sortDirection = "DESC"
    }
    
    q = q.Order(sortField + " " + sortDirection)
    
    // Apply pagination
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

// CountByUserID: Menghitung total transaksi milik user untuk pagination metadata
func (r *transactionRepository) CountByUserID(userID uuid.UUID, filters TransactionFilters) (int64, error) {
    var count int64
    
    q := r.db.Model(&models.Transaction{}).Where("user_id = ?", userID)
    
    // Date range filtering
    if filters.DateFrom != "" {
        q = q.Where("DATE(transaction_date) >= ?", filters.DateFrom)
    }
    if filters.DateTo != "" {
        q = q.Where("DATE(transaction_date) <= ?", filters.DateTo)
    }
    
    // Amount range filtering
    if filters.AmountMin != nil {
        q = q.Where("amount >= ?", *filters.AmountMin)
    }
    if filters.AmountMax != nil {
        q = q.Where("amount <= ?", *filters.AmountMax)
    }
    
    // Delta sync / soft-delete handling
    if filters.LastUpdated != "" {
        q = q.Unscoped().Where("updated_at >= ? OR deleted_at >= ?", filters.LastUpdated, filters.LastUpdated)
    } else {
        if filters.ExcludeDeleted {
            q = q.Where("deleted_at IS NULL")
        }
    }
    
    if err := q.Count(&count).Error; err != nil {
        return 0, err
    }
    return count, nil
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

// CountSearchByMerchant: Menghitung hasil pencarian untuk pagination
func (r *transactionRepository) CountSearchByMerchant(userID uuid.UUID, query string) (int64, error) {
    var count int64
    q := r.db.Model(&models.Transaction{}).Where("user_id = ? AND merchant ILIKE ?", userID, "%"+query+"%")
    
    if err := q.Count(&count).Error; err != nil {
        return 0, err
    }
    return count, nil
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