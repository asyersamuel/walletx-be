package app

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/ai"
	aihttp "walletx-be/internal/modules/ai/delivery/http"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/categories"
	"walletx-be/internal/modules/transactions"
	transactionhttp "walletx-be/internal/modules/transactions/delivery/http"
	"walletx-be/internal/modules/users"
	"walletx-be/internal/platform/logger"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type routerLogger struct{}

func (routerLogger) Info(string)                                       {}
func (routerLogger) Warn(string)                                       {}
func (routerLogger) Error(string)                                      {}
func (routerLogger) Debug(string)                                      {}
func (l routerLogger) WithField(string, interface{}) logger.Logger     { return l }
func (l routerLogger) WithError(error) logger.Logger                   { return l }
func (l routerLogger) WithFields(map[string]interface{}) logger.Logger { return l }

type routerAIService struct{ id uuid.UUID }

func (s routerAIService) Chat(_ context.Context, r ai.ChatRequest) (ai.ChatStream, error) {
	if r.UserID != s.id {
		return nil, middleware.ErrInvalidClaims
	}
	ch := make(chan ai.ChatEvent, 1)
	ch <- ai.ChatEvent{Kind: ai.ChatEventComplete, Message: &ai.ChatMessage{Role: ai.ChatRoleAssistant, Parts: []ai.ChatPart{{Text: "hello"}}}}
	close(ch)
	return ch, nil
}
func (s routerAIService) ExtractTransaction(_ context.Context, r ai.ExtractionInput) (ai.ExtractedTransaction, error) {
	if r.UserID != s.id {
		return ai.ExtractedTransaction{}, middleware.ErrInvalidClaims
	}
	return ai.ExtractedTransaction{Amount: 10, Merchant: "Coffee", CategoryID: uuid.New(), Date: "2026-10-03"}, nil
}

func TestAIRoutesUseJWTMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := uuid.New()
	secret := "test-signing-key"
	log := routerLogger{}
	modules := &Modules{
		Auth: &auth.Module{Handler: &auth.Handler{}}, Users: &users.Module{Handler: &users.Handler{}},
		Categories: &categories.Module{Handler: &categories.Handler{}}, Transactions: &transactions.Module{Handler: &transactionhttp.Handler{}},
		AI: aihttp.NewHandler(routerAIService{id: id}, log),
	}
	r := SetupRouter(modules, &configs.Config{}, middleware.NewJWTValidator(secret, auth.NewInMemoryBlacklistRepository()), log)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": id.String(), "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(secret))
	require.NoError(t, err)
	for _, path := range []string{"/api/v1/ai/chat", "/api/v1/ai/extract"} {
		for _, authorized := range []bool{false, true} {
			body := `{"prompt":"hello"}`
			if strings.HasSuffix(path, "extract") {
				body = `{"text":"coffee"}`
			}
			req := httptest.NewRequest("POST", path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if authorized {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if !authorized {
				require.Equal(t, 401, w.Code)
				continue
			}
			require.Equal(t, 200, w.Code)
			if strings.HasSuffix(path, "chat") {
				require.Contains(t, w.Body.String(), "event: complete")
			} else {
				require.Contains(t, w.Body.String(), `"merchant":"Coffee"`)
			}
		}
	}
}
