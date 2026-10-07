package configs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	apperrors "walletx-be/internal/shared/errors"
)

const DefaultChatModel = "gemini-3.5-flash"
const DefaultReceiptExtractionTimeout = 20 * time.Second
const DefaultReceiptStorageDirectory = ".data/receipts"
const DefaultReceiptStorageTimeout = 30 * time.Second

type Config struct {
	Server         ServerConfig
	Database       DatabaseConfig
	JWT            JWTConfig
	App            AppConfig
	OAuth          OAuthConfig
	AI             AIConfig
	ReceiptStorage ReceiptStorageConfig
	Receipt        ReceiptConfig
}

type ReceiptStorageConfig struct {
	Provider               string
	Directory              string
	SupabaseURL            string
	SupabaseServiceRoleKey string
	UploadTimeout          time.Duration
}

// Validate checks provider-specific settings before dependency construction.
// Load preserves invalid values so startup fails rather than silently using a
// fallback provider or timeout. Errors never include configuration values.
func (c ReceiptStorageConfig) Validate() error {
	switch c.Provider {
	case "local":
		return nil
	case "supabase":
		if strings.TrimSpace(c.SupabaseURL) == "" {
			return fmt.Errorf("SUPABASE_URL is required for Supabase receipt storage: %w", apperrors.ErrInvalidInput)
		}
		if strings.TrimSpace(c.SupabaseServiceRoleKey) == "" {
			return fmt.Errorf("SUPABASE_SERVICE_ROLE_KEY is required for Supabase receipt storage: %w", apperrors.ErrInvalidInput)
		}
		if c.UploadTimeout <= 0 {
			return fmt.Errorf("RECEIPT_STORAGE_TIMEOUT must be a positive duration for Supabase receipt storage: %w", apperrors.ErrInvalidInput)
		}
		return nil
	default:
		return fmt.Errorf("unsupported receipt storage provider: %w", apperrors.ErrInvalidInput)
	}
}

type ReceiptConfig struct {
	ExtractionTimeout time.Duration
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type ServerConfig struct {
	Port string
}

type DatabaseConfig struct {
	URL string
}

type JWTConfig struct {
	Secret     string
	Expiration int
}

type AppConfig struct {
	DevMode bool
}

type AIConfig struct {
	GeminiAPIKey string
	ChatModel    string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", "8080"),
		},
		Database: DatabaseConfig{
			URL: getEnv("DATABASE_URL", ""),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", ""),
			Expiration: getEnvAsInt("JWT_EXPIRATION", 24),
		},
		App: AppConfig{
			DevMode: getEnvAsBool("DEV_MODE", false),
		},
		OAuth: OAuthConfig{
			ClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			RedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
		},
		AI: AIConfig{
			GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
			ChatModel:    getEnv("GEMINI_CHAT_MODEL", DefaultChatModel),
		},
		ReceiptStorage: ReceiptStorageConfig{
			Provider:               getEnv("RECEIPT_STORAGE_PROVIDER", "local"),
			Directory:              getEnv("RECEIPT_STORAGE_DIR", DefaultReceiptStorageDirectory),
			SupabaseURL:            getEnv("SUPABASE_URL", ""),
			SupabaseServiceRoleKey: getEnv("SUPABASE_SERVICE_ROLE_KEY", ""),
			UploadTimeout:          getEnvAsDuration("RECEIPT_STORAGE_TIMEOUT", DefaultReceiptStorageTimeout),
		},
		Receipt: ReceiptConfig{
			ExtractionTimeout: getEnvAsDuration("RECEIPT_AI_TIMEOUT", DefaultReceiptExtractionTimeout),
		},
	}
}

// Explicitly invalid values become zero so dependency construction rejects them
// rather than silently discarding a configured timeout.
func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return duration
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
