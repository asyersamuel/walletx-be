package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"walletx-be/internal/middleware"
	transactionservice "walletx-be/internal/modules/transactions/service"

	domain "walletx-be/internal/modules/transactions/contracts"

	apperrors "walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockTransactionService struct {
	mock.Mock
}

type mockAgentService struct{ mock.Mock }

func (m *mockAgentService) Handle(ctx context.Context, userID uuid.UUID, prompt string) (domain.AgentResponse, error) {
	args := m.Called(ctx, userID, prompt)
	result, _ := args.Get(0).(domain.AgentResponse)
	return result, args.Error(1)
}

type mockReceiptService struct{ mock.Mock }

func (m *mockReceiptService) ProcessReceipt(ctx context.Context, userID uuid.UUID, contentType, filename string, image []byte) (domain.AgentResponse, error) {
	args := m.Called(ctx, userID, contentType, filename, image)
	result, _ := args.Get(0).(domain.AgentResponse)
	return result, args.Error(1)
}

var _ domain.AgentService = (*mockAgentService)(nil)
var _ domain.ReceiptService = (*mockReceiptService)(nil)

func (m *mockTransactionService) CreateTransaction(ctx context.Context, userID uuid.UUID, input domain.CreateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, input)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}

func (m *mockTransactionService) GetTransaction(ctx context.Context, userID, transactionID uuid.UUID) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, transactionID)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}

func (m *mockTransactionService) ListTransactions(ctx context.Context, userID uuid.UUID, input domain.ListTransactionsInput) ([]domain.Transaction, error) {
	args := m.Called(ctx, userID, input)
	transactions, _ := args.Get(0).([]domain.Transaction)
	return transactions, args.Error(1)
}

func (m *mockTransactionService) UpdateTransaction(ctx context.Context, userID, transactionID uuid.UUID, input domain.UpdateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, transactionID, input)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}

func (m *mockTransactionService) DeleteTransaction(ctx context.Context, userID, transactionID uuid.UUID) error {
	return m.Called(ctx, userID, transactionID).Error(0)
}

var _ domain.TransactionService = (*mockTransactionService)(nil)

type mockTokenValidator struct {
	claims *middleware.TokenClaims
	err    error
}

func (m mockTokenValidator) ValidateToken(context.Context, string) (*middleware.TokenClaims, error) {
	return m.claims, m.err
}

func setupTransactionRouter(service domain.TransactionService, withAuth bool, userID uuid.UUID, dependencies ...interface{}) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(service, dependencies...)
	group := router.Group("")
	if withAuth {
		group.Use(func(c *gin.Context) {
			c.Set("user_id", userID.String())
			c.Next()
		})
	}
	RegisterProtectedRoutes(group, handler)
	return router
}

func TestTransactionHandler_Create_NoTokenReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := new(mockTransactionService)
	router := gin.New()
	router.Use(middleware.AuthMiddleware(mockTokenValidator{}))
	RegisterProtectedRoutes(router.Group(""), NewHandler(service))

	req := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	service.AssertNotCalled(t, "CreateTransaction", mock.Anything, mock.Anything, mock.Anything)
}

func TestTransactionHandler_Create_ValidAuthInjectsUserAndIgnoresBodyUserID(t *testing.T) {
	userID := uuid.New()
	bodyUserID := uuid.New()
	categoryID := uuid.New()
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	service := new(mockTransactionService)
	wantInput := domain.CreateTransactionInput{UserID: userID, CategoryID: categoryID, CategoryName: "Food", MerchantName: "Cafe", Amount: 10, TransactionDate: date}
	created := &domain.Transaction{ID: uuid.New(), UserID: userID, CategoryID: &categoryID, MerchantName: "Cafe", Amount: 10, TransactionDate: date, CreatedAt: date, UpdatedAt: date}
	service.On("CreateTransaction", mock.Anything, userID, wantInput).Return(created, nil).Once()

	router := setupTransactionRouter(service, true, userID)
	req := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{"user_id":"`+bodyUserID.String()+`","category_id":"`+categoryID.String()+`","category_name":"Food","merchant_name":"Cafe","amount":10,"transaction_date":"2026-09-20T12:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusCreated, recorder.Code)
	service.AssertExpectations(t)
}

func TestTransactionHandler_Create_SuccessReturnsTransactionDTO(t *testing.T) {
	userID := uuid.New()
	categoryID := uuid.New()
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	service := new(mockTransactionService)
	created := &domain.Transaction{ID: uuid.New(), UserID: userID, CategoryID: &categoryID, MerchantName: "Cafe", Amount: 10, TransactionDate: date, CreatedAt: date, UpdatedAt: date}
	service.On("CreateTransaction", mock.Anything, userID, mock.Anything).Return(created, nil).Once()

	router := setupTransactionRouter(service, true, userID)
	req := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{"category_id":"`+categoryID.String()+`","merchant_name":"Cafe","amount":10,"transaction_date":"2026-09-20T12:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusCreated, recorder.Code)
	var envelope response.APIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, created.ID.String(), data["id"])
	assert.Equal(t, "Cafe", data["merchant_name"])
	service.AssertExpectations(t)
}

func TestTransactionHandler_Create_ValidationResponses(t *testing.T) {
	userID := uuid.New()
	categoryID := uuid.New()
	tests := []struct {
		name       string
		body       string
		serviceErr error
	}{
		{name: "zero amount", body: `{"category_id":"` + categoryID.String() + `","merchant_name":"Cafe","amount":0,"transaction_date":"2026-09-20T12:00:00Z"}`, serviceErr: transactionservice.ErrAmountMustBePositive},
		{name: "negative amount", body: `{"category_id":"` + categoryID.String() + `","merchant_name":"Cafe","amount":-1,"transaction_date":"2026-09-20T12:00:00Z"}`, serviceErr: transactionservice.ErrAmountMustBePositive},
		{name: "missing merchant", body: `{"category_id":"` + categoryID.String() + `","amount":10,"transaction_date":"2026-09-20T12:00:00Z"}`, serviceErr: transactionservice.InvalidInputError("merchant name must not be empty")},
		{name: "invalid date", body: `{"category_id":"` + categoryID.String() + `","merchant_name":"Cafe","amount":10,"transaction_date":"not-a-date"}`},
		{name: "empty body", body: ""},
		{name: "malformed JSON", body: `{"category_id":`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := new(mockTransactionService)
			if tt.serviceErr != nil {
				service.On("CreateTransaction", mock.Anything, userID, mock.Anything).Return(nil, tt.serviceErr).Once()
			}
			router := setupTransactionRouter(service, true, userID)
			req := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			if tt.serviceErr == nil {
				service.AssertNotCalled(t, "CreateTransaction", mock.Anything, mock.Anything, mock.Anything)
			} else {
				service.AssertExpectations(t)
			}
		})
	}
}

func TestTransactionHandler_Create_ServiceErrors(t *testing.T) {
	userID := uuid.New()
	categoryID := uuid.New()
	body := `{"category_id":"` + categoryID.String() + `","merchant_name":"Cafe","amount":10,"transaction_date":"2026-09-20T12:00:00Z"}`

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "service validation", err: transactionservice.ErrAmountMustBePositive, want: http.StatusBadRequest},
		{name: "category not found", err: apperrors.ErrNotFound, want: http.StatusNotFound},
		{name: "internal error", err: apperrors.ErrInternal, want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := new(mockTransactionService)
			service.On("CreateTransaction", mock.Anything, userID, mock.Anything).Return(nil, tt.err).Once()
			router := setupTransactionRouter(service, true, userID)
			req := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.want, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "stack")
			service.AssertExpectations(t)
		})
	}
}

func TestTransactionHandler_Get_SuccessAndInvalidUUID(t *testing.T) {
	userID := uuid.New()
	transactionID := uuid.New()
	categoryID := uuid.New()
	service := new(mockTransactionService)
	want := &domain.Transaction{ID: transactionID, UserID: userID, CategoryID: &categoryID, MerchantName: "Cafe", Amount: 10, TransactionDate: time.Now()}
	service.On("GetTransaction", mock.Anything, userID, transactionID).Return(want, nil).Once()
	router := setupTransactionRouter(service, true, userID)

	req := httptest.NewRequest(http.MethodGet, "/transactions/"+transactionID.String(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusOK, recorder.Code)
	service.AssertExpectations(t)

	invalid := httptest.NewRequest(http.MethodGet, "/transactions/not-a-uuid", nil)
	invalidRecorder := httptest.NewRecorder()
	router.ServeHTTP(invalidRecorder, invalid)
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
}

func TestTransactionHandler_Get_CrossUserNotFoundHasNoData(t *testing.T) {
	userID := uuid.New()
	transactionID := uuid.New()
	service := new(mockTransactionService)
	service.On("GetTransaction", mock.Anything, userID, transactionID).Return(nil, apperrors.ErrNotFound).Once()
	router := setupTransactionRouter(service, true, userID)

	req := httptest.NewRequest(http.MethodGet, "/transactions/"+transactionID.String(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), transactionID.String())
	service.AssertExpectations(t)
}

func TestTransactionHandler_List_ParsesPaginationAndDates(t *testing.T) {
	userID := uuid.New()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	service := new(mockTransactionService)
	wantInput := domain.ListTransactionsInput{UserID: userID, Limit: 10, Offset: 5, StartDate: &start, EndDate: &end}
	service.On("ListTransactions", mock.Anything, userID, wantInput).Return([]domain.Transaction{}, nil).Once()
	router := setupTransactionRouter(service, true, userID)

	req := httptest.NewRequest(http.MethodGet, "/transactions?limit=10&offset=5&start_date=2026-09-01T00:00:00Z&end_date=2026-09-30T23:59:59Z", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"data":[]`)
	service.AssertExpectations(t)
}

func TestTransactionHandler_List_RejectsInvalidPaginationAndDates(t *testing.T) {
	userID := uuid.New()
	tests := []string{
		"/transactions?limit=-1",
		"/transactions?offset=-1",
		"/transactions?limit=nope",
		"/transactions?start_date=bad-date",
		"/transactions?end_date=bad-date",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			service := new(mockTransactionService)
			router := setupTransactionRouter(service, true, userID)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			service.AssertNotCalled(t, "ListTransactions", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestTransactionHandler_List_EmptyResultIsJSONArray(t *testing.T) {
	userID := uuid.New()
	service := new(mockTransactionService)
	service.On("ListTransactions", mock.Anything, userID, mock.Anything).Return([]domain.Transaction{}, nil).Once()
	router := setupTransactionRouter(service, true, userID)

	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"data":[]`)
	service.AssertExpectations(t)
}

func TestTransactionHandler_List_ServiceNotFoundMaps404(t *testing.T) {
	userID := uuid.New()
	service := new(mockTransactionService)
	service.On("ListTransactions", mock.Anything, userID, mock.Anything).Return(nil, apperrors.ErrNotFound).Once()
	router := setupTransactionRouter(service, true, userID)

	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "merchant_name")
}

func TestTransactionHandler_Update_SuccessAndValidation(t *testing.T) {
	userID := uuid.New()
	transactionID := uuid.New()
	service := new(mockTransactionService)
	updated := &domain.Transaction{ID: transactionID, UserID: userID, MerchantName: "Updated", Amount: 20, TransactionDate: time.Now()}
	service.On("UpdateTransaction", mock.Anything, userID, transactionID, mock.Anything).Return(updated, nil).Once()
	router := setupTransactionRouter(service, true, userID)

	req := httptest.NewRequest(http.MethodPatch, "/transactions/"+transactionID.String(), strings.NewReader(`{"merchant_name":"Updated","amount":20}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusOK, recorder.Code)
	service.AssertExpectations(t)

	invalidService := new(mockTransactionService)
	invalidService.On("UpdateTransaction", mock.Anything, userID, transactionID, mock.Anything).Return(nil, transactionservice.ErrAmountMustBePositive).Once()
	invalidRouter := setupTransactionRouter(invalidService, true, userID)
	invalidReq := httptest.NewRequest(http.MethodPatch, "/transactions/"+transactionID.String(), strings.NewReader(`{"amount":-1}`))
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRecorder := httptest.NewRecorder()
	invalidRouter.ServeHTTP(invalidRecorder, invalidReq)
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
	invalidService.AssertExpectations(t)
}

func TestTransactionHandler_Update_InvalidUUIDAndNotFound(t *testing.T) {
	userID := uuid.New()
	service := new(mockTransactionService)
	router := setupTransactionRouter(service, true, userID)

	invalid := httptest.NewRequest(http.MethodPatch, "/transactions/bad", strings.NewReader(`{"amount":10}`))
	invalidRecorder := httptest.NewRecorder()
	router.ServeHTTP(invalidRecorder, invalid)
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)

	transactionID := uuid.New()
	service.On("UpdateTransaction", mock.Anything, userID, transactionID, mock.Anything).Return(nil, apperrors.ErrNotFound).Once()
	notFound := httptest.NewRequest(http.MethodPatch, "/transactions/"+transactionID.String(), strings.NewReader(`{"amount":10}`))
	notFoundRecorder := httptest.NewRecorder()
	router.ServeHTTP(notFoundRecorder, notFound)
	assert.Equal(t, http.StatusNotFound, notFoundRecorder.Code)
	assert.NotContains(t, notFoundRecorder.Body.String(), transactionID.String())
}

func TestTransactionHandler_Delete_SuccessNotFoundAndInvalidUUID(t *testing.T) {
	userID := uuid.New()
	transactionID := uuid.New()
	service := new(mockTransactionService)
	service.On("DeleteTransaction", mock.Anything, userID, transactionID).Return(nil).Once()
	router := setupTransactionRouter(service, true, userID)

	req := httptest.NewRequest(http.MethodDelete, "/transactions/"+transactionID.String(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Empty(t, recorder.Body.String())
	service.AssertExpectations(t)

	invalidID := uuid.New()
	service.On("DeleteTransaction", mock.Anything, userID, invalidID).Return(apperrors.ErrNotFound).Once()
	notFound := httptest.NewRequest(http.MethodDelete, "/transactions/"+invalidID.String(), nil)
	notFoundRecorder := httptest.NewRecorder()
	router.ServeHTTP(notFoundRecorder, notFound)
	assert.Equal(t, http.StatusNotFound, notFoundRecorder.Code)
	assert.NotContains(t, notFoundRecorder.Body.String(), invalidID.String())

	badUUID := httptest.NewRequest(http.MethodDelete, "/transactions/not-uuid", nil)
	badUUIDRecorder := httptest.NewRecorder()
	router.ServeHTTP(badUUIDRecorder, badUUID)
	assert.Equal(t, http.StatusBadRequest, badUUIDRecorder.Code)
}

func TestTransactionHandler_Chat_RecordTransactionReturnsCreated(t *testing.T) {
	userID := uuid.New()
	transaction := &domain.Transaction{ID: uuid.New(), UserID: userID, MerchantName: "Cafe", Amount: 50000, TransactionDate: time.Now()}
	agent := new(mockAgentService)
	agent.On("Handle", mock.Anything, userID, "Spent 50k on coffee").Return(domain.AgentResponse{Text: "Transaction recorded.", Transaction: transaction}, nil).Once()
	router := setupTransactionRouter(new(mockTransactionService), true, userID, agent)

	req := httptest.NewRequest(http.MethodPost, "/transactions/chat", strings.NewReader(`{"message":"Spent 50k on coffee"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Contains(t, recorder.Body.String(), transaction.ID.String())
	agent.AssertExpectations(t)
}

func TestTransactionHandler_Chat_AnalyticalPromptReturnsRAGAnswer(t *testing.T) {
	userID := uuid.New()
	agent := new(mockAgentService)
	agent.On("Handle", mock.Anything, userID, "biggest expense").Return(domain.AgentResponse{
		Text:         "Your biggest expense was coffee.",
		SimilarItems: []domain.SimilarTransaction{{ID: uuid.New(), UserID: userID, MerchantName: "Coffee", Similarity: 0.9}},
	}, nil).Once()
	router := setupTransactionRouter(new(mockTransactionService), true, userID, agent)

	req := httptest.NewRequest(http.MethodPost, "/transactions/chat", strings.NewReader(`{"message":"biggest expense"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Your biggest expense was coffee.")
	agent.AssertExpectations(t)
}

func TestTransactionHandler_Chat_FallbackAndSecurity(t *testing.T) {
	userID := uuid.New()
	agent := new(mockAgentService)
	agent.On("Handle", mock.Anything, userID, "weather").Return(domain.AgentResponse{Text: transactionservice.FallbackText}, nil).Once()
	router := setupTransactionRouter(new(mockTransactionService), true, userID, agent)

	req := httptest.NewRequest(http.MethodPost, "/transactions/chat", strings.NewReader(`{"message":"weather"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), transactionservice.FallbackText)

	attacker := uuid.New()
	secured := httptest.NewRequest(http.MethodPost, "/transactions/chat", strings.NewReader(`{"message":"weather","user_id":"`+attacker.String()+`"}`))
	secured.Header.Set("Content-Type", "application/json")
	securedRecorder := httptest.NewRecorder()
	router.ServeHTTP(securedRecorder, secured)
	assert.Equal(t, http.StatusBadRequest, securedRecorder.Code)
}

func TestTransactionHandler_Chat_AIValidationAndUnavailableErrors(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "malformed tool arguments", err: apperrors.ErrInvalidInput, want: http.StatusBadRequest},
		{name: "ai internal timeout", err: apperrors.ErrInternal, want: http.StatusInternalServerError},
		{name: "degraded service", err: errors.New("service unavailable"), want: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := new(mockAgentService)
			agent.On("Handle", mock.Anything, userID, "finance").Return(domain.AgentResponse{}, tt.err).Once()
			router := setupTransactionRouter(new(mockTransactionService), true, userID, agent)
			req := httptest.NewRequest(http.MethodPost, "/transactions/chat", strings.NewReader(`{"message":"finance"}`))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			assert.Equal(t, tt.want, recorder.Code)
			agent.AssertExpectations(t)
		})
	}
}

func TestTransactionHandler_Receipt_ValidImageReturnsCreated(t *testing.T) {
	userID := uuid.New()
	agent := new(mockReceiptService)
	transaction := &domain.Transaction{ID: uuid.New(), UserID: userID, MerchantName: "Cafe", Amount: 10, TransactionDate: time.Now()}
	agent.On("ProcessReceipt", mock.Anything, userID, "image/png", "receipt.png", mock.Anything).Return(domain.AgentResponse{Text: "Receipt recorded.", Transaction: transaction}, nil).Once()
	router := setupTransactionRouter(new(mockTransactionService), true, userID, agent)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("receipt", "receipt.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/transactions/receipt", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Contains(t, recorder.Body.String(), transaction.ID.String())
	agent.AssertExpectations(t)
}

func TestTransactionHandler_Receipt_RejectsInvalidUploadsBeforeService(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name        string
		filename    string
		contentType string
		data        []byte
		want        int
	}{
		{name: "empty", filename: "empty.png", contentType: "image/png", data: nil, want: http.StatusBadRequest},
		{name: "unsupported", filename: "payload.exe", contentType: "application/octet-stream", data: []byte("MZ"), want: http.StatusUnsupportedMediaType},
		{name: "text spoofed as image", filename: "receipt.txt", contentType: "text/plain", data: []byte("not an image"), want: http.StatusUnsupportedMediaType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receipt := new(mockReceiptService)
			router := setupTransactionRouter(new(mockTransactionService), true, userID, receipt)
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			part, err := writer.CreateFormFile("receipt", tt.filename)
			require.NoError(t, err)
			_, err = part.Write(tt.data)
			require.NoError(t, err)
			require.NoError(t, writer.Close())

			req := httptest.NewRequest(http.MethodPost, "/transactions/receipt", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.want, recorder.Code)
			receipt.AssertNotCalled(t, "ProcessReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestTransactionHandler_Receipt_RejectsOversizedUploadBeforeService(t *testing.T) {
	userID := uuid.New()
	receipt := new(mockReceiptService)
	router := setupTransactionRouter(new(mockTransactionService), true, userID, receipt)
	data := bytes.Repeat([]byte("x"), int(maxReceiptBytes)+1)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("receipt", "large.png")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/transactions/receipt", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	receipt.AssertNotCalled(t, "ProcessReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTransactionHandler_Receipt_UnauthenticatedReturns401(t *testing.T) {
	receipt := new(mockReceiptService)
	router := gin.New()
	router.Use(middleware.AuthMiddleware(mockTokenValidator{}))
	RegisterProtectedRoutes(router.Group(""), NewHandler(new(mockTransactionService), receipt))

	req := httptest.NewRequest(http.MethodPost, "/transactions/receipt", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	receipt.AssertNotCalled(t, "ProcessReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
