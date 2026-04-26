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
	OAuth    OAuthConfig
}

type IMAPConfig struct {
    Email    string
    Password string
    Server   string
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
	Port     string
	Host     string
	CertFile string
	KeyFile  string
}

type DatabaseConfig struct {
    // Preferred: full connection URL, e.g., postgres://user:pass@host:5432/db?sslmode=require
    URL      string
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type JWTConfig struct {
	Secret     string
	Expiration int // in hours
}

type AppConfig struct {
	DevMode bool // Development mode flag
}

type MediaConfig struct {
	StorageType        string // "local" or "supabase"
	UploadDir          string // Base directory for uploads (local storage only)
	MaxImageSize       int64  // Maximum image size in bytes
	MaxVideoSize       int64  // Maximum video size in bytes
	BaseURL            string // Base URL for serving files (local storage only)
	
	// Supabase Storage configuration
	SupabaseStorageURL string // Supabase Storage API URL
	SupabaseStorageKey string // Supabase anon/public key
	SupabaseBucket     string // Storage bucket name
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:     getEnv("PORT", "8443"),
			Host:     getEnv("HOST", "localhost"),
			CertFile: getEnv("CERT_FILE", "certs/cert.pem"),
			KeyFile:  getEnv("KEY_FILE", "certs/key.pem"),
		},
		Database: DatabaseConfig{
            // Preferred: full connection URL, e.g., postgres://user:pass@host:5432/db?sslmode=require
            URL:      firstNonEmpty(os.Getenv("DATABASE_URL"), os.Getenv("DB_URL"), os.Getenv("SUPABASE_DB_URL")),
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "password"),
			DBName:   getEnv("DB_NAME", "backend_service"),
            SSLMode:  getEnv("DB_SSLMODE", "require"),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", ""),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", "your-secret-key"),
			Expiration: getEnvAsInt("JWT_EXPIRATION", 24),
		},
		App: AppConfig{
			DevMode: getEnvAsBool("DEV_MODE", false),
		},
		Media: MediaConfig{
			StorageType:        getEnv("STORAGE_TYPE", "local"), // Default to local storage
			UploadDir:          getEnv("UPLOAD_DIR", "uploads"),
			MaxImageSize:       int64(getEnvAsInt("MAX_IMAGE_SIZE_MB", 5)) * 1024 * 1024,  // 5MB - best practice for small-scale apps
			MaxVideoSize:       int64(getEnvAsInt("MAX_VIDEO_SIZE_MB", 50)) * 1024 * 1024, // 50MB - suitable for short clips
			BaseURL:            getEnv("MEDIA_BASE_URL", "/uploads"),
			SupabaseStorageURL: getEnv("SUPABASE_STORAGE_URL", ""),
			SupabaseStorageKey: getEnv("SUPABASE_STORAGE_KEY", ""),
			SupabaseBucket:     getEnv("SUPABASE_STORAGE_BUCKET", "media"),
		},
		IMAP: IMAPConfig{
            Email:    getEnv("IMAP_EMAIL", "walletxforyourfuture@gmail.com"),
            Password: getEnv("IMAP_PASSWORD", ""), 
            Server:   getEnv("IMAP_SERVER", "imap.gmail.com:993"),
        },
		OAuth: OAuthConfig{
			ClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			RedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
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

// firstNonEmpty returns the first non-empty string from the provided list
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
