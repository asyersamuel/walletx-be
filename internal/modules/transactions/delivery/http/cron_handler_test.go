package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
)

type pendingProcessorFunc func(context.Context) (domain.PendingReceiptSummary, error)

func (f pendingProcessorFunc) ProcessPendingReceipts(ctx context.Context) (domain.PendingReceiptSummary, error) {
	return f(ctx)
}

func TestCronHandlerResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		summary     domain.PendingReceiptSummary
		err         error
		unavailable bool
		status      int
	}{
		{name: "success", summary: domain.PendingReceiptSummary{Fetched: 2, Succeeded: 2}, status: 200},
		{name: "individual failures", summary: domain.PendingReceiptSummary{Fetched: 2, Failed: 2}, status: 200},
		{name: "budget exhausted", summary: domain.PendingReceiptSummary{Fetched: 3, Succeeded: 1, Deferred: 2, StoppedEarly: true}, status: 200},
		{name: "database unavailable", err: errors.New("secret connection details"), status: 500},
		{name: "unavailable processor", unavailable: true, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var processor domain.PendingReceiptProcessor
			key := struct{}{}
			if !tc.unavailable {
				processor = pendingProcessorFunc(func(ctx context.Context) (domain.PendingReceiptSummary, error) {
					require.Equal(t, "request", ctx.Value(key))
					return tc.summary, tc.err
				})
			}
			router := gin.New()
			RegisterCronRoutes(router.Group("/api/v1/cron"), NewCronHandler(processor))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/cron/process-pending-receipts", nil)
			req = req.WithContext(context.WithValue(req.Context(), key, "request"))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.NotContains(t, w.Body.String(), "secret")
			if tc.status == 200 {
				var body struct {
					Data domain.PendingReceiptSummary `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Equal(t, tc.summary, body.Data)
			}
		})
	}
}
