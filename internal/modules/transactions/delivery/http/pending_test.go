package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

func TestTransactionHandler_PendingNotFoundAndAbsentFromList(t *testing.T) {
	owner, id := uuid.New(), uuid.New()
	for _, listing := range []bool{false, true} {
		service := new(mockTransactionService)
		path := "/transactions/" + id.String()
		if listing {
			path = "/transactions"
			service.On("ListTransactions", mock.Anything, owner, mock.Anything).Return([]domain.Transaction{}, nil).Once()
		} else {
			service.On("GetTransaction", mock.Anything, owner, id).Return(nil, apperrors.ErrNotFound).Once()
		}
		router := setupTransactionRouter(service, true, owner)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if listing {
			require.Equal(t, http.StatusOK, w.Code)
			var body struct {
				Data []domain.Transaction `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.NotNil(t, body.Data)
			require.Empty(t, body.Data)
		} else {
			require.Equal(t, http.StatusNotFound, w.Code)
		}
		service.AssertExpectations(t)
	}
}

func TestTransactionResponsePreservesPresentPendingAndCompletedFields(t *testing.T) {
	date := time.Date(2026, 10, 7, 1, 2, 3, 0, time.UTC)
	for _, status := range []domain.TransactionStatus{domain.TransactionStatusPending, domain.TransactionStatusSuccess} {
		response := transactionResponseFromDomain(&domain.Transaction{
			Status: status, MerchantName: "Coffee", Amount: 12.5, TransactionDate: date,
		})
		require.Equal(t, status, response.Status)
		require.Equal(t, "Coffee", *response.MerchantName)
		require.Equal(t, 12.5, *response.Amount)
		require.Equal(t, date.Format(time.RFC3339), *response.TransactionDate)
	}
}
