package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/configs"
	"walletx-be/internal/platform/storage"
	apperrors "walletx-be/internal/shared/errors"
)

func TestBuildReceiptStorage(t *testing.T) {
	root := t.TempDir()
	store, err := buildReceiptStorage(configs.ReceiptStorageConfig{Provider: "local", Directory: root})
	require.NoError(t, err)
	require.IsType(t, &storage.Local{}, store)
	ref, err := store.SaveReceipt(context.Background(), uuid.New(), "image/png", "receipt.png", []byte{1})
	require.NoError(t, err)
	require.NotEmpty(t, ref)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	_, err = buildReceiptStorage(configs.ReceiptStorageConfig{Provider: "unknown", Directory: root})
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	_, err = buildReceiptStorage(configs.ReceiptStorageConfig{Provider: "local", Directory: ""})
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
}

func TestBuildSupabaseReceiptStorage(t *testing.T) {
	type request struct {
		method, path, authorization, apiKey string
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		requests <- request{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("apikey")}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	store, err := buildReceiptStorage(configs.ReceiptStorageConfig{
		Provider: "supabase", SupabaseURL: server.URL, SupabaseServiceRoleKey: "dummy-key", UploadTimeout: time.Second,
	})
	require.NoError(t, err)
	require.IsType(t, &storage.Supabase{}, store)
	ref, err := store.SaveReceipt(context.Background(), uuid.New(), "image/png", "receipt.png", []byte{1})
	require.NoError(t, err)
	got := <-requests
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/storage/v1/object/receipts/"+ref, got.path)
	require.Equal(t, "Bearer dummy-key", got.authorization)
	require.Equal(t, "dummy-key", got.apiKey)
}

func TestBuildSupabaseReceiptStorageInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		name, url, key string
		timeout        time.Duration
	}{
		{"missing URL", "", "dummy-key", time.Second},
		{"missing key", "https://project.supabase.co", "", time.Second},
		{"malformed URL", "://invalid", "dummy-key", time.Second},
		{"unsafe URL", "https://secret@project.supabase.co", "dummy-key", time.Second},
		{"malformed key", "https://project.supabase.co", "dummy-key\r\n", time.Second},
		{"zero timeout", "https://project.supabase.co", "dummy-key", 0},
		{"negative timeout", "https://project.supabase.co", "dummy-key", -time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := buildReceiptStorage(configs.ReceiptStorageConfig{
				Provider: "supabase", Directory: t.TempDir(), SupabaseURL: tc.url, SupabaseServiceRoleKey: tc.key, UploadTimeout: tc.timeout,
			})
			require.Nil(t, store, "invalid Supabase configuration must not fall back to local")
			require.ErrorIs(t, err, apperrors.ErrInvalidInput)
			require.NotContains(t, err.Error(), "dummy-key")
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestBuildSupabaseReceiptStorageUsesConfiguredTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	store, err := buildReceiptStorage(configs.ReceiptStorageConfig{
		Provider: "supabase", SupabaseURL: server.URL, SupabaseServiceRoleKey: "dummy-key", UploadTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ref, err := store.SaveReceipt(ctx, uuid.New(), "image/png", "receipt.png", []byte{1})
	require.Empty(t, ref)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, ctx.Err(), "HTTP client timeout should expire before the caller deadline")
}
