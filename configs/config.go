package configs

import (
	"os"
	"strconv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	App      AppConfig
	Media    MediaConfig
	IMAP     IMAPConfig
	Gemini   GeminiConfig
	OAuth    OAuthConfig
	Cron     CronConfig
	Telegram TelegramConfig
	SMTP     SMTPConfig
}

type IMAPConfig struct {
	Email    string
	Password string
	Server   string
}

type GeminiConfig struct {
	APIKey string
}

type CronConfig struct {
	Secret string
}

type TelegramConfig struct {
	BotToken string
}

type SMTPConfig struct {
	Host     string
	Port     string
	User     string
	Password string
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type RedisConfig struct {
	URL string
}

type ServerConfig struct {
	Port string
}

type DatabaseConfig struct {
	// Use one URL for both local Supabase and hosted Supabase Postgres.
	// Migrations are managed separately by the Supabase CLI.
	URL string
}

type JWTConfig struct {
	Secret     string
	Expiration int // in hours
}

type AppConfig struct {
	DevMode bool   // Development mode flag
	URL     string // Frontend or API Base URL, e.g. https://walletx-be.vercel.app
}

type MediaConfig struct {
	StorageType string // "local" or another storage adapter
	UploadDir   string // Base directory for local uploads
	BaseURL     string // Base URL for serving local files
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", "8080"),
		},
		Database: DatabaseConfig{
			URL: getEnv("DATABASE_URL", ""),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", ""),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", ""),
			Expiration: getEnvAsInt("JWT_EXPIRATION", 24),
		},
		App: AppConfig{
			DevMode: getEnvAsBool("DEV_MODE", false),
			URL:     getEnv("APP_URL", "http://localhost:8080"),
		},
		Media: MediaConfig{
			StorageType: getEnv("STORAGE_TYPE", "local"),
			UploadDir:   getEnv("UPLOAD_DIR", "uploads"),
			BaseURL:     getEnv("MEDIA_BASE_URL", "/uploads"),
		},
		IMAP: IMAPConfig{
			Email:    getEnv("IMAP_EMAIL", ""),
			Password: getEnv("IMAP_PASSWORD", ""),
			Server:   getEnv("IMAP_SERVER", "imap.gmail.com:993"),
		},
		Gemini: GeminiConfig{
			APIKey: getEnv("GEMINI_API_KEY", ""),
		},
		OAuth: OAuthConfig{
			ClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			RedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
		},
		Cron: CronConfig{
			Secret: getEnv("CRON_SECRET", ""),
		},
		Telegram: TelegramConfig{
			BotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		},
		SMTP: SMTPConfig{
			Host:     getEnv("SMTP_HOST", "smtp.gmail.com"),
			Port:     getEnv("SMTP_PORT", "587"),
			User:     getEnv("SMTP_USER", ""),
			Password: getEnv("SMTP_PASS", ""),
		},
	}
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
