package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/ai"
	domain "walletx-be/internal/modules/transactions/contracts"
	transactionhttp "walletx-be/internal/modules/transactions/delivery/http"
	"walletx-be/internal/modules/transactions/service"
	"walletx-be/internal/platform/storage"
	apperrors "walletx-be/internal/shared/errors"
)

type flowExtractor func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error)

func (f flowExtractor) ExtractTransaction(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
	return f(ctx, input)
}

type flowCategories func(context.Context, uuid.UUID) ([]ai.CategoryOption, error)

func (f flowCategories) ListByUser(ctx context.Context, user uuid.UUID) ([]ai.CategoryOption, error) {
	return f(ctx, user)
}

type flowEmbedding func(context.Context, string) ([]float32, error)

func (f flowEmbedding) Generate(ctx context.Context, text string) ([]float32, error) {
	return f(ctx, text)
}

type flowValidator struct{ user uuid.UUID }

func (v flowValidator) ValidateToken(context.Context, string) (*middleware.TokenClaims, error) {
	return &middleware.TokenClaims{UserID: v.user.String()}, nil
}

func receiptRequest(t *testing.T, handler *transactionhttp.Handler, user uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	transactionhttp.RegisterProtectedRoutes(router.Group("/api/v1", middleware.AuthMiddleware(flowValidator{user})), handler)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("receipt", "receipt.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/transactions/receipt", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestReceiptFlow_CommittedBeforeAIAndHTTPOutcomes(t *testing.T) {
	skipIfNoDatabase(t)
	for _, scenario := range []string{"success", "provider failure", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			truncateTables(t)
			ctx := context.Background()
			user := seedUser(t)
			category := seedCategory(t, user, "Food", "expense")
			store, err := storage.NewLocal(t.TempDir())
			require.NoError(t, err)
			repo := newTestRepository(t)
			var embeds atomic.Int32
			embedding := flowEmbedding(func(ctx context.Context, text string) ([]float32, error) {
				embeds.Add(1)
				vector := make([]float32, domain.EmbeddingDimension)
				vector[0] = 1
				return vector, nil
			})
			transactions := service.NewTransactionServiceWithLogger(repo, embedding, nopLogger{}, store)
			var pendingID uuid.UUID
			extractor := flowExtractor(func(extractionCtx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
				var status, reference string
				var allNull bool
				// This independent connection can see the row only if pending insertion committed.
				require.NoError(t, testPool.QueryRow(ctx, `select id,status,receipt_image_url,
      amount is null and merchant_name is null and category_id is null and transaction_date is null
      from public.transactions where user_id=$1`, user).Scan(&pendingID, &status, &reference, &allNull))
				require.Equal(t, "pending", status)
				require.True(t, allNull)
				image, err := store.ReadReceipt(ctx, user, reference)
				require.NoError(t, err)
				require.Equal(t, input.Image, image)
				if scenario == "provider failure" {
					return ai.ExtractedTransaction{}, errors.New("provider credential must not leak")
				}
				if scenario == "timeout" {
					<-extractionCtx.Done()
					return ai.ExtractedTransaction{}, extractionCtx.Err()
				}
				return ai.ExtractedTransaction{Amount: 123.45, Merchant: "Cafe", CategoryID: category, Date: "2026-10-06"}, nil
			})
			categories := flowCategories(func(context.Context, uuid.UUID) ([]ai.CategoryOption, error) {
				return []ai.CategoryOption{{ID: category, Name: "Food"}}, nil
			})
			location := time.FixedZone("Asia/Jakarta", 7*60*60)
			timeout := time.Second
			if scenario == "timeout" {
				timeout = 20 * time.Millisecond
			}
			receipt, err := service.NewReceiptService(extractor, categories, transactions, store, nopLogger{}, timeout, location, time.Now)
			require.NoError(t, err)
			response := receiptRequest(t, transactionhttp.NewHandler(transactions, receipt), user)
			var body struct {
				Data struct {
					Status      domain.TransactionStatus `json:"status"`
					ID          uuid.UUID                `json:"transaction_id"`
					Reference   string                   `json:"receipt_image_url"`
					Transaction *domain.Transaction      `json:"transaction"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, pendingID, body.Data.ID)
			require.NotContains(t, response.Body.String(), "credential")
			image, err := store.ReadReceipt(ctx, user, body.Data.Reference)
			require.NoError(t, err)
			require.Equal(t, []byte("\x89PNG\r\n\x1a\n"), image)
			var status string
			var count int
			require.NoError(t, testPool.QueryRow(ctx, "select count(*) from public.transactions where user_id=$1", user).Scan(&count))
			require.Equal(t, 1, count)
			require.NoError(t, testPool.QueryRow(ctx, "select status from public.transactions where id=$1", pendingID).Scan(&status))
			if scenario == "success" {
				require.Equal(t, http.StatusCreated, response.Code)
				require.Equal(t, "success", status)
				require.Equal(t, domain.TransactionStatusSuccess, body.Data.Status)
				require.NotNil(t, body.Data.Transaction)
				require.Equal(t, pendingID, body.Data.Transaction.ID)
				require.Equal(t, 123.45, body.Data.Transaction.Amount)
				require.Eventually(t, func() bool {
					var present bool
					err := testPool.QueryRow(ctx, "select embedding is not null from public.transactions where id=$1", pendingID).Scan(&present)
					return err == nil && present
				}, time.Second, 10*time.Millisecond)
				require.Equal(t, int32(1), embeds.Load())
			} else {
				require.Equal(t, http.StatusAccepted, response.Code)
				require.Equal(t, "pending", status)
				require.Equal(t, domain.TransactionStatusPending, body.Data.Status)
				require.Nil(t, body.Data.Transaction)
				require.Zero(t, embeds.Load())
				rows, err := transactions.ListTransactions(ctx, user, domain.ListTransactionsInput{})
				require.NoError(t, err)
				require.Empty(t, rows)
				got, err := transactions.GetTransaction(ctx, user, pendingID)
				require.ErrorIs(t, err, apperrors.ErrNotFound)
				require.Nil(t, got)
			}
		})
	}
}

type recordingStorage struct {
	*storage.Local
	reference string
	deletes   int
}

func (s *recordingStorage) SaveReceipt(ctx context.Context, user uuid.UUID, mime, name string, image []byte) (string, error) {
	ref, err := s.Local.SaveReceipt(ctx, user, mime, name, image)
	s.reference = ref
	return ref, err
}
func (s *recordingStorage) DeleteReceipt(ctx context.Context, user uuid.UUID, ref string) error {
	s.deletes++
	return s.Local.DeleteReceipt(ctx, user, ref)
}

func TestReceiptFlow_InsertRejectionCleansUploadBeforeAI(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	// Authenticated identity no longer exists in users: the real FK rejects INSERT.
	user := uuid.New()
	local, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	store := &recordingStorage{Local: local}
	transactions := service.NewTransactionServiceWithLogger(newTestRepository(t), nil, nopLogger{}, store)
	extractor := flowExtractor(func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		t.Fatal("AI ran before pending insert succeeded")
		return ai.ExtractedTransaction{}, nil
	})
	categories := flowCategories(func(context.Context, uuid.UUID) ([]ai.CategoryOption, error) { return nil, nil })
	receipt, err := service.NewReceiptService(extractor, categories, transactions, store, nopLogger{}, time.Second, time.FixedZone("Asia/Jakarta", 7*60*60), time.Now)
	require.NoError(t, err)
	response := receiptRequest(t, transactionhttp.NewHandler(transactions, receipt), user)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, 1, store.deletes)
	_, err = local.ReadReceipt(context.Background(), user, store.reference)
	require.True(t, errors.Is(err, os.ErrNotExist))
	var count int
	require.NoError(t, testPool.QueryRow(context.Background(), "select count(*) from public.transactions").Scan(&count))
	require.Zero(t, count)
}

func TestReceiptFlow_StorageFailureStopsBeforeInsert(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	user := seedUser(t)
	directory := t.TempDir()
	store, err := storage.NewLocal(directory)
	require.NoError(t, err)
	// Simulate loss of the storage mount after successful dependency construction.
	require.NoError(t, os.Remove(directory))
	transactions := service.NewTransactionServiceWithLogger(newTestRepository(t), nil, nopLogger{}, store)
	extractor := flowExtractor(func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		t.Fatal("AI ran after storage failure")
		return ai.ExtractedTransaction{}, nil
	})
	categories := flowCategories(func(context.Context, uuid.UUID) ([]ai.CategoryOption, error) { return nil, nil })
	receipt, err := service.NewReceiptService(extractor, categories, transactions, store, nopLogger{}, time.Second, time.FixedZone("Asia/Jakarta", 7*60*60), time.Now)
	require.NoError(t, err)
	response := receiptRequest(t, transactionhttp.NewHandler(transactions, receipt), user)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NotContains(t, response.Body.String(), directory)
	var count int
	require.NoError(t, testPool.QueryRow(context.Background(), "select count(*) from public.transactions").Scan(&count))
	require.Zero(t, count)
}

func TestReceiptFlow_GetAndDeletePendingAndSuccessWithStorageCleanup(t *testing.T) {
	skipIfNoDatabase(t)
	for _, state := range []domain.TransactionStatus{domain.TransactionStatusPending, domain.TransactionStatusSuccess} {
		t.Run(string(state), func(t *testing.T) {
			truncateTables(t)
			ctx := context.Background()
			owner, other := seedUser(t), seedUser(t)
			store, err := storage.NewLocal(t.TempDir())
			require.NoError(t, err)
			reference, err := store.SaveReceipt(ctx, owner, "image/png", "receipt.png", []byte{1})
			require.NoError(t, err)
			repo := newTestRepository(t)
			pending, err := repo.CreatePendingTransaction(ctx, domain.CreatePendingTransactionInput{UserID: owner, ReceiptImageURL: reference})
			require.NoError(t, err)
			if state == domain.TransactionStatusSuccess {
				category := seedCategory(t, owner, "Food", "expense")
				_, err = repo.CompletePendingTransaction(ctx, completionInput(pending, category))
				require.NoError(t, err)
			}
			crud := service.NewTransactionServiceWithLogger(repo, nil, nopLogger{}, store)
			handler := transactionhttp.NewHandler(crud)
			perform := func(user uuid.UUID, method string) *httptest.ResponseRecorder {
				router := gin.New()
				transactionhttp.RegisterProtectedRoutes(router.Group("/api/v1", middleware.AuthMiddleware(flowValidator{user})), handler)
				req := httptest.NewRequest(method, "/api/v1/transactions/"+pending.ID.String(), nil)
				req.Header.Set("Authorization", "Bearer test-token")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w
			}
			require.Equal(t, http.StatusNotFound, perform(other, http.MethodDelete).Code)
			_, err = store.ReadReceipt(ctx, owner, reference)
			require.NoError(t, err, "foreign deletion must retain the image")
			w := perform(owner, http.MethodGet)
			if state == domain.TransactionStatusPending {
				require.Equal(t, http.StatusNotFound, w.Code)
			} else {
				require.Equal(t, http.StatusOK, w.Code)
				var result struct {
					Data map[string]interface{} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				require.Equal(t, string(state), result.Data["status"])
				require.Equal(t, reference, result.Data["receipt_image_url"])
			}
			require.Equal(t, http.StatusNoContent, perform(owner, http.MethodDelete).Code)
			require.Equal(t, http.StatusNotFound, perform(owner, http.MethodGet).Code)
			_, err = store.ReadReceipt(ctx, owner, reference)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
