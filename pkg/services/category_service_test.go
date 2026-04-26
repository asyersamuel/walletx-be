package services

import (
	"errors"
	"testing"

	"walletx-be/pkg/models"
	// "walletx-be/pkg/repository" 

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockCategoryRepository struct {
	mock.Mock
}

func (m *mockCategoryRepository) Create(category *models.Category) error {
	args := m.Called(category)
	return args.Error(0)
}


func (m *mockCategoryRepository) List(userID uuid.UUID) ([]models.Category, error) { return nil, nil }
func (m *mockCategoryRepository) GetByID(id, userID uuid.UUID) (*models.Category, error) { return nil, nil }
func (m *mockCategoryRepository) Update(category *models.Category) error { return nil }
func (m *mockCategoryRepository) Delete(id, userID uuid.UUID) error { return nil }
func (m *mockCategoryRepository) GetByName(name string, userID uuid.UUID) (*models.Category, error) { return nil, nil }

func TestCreateCategory(t *testing.T) {
	// Kita buat UUID sembarang untuk ID user yang pura-puranya sedang login
	userID := uuid.New()

	// Tabel Skenario
	tests := []struct {
		name          string // Nama test
		inputName     string // Data yang mau kita kirim
		inputIcon     string 
		mockBehavior  func(m *mockCategoryRepository) // Skenario untuk si "Pemeran Pengganti"
		expectedError string // Error apa yang kita harapkan? (kosong jika sukses)
	}{
		{
			// SKENARIO 1: SEMUA LANCAR (HAPPY PATH)
			name:      "Success Create Category",
			inputName: "Food",
			inputIcon: "burger.png",
			mockBehavior: func(m *mockCategoryRepository) {
				// Skenario: Jika fungsi Create dipanggil (dengan kategori apa pun), 
				// kembalikan error "nil" (artinya sukses).
				m.On("Create", mock.AnythingOfType("*models.Category")).Return(nil)
			},
			expectedError: "", // Tidak berharap ada error
		},
		{
			// SKENARIO 2: NAMA KOSONG (VALIDASI SERVICE)
			name:      "Error Empty Name",
			inputName: "", // Sengaja dikosongkan
			inputIcon: "burger.png",
			mockBehavior: func(m *mockCategoryRepository) {
				// Perhatikan: Karena nama kosong, Service akan melempar error DULUAN.
				// Fungsi Create di Repository TIDAK AKAN PERNAH dipanggil.
				// Jadi kita tidak perlu mengatur m.On() di sini.
			},
			expectedError: "category name cannot be empty", // Ini pesan error dari service
		},
		{
			// SKENARIO 3: DATABASE ERROR
			name:      "Error From Database",
			inputName: "Salary",
			inputIcon: "money.png",
			mockBehavior: func(m *mockCategoryRepository) {
				// Skenario: Pura-puranya database tiba-tiba mati / duplicate
				m.On("Create", mock.Anything).Return(errors.New("database connection failed"))
			},
			expectedError: "database connection failed",
		},
	}

	// Mesin penjalannya (Loop)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Siapkan Aktor Pengganti (Mock Repo)
			mockRepo := new(mockCategoryRepository)
			
			// 2. Beri naskah skenario ke aktor tersebut (jalankan fungsi mockBehavior)
			tt.mockBehavior(mockRepo)

			// 3. Masukkan aktor pengganti ini ke dalam Chef (Category Service)
			// (Dependency Injection terjadi di sini)
			service := NewCategoryService(mockRepo)

			// 4. ACTION! Panggil fungsi yang mau dites
			result, err := service.CreateCategory(userID, tt.inputName, tt.inputIcon)

			// 5. PENJURIAN (Assertion)
			if tt.expectedError != "" {
				// Jika skenario ini adalah skenario gagal:
				assert.Error(t, err) // Pastikan benar-benar ada error
				assert.Equal(t, tt.expectedError, err.Error()) // Pastikan pesan errornya persis
				assert.Nil(t, result) // Pastikan tidak ada data yang dikembalikan
			} else {
				// Jika skenario ini adalah skenario sukses:
				assert.NoError(t, err) // Pastikan tidak ada error
				assert.NotNil(t, result) // Pastikan data kembalian ada isinya
				assert.Equal(t, tt.inputName, result.Name) // Pastikan namanya sesuai
				assert.Equal(t, userID, result.UserID) // Pastikan User ID-nya sesuai
			}

			// 6. Validasi akhir untuk si Aktor Pengganti
			// Memastikan aktor melakukan adegan persis seperti naskah (misal: "Create" benar dipanggil 1x)
			mockRepo.AssertExpectations(t)
		})
	}
}