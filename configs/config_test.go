package configs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apperrors "walletx-be/internal/shared/errors"
)

func TestReceiptConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("RECEIPT_STORAGE_PROVIDER", "")
	t.Setenv("RECEIPT_STORAGE_DIR", "")
	t.Setenv("RECEIPT_AI_TIMEOUT", "")
	cfg := Load()
	require.Equal(t, "local", cfg.ReceiptStorage.Provider)
	require.Equal(t, DefaultReceiptStorageDirectory, cfg.ReceiptStorage.Directory)
	require.Equal(t, DefaultReceiptExtractionTimeout, cfg.Receipt.ExtractionTimeout)
	t.Setenv("RECEIPT_STORAGE_DIR", t.TempDir())
	t.Setenv("RECEIPT_AI_TIMEOUT", "750ms")
	cfg = Load()
	require.NotEqual(t, DefaultReceiptStorageDirectory, cfg.ReceiptStorage.Directory)
	require.Equal(t, 750*time.Millisecond, cfg.Receipt.ExtractionTimeout)
	t.Setenv("RECEIPT_AI_TIMEOUT", "invalid")
	require.Zero(t, Load().Receipt.ExtractionTimeout)
	t.Setenv("RECEIPT_AI_TIMEOUT", "-1s")
	require.Negative(t, Load().Receipt.ExtractionTimeout)
}

func TestReceiptStorageConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("RECEIPT_STORAGE_PROVIDER", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "")
	t.Setenv("RECEIPT_STORAGE_TIMEOUT", "")
	cfg := Load().ReceiptStorage
	require.Equal(t, "local", cfg.Provider)
	require.Empty(t, cfg.SupabaseURL)
	require.Empty(t, cfg.SupabaseServiceRoleKey)
	require.Equal(t, 30*time.Second, cfg.UploadTimeout)
	require.NoError(t, cfg.Validate())

	t.Setenv("RECEIPT_STORAGE_PROVIDER", "supabase")
	t.Setenv("SUPABASE_URL", "https://project.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "dummy-service-role-key")
	t.Setenv("RECEIPT_STORAGE_TIMEOUT", "750ms")
	cfg = Load().ReceiptStorage
	require.Equal(t, "supabase", cfg.Provider)
	require.Equal(t, "https://project.supabase.co", cfg.SupabaseURL)
	require.Equal(t, "dummy-service-role-key", cfg.SupabaseServiceRoleKey)
	require.Equal(t, 750*time.Millisecond, cfg.UploadTimeout)
	require.NoError(t, cfg.Validate())
}

func TestReceiptStorageConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name, provider, url, key, timeout string
		valid                             bool
	}{
		{"local needs no Supabase settings", "local", "", "", "invalid", true},
		{"Supabase default timeout", "supabase", "https://project.supabase.co", "dummy-key", "", true},
		{"missing URL", "supabase", "", "dummy-key", "30s", false},
		{"blank URL", "supabase", " \t", "dummy-key", "30s", false},
		{"missing key", "supabase", "https://project.supabase.co", "", "30s", false},
		{"blank key", "supabase", "https://project.supabase.co", " \t", "30s", false},
		{"invalid timeout", "supabase", "https://project.supabase.co", "dummy-key", "invalid", false},
		{"zero timeout", "supabase", "https://project.supabase.co", "dummy-key", "0s", false},
		{"negative timeout", "supabase", "https://project.supabase.co", "dummy-key", "-1s", false},
		{"unknown provider", "unknown", "", "", "30s", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RECEIPT_STORAGE_PROVIDER", tc.provider)
			t.Setenv("SUPABASE_URL", tc.url)
			t.Setenv("SUPABASE_SERVICE_ROLE_KEY", tc.key)
			t.Setenv("RECEIPT_STORAGE_TIMEOUT", tc.timeout)
			err := Load().ReceiptStorage.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, apperrors.ErrInvalidInput)
				require.NotContains(t, err.Error(), "dummy-key")
				require.NotContains(t, err.Error(), "https://project.supabase.co")
			}
		})
	}
}
