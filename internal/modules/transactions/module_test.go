package transactions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/ai"
	aipostgres "walletx-be/internal/modules/ai/repository/postgres"
	transactionhttp "walletx-be/internal/modules/transactions/delivery/http"
	transactionservice "walletx-be/internal/modules/transactions/service"
	db "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/gemini"
	"walletx-be/internal/platform/logger"
	"walletx-be/internal/platform/storage"
)

type moduleLogger struct{ debugCalls int }

func (*moduleLogger) Info(string)                                       {}
func (*moduleLogger) Warn(string)                                       {}
func (*moduleLogger) Error(string)                                      {}
func (l *moduleLogger) Debug(string)                                    { l.debugCalls++ }
func (l *moduleLogger) WithField(string, interface{}) logger.Logger     { return l }
func (l *moduleLogger) WithError(error) logger.Logger                   { return l }
func (l *moduleLogger) WithFields(map[string]interface{}) logger.Logger { return l }

type moduleTokenValidator struct{ owner uuid.UUID }

func (v moduleTokenValidator) ValidateToken(context.Context, string) (*middleware.TokenClaims, error) {
	return &middleware.TokenClaims{UserID: v.owner.String()}, nil
}

// Exercise the real module wiring and SDK against a local Gemini HTTP server.
// Category lookup is required; transaction mutation SQL is not used for plain text.
func TestModuleChatReachesGemini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "provider failure"
		}
		t.Run(name, func(t *testing.T) {
			owner := uuid.New()
			categories := &moduleCategoryReader{owner: owner}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "/v1beta/models/chat-test:generateContent", r.URL.Path)
				var body struct {
					Contents []genai.Content `json:"contents"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.NotEmpty(t, body.Contents)
				prompt := body.Contents[len(body.Contents)-1].Parts[0].Text
				contextAndMessage := strings.SplitN(strings.SplitN(prompt, "WalletX server context (JSON):\n", 2)[1], "\nUser message (JSON):\n", 2)
				var serverContext struct {
					CurrentTime string   `json:"current_time"`
					TimeZone    string   `json:"time_zone"`
					Categories  []string `json:"available_categories"`
				}
				require.NoError(t, json.Unmarshal([]byte(contextAndMessage[0]), &serverContext))
				require.Equal(t, "Asia/Jakarta", serverContext.TimeZone)
				require.Equal(t, []string{"Coffee"}, serverContext.Categories)
				current, err := time.Parse(time.RFC3339Nano, serverContext.CurrentTime)
				require.NoError(t, err)
				_, offset := current.Zone()
				require.Equal(t, 7*60*60, offset)
				require.WithinDuration(t, time.Now(), current, time.Minute)
				var message string
				require.NoError(t, json.Unmarshal([]byte(contextAndMessage[1]), &message))
				require.Equal(t, "hello", message)
				if fail {
					http.Error(w, "test provider failed", http.StatusInternalServerError)
					return
				}
				data, err := json.Marshal(genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
					FinishReason: genai.FinishReasonStop,
					Content:      &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "Hello from Gemini"}}},
				}}})
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(data)
				require.NoError(t, err)
			}))
			defer server.Close()
			client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
				APIKey: "test-key", Backend: genai.BackendGeminiAPI,
				HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
			})
			require.NoError(t, err)
			appLogger := &moduleLogger{}
			queries := db.New(nil)
			provider := gemini.NewProvider(client, "chat-test", "embedding-test")
			aiRepository := aipostgres.NewRepository(queries)
			receiptStorage, err := storage.NewLocal(t.TempDir())
			require.NoError(t, err)
			module, err := NewModule(queries, appLogger, nil, gemini.NewTransactionLLM(client, "chat-test"), ai.NewAIService(provider, aiRepository, aiRepository), categories, receiptStorage, time.Second)
			require.NoError(t, err)
			router := gin.New()
			protected := router.Group("/api/v1", middleware.AuthMiddleware(moduleTokenValidator{owner: owner}))
			transactionhttp.RegisterProtectedRoutes(protected, module.Handler)
			// A missing upload must reach validation, rather than the nil-service 503 guard.
			receiptRequest := httptest.NewRequest("POST", "/api/v1/transactions/receipt", nil)
			receiptRequest.Header.Set("Authorization", "Bearer test-token")
			receiptResponse := httptest.NewRecorder()
			router.ServeHTTP(receiptResponse, receiptRequest)
			require.Equal(t, http.StatusBadRequest, receiptResponse.Code)
			require.NotContains(t, receiptResponse.Body.String(), "Receipt service unavailable")
			req := httptest.NewRequest("POST", "/api/v1/transactions/chat", strings.NewReader(`{"message":"hello"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, int32(1), calls.Load())
			require.Equal(t, []uuid.UUID{owner}, categories.users)
			require.Equal(t, 1, appLogger.debugCalls)
			require.NotContains(t, response.Body.String(), "AI service unavailable")
			if fail {
				require.Equal(t, http.StatusInternalServerError, response.Code)
				require.NotContains(t, response.Body.String(), "test provider failed")
			} else {
				require.Equal(t, http.StatusOK, response.Code)
				require.Contains(t, response.Body.String(), transactionservice.FallbackText)
			}
		})
	}
}

type moduleCategoryReader struct {
	owner uuid.UUID
	users []uuid.UUID
}

func (r *moduleCategoryReader) ListByUser(ctx context.Context, userID uuid.UUID) ([]ai.CategoryOption, error) {
	r.users = append(r.users, userID)
	if userID != r.owner {
		return nil, nil
	}
	return []ai.CategoryOption{{ID: uuid.New(), Name: "Coffee", Type: "expense"}}, nil
}
