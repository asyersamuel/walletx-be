package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"walletx-be/internal/domain"
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/models"
	"walletx-be/pkg/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockCategoryService struct {
	services.CategoryService
	mock.Mock
}

func (m *MockCategoryService) CreateCategory(ctx context.Context, userID uuid.UUID, name, icon string) (*models.Category, error) {
	args := m.Called(ctx, userID, name, icon)
	if args.Get(0) != nil {
		return args.Get(0).(*models.Category), args.Error(1)
	}
	return nil, args.Error(1)
}

func setupTestRouter(userID string, handler *handlers.CategoryHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	
	if userID != "" {
		r.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
	}
	
	r.POST("/api/v1/categories", handler.CreateCategory)
	return r
}

func TestCreateCategory(t *testing.T) {
	userID := uuid.New()
	
	tests := []struct {
		name           string
		authUserID     string // If empty, simulates no auth
		requestBody    map[string]interface{}
		setupMock      func(mockSvc *MockCategoryService)
		expectedStatus int
	}{
		{
			name:       "Success - Category Created",
			authUserID: userID.String(),
			requestBody: map[string]interface{}{
				"name": "Makanan",
				"icon": "🍔",
			},
			setupMock: func(mockSvc *MockCategoryService) {
				expectedCat := &models.Category{
					ID:     uuid.New(),
					UserID: userID,
					Name:   "Makanan",
					Icon:   "🍔",
				}
				mockSvc.On("CreateCategory", mock.Anything, userID, "Makanan", "🍔").Return(expectedCat, nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:       "Error - Missing Auth Context",
			authUserID: "", // Simulate unauthenticated
			requestBody: map[string]interface{}{
				"name": "Makanan",
			},
			setupMock: func(mockSvc *MockCategoryService) {
				// No mock calls expected
			},
			expectedStatus: http.StatusUnauthorized, // From utils.UnauthorizedResponse
		},
		{
			name:       "Error - Invalid JSON Payload",
			authUserID: userID.String(),
			requestBody: map[string]interface{}{
				// name is required, simulate missing/invalid by sending something else
				"nama": "Makanan", 
			},
			setupMock: func(mockSvc *MockCategoryService) {
				// No mock calls expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "Error - Duplicate Category",
			authUserID: userID.String(),
			requestBody: map[string]interface{}{
				"name": "Makanan",
			},
			setupMock: func(mockSvc *MockCategoryService) {
				mockSvc.On("CreateCategory", mock.Anything, userID, "Makanan", "").Return(nil, domain.ErrDuplicate)
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockCategoryService)
			tt.setupMock(mockSvc)

			handler := handlers.NewCategoryHandler(mockSvc)
			router := setupTestRouter(tt.authUserID, handler)

			body, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			mockSvc.AssertExpectations(t)
		})
	}
}
