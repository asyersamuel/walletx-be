package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/configs"
	"walletx-be/internal/middleware"
	aihttp "walletx-be/internal/modules/ai/delivery/http"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/categories"
	"walletx-be/internal/modules/transactions"
	domain "walletx-be/internal/modules/transactions/contracts"
	transactionhttp "walletx-be/internal/modules/transactions/delivery/http"
	"walletx-be/internal/modules/users"
	"walletx-be/internal/platform/storage"
)

const storageTestURL = "/api/v1/dev/storage/receipts"

func storageDebugRouter(t *testing.T, store domain.ReceiptStorage, devMode bool, owner uuid.UUID) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := routerLogger{}
	modules := &Modules{
		Auth: &auth.Module{Handler: &auth.Handler{}}, Users: &users.Module{Handler: &users.Handler{}},
		Categories: &categories.Module{Handler: &categories.Handler{}}, Transactions: &transactions.Module{Handler: &transactionhttp.Handler{}},
		AI: aihttp.NewHandler(routerAIService{id: owner}, log), ReceiptStorage: store,
	}
	const secret = "storage-test-signing-key"
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": owner.String(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	require.NoError(t, err)
	router := SetupRouter(modules, &configs.Config{App: configs.AppConfig{DevMode: devMode}}, middleware.NewJWTValidator(secret, auth.NewInMemoryBlacklistRepository()), log)
	return router, token
}

func storageUploadRequest(t *testing.T, image []byte, field, token string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(field, "receipt.png")
	require.NoError(t, err)
	_, err = part.Write(image)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, storageTestURL, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func storageDeleteRequest(body, token string) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, storageTestURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestStorageDebugRoutesDevModeAndAuthentication(t *testing.T) {
	owner := uuid.New()
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	for _, devMode := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			router, token := storageDebugRouter(t, store, devMode, owner)
			if !authorized {
				token = ""
			}
			for _, req := range []*http.Request{
				storageUploadRequest(t, []byte("\x89PNG\r\n\x1a\n"), "receipt", token),
				storageDeleteRequest(`{}`, token),
			} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				switch {
				case !devMode:
					require.Equal(t, http.StatusNotFound, w.Code)
				case !authorized:
					require.Equal(t, http.StatusUnauthorized, w.Code)
				case req.Method == http.MethodPost:
					require.Equal(t, http.StatusCreated, w.Code)
				default:
					require.Equal(t, http.StatusBadRequest, w.Code)
				}
			}
		}
	}
}

func TestStorageDebugRoutesSupabaseUploadAndDelete(t *testing.T) {
	type providerRequest struct {
		method, path string
		body         []byte
	}
	requests := make(chan providerRequest, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls.Add(1)
		requests <- providerRequest{r.Method, r.URL.Path, body}
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)
	store, err := buildReceiptStorage(configs.ReceiptStorageConfig{
		Provider: "supabase", SupabaseURL: server.URL, SupabaseServiceRoleKey: "dummy-key", UploadTimeout: time.Second,
	})
	require.NoError(t, err)
	owner := uuid.New()
	router, token := storageDebugRouter(t, store, true, owner)
	image := []byte("\x89PNG\r\n\x1a\n")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, storageUploadRequest(t, image, "receipt", token))
	require.Equal(t, http.StatusCreated, w.Code)
	var result struct {
		Data struct {
			Path string `json:"path"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	ref := result.Data.Path
	require.True(t, strings.HasPrefix(ref, owner.String()+"/"))
	got := <-requests
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/storage/v1/object/receipts/"+ref, got.path)
	require.Equal(t, image, got.body)

	body, err := json.Marshal(map[string]string{"path": ref})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, storageDeleteRequest(string(body), token))
	require.Equal(t, http.StatusOK, w.Code)
	got = <-requests
	require.Equal(t, http.MethodDelete, got.method)
	require.Equal(t, "/storage/v1/object/receipts", got.path)
	require.JSONEq(t, `{"prefixes":["`+ref+`"]}`, string(got.body))

	for _, bad := range []string{
		`{"path":"` + uuid.NewString() + `/` + uuid.NewString() + `_123.png"}`,
		`{"path":"../../receipt.png"}`,
		`{"path":"https://example.com/receipt.png"}`,
		`{"path":"` + ref + `","user_id":"` + uuid.NewString() + `"}`,
		`{"path":"` + ref + `"} {}`,
		`{`, `{}`, `null`,
	} {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, storageDeleteRequest(bad, token))
		require.Equal(t, http.StatusBadRequest, w.Code, bad)
	}
	require.EqualValues(t, 2, calls.Load(), "invalid deletion inputs must not reach the provider")
}

func TestStorageDebugUploadRejectsInvalidFiles(t *testing.T) {
	store := &storageDebugStub{}
	router, token := storageDebugRouter(t, store, true, uuid.New())
	for _, tc := range []struct {
		name, field string
		image       []byte
		want        int
	}{
		{"missing field", "wrong_field", []byte("\x89PNG\r\n\x1a\n"), http.StatusBadRequest},
		{"empty image", "receipt", nil, http.StatusBadRequest},
		{"spoofed PNG", "receipt", []byte("plain text"), http.StatusUnsupportedMediaType},
		{"oversized image", "receipt", make([]byte, domain.MaxReceiptBytes+1), http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, storageUploadRequest(t, tc.image, tc.field, token))
			require.Equal(t, tc.want, w.Code)
		})
	}
	// Bound even a request whose Content-Length is unknown.
	req := storageUploadRequest(t, make([]byte, domain.MaxReceiptBytes+(2<<20)), "receipt", token)
	req.ContentLength = -1
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.Zero(t, store.calls)
}

type storageDebugStub struct {
	err   error
	calls int
}

func (s *storageDebugStub) SaveReceipt(context.Context, uuid.UUID, string, string, []byte) (string, error) {
	s.calls++
	return "", s.err
}

func (s *storageDebugStub) DeleteReceipt(context.Context, uuid.UUID, string) error {
	s.calls++
	return s.err
}

func TestStorageDebugProviderErrorsAreGeneric(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{errors.New("sensitive provider token"), http.StatusInternalServerError},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{context.Canceled, http.StatusRequestTimeout},
	} {
		store := &storageDebugStub{err: tc.err}
		router, token := storageDebugRouter(t, store, true, uuid.New())
		for _, req := range []*http.Request{
			storageUploadRequest(t, []byte("\x89PNG\r\n\x1a\n"), "receipt", token),
			storageDeleteRequest(`{"path":"test"}`, token),
		} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code)
			require.NotContains(t, w.Body.String(), "sensitive")
		}
	}
}
