package utils

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	config "walletx-be/configs"
)

// MediaConfig holds configuration for media uploads
// This is a wrapper around config.MediaConfig with utility methods
type MediaConfig struct {
	config        config.MediaConfig
	AllowedImages []string
	AllowedVideos []string
}

// NewMediaConfig creates a MediaConfig from config.MediaConfig
func NewMediaConfig(cfg config.MediaConfig) MediaConfig {
	return MediaConfig{
		config:        cfg,
		AllowedImages: []string{".jpg", ".jpeg", ".png"},
		AllowedVideos: []string{".mp4"},
	}
}

// Helper methods to access config fields
func (m MediaConfig) StorageType() string        { return m.config.StorageType }
func (m MediaConfig) UploadDir() string          { return m.config.UploadDir }
func (m MediaConfig) MaxImageSize() int64        { return m.config.MaxImageSize }
func (m MediaConfig) MaxVideoSize() int64        { return m.config.MaxVideoSize }
func (m MediaConfig) BaseURL() string            { return m.config.BaseURL }
func (m MediaConfig) SupabaseStorageURL() string { return m.config.SupabaseStorageURL }
func (m MediaConfig) SupabaseStorageKey() string { return m.config.SupabaseStorageKey }
func (m MediaConfig) SupabaseBucket() string     { return m.config.SupabaseBucket }

// MediaType represents the type of media
type MediaType string

const (
	MediaTypeImage   MediaType = "image"
	MediaTypeVideo   MediaType = "video"
	MediaTypeUnknown MediaType = "unknown"
)

// MediaInfo contains information about uploaded media
type MediaInfo struct {
	Filename    string    // Original filename
	StoredPath  string    // Path where file is stored on disk
	PublicURL   string    // Public URL to access the file
	MediaType   MediaType // Type of media (image/video)
	Size        int64     // File size in bytes
	ContentType string    // MIME type
}

// ValidateFile validates if a file meets the requirements
func ValidateFile(file *multipart.FileHeader, mediaType MediaType, cfg MediaConfig) error {
	// Check file size
	if mediaType == MediaTypeImage && file.Size > cfg.MaxImageSize() {
		return fmt.Errorf("image file too large: maximum size is %d bytes", cfg.MaxImageSize())
	}
	if mediaType == MediaTypeVideo && file.Size > cfg.MaxVideoSize() {
		return fmt.Errorf("video file too large: maximum size is %d bytes", cfg.MaxVideoSize())
	}

	// Get file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))

	// Check if extension is allowed
	if mediaType == MediaTypeImage {
		allowed := false
		for _, allowedExt := range cfg.AllowedImages {
			if ext == allowedExt {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("invalid image format. Allowed: %v", cfg.AllowedImages)
		}
	} else if mediaType == MediaTypeVideo {
		allowed := false
		for _, allowedExt := range cfg.AllowedVideos {
			if ext == allowedExt {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("invalid video format. Allowed: %v", cfg.AllowedVideos)
		}
	}

	return nil
}

// DetectMediaType detects if a file is an image or video based on extension
func DetectMediaType(filename string, cfg MediaConfig) MediaType {
	ext := strings.ToLower(filepath.Ext(filename))

	for _, imgExt := range cfg.AllowedImages {
		if ext == imgExt {
			return MediaTypeImage
		}
	}

	for _, vidExt := range cfg.AllowedVideos {
		if ext == vidExt {
			return MediaTypeVideo
		}
	}

	return MediaTypeUnknown
}

// SaveUploadedFile saves an uploaded file using the configured storage backend
func SaveUploadedFile(file *multipart.FileHeader, subDir string, cfg MediaConfig) (*MediaInfo, error) {
	// Create storage backend based on configuration
	storage := NewStorage(cfg)

	// Use storage backend to save file
	return storage.SaveFile(file, subDir)
}

// DeleteFile deletes a file using the configured storage backend
func DeleteFile(filePath string, cfg MediaConfig) error {
	// Create storage backend based on configuration
	storage := NewStorage(cfg)

	// Use storage backend to delete file
	return storage.DeleteFile(filePath)
}

// GetFileFromURL extracts the file path from a public URL
func GetFileFromURL(publicURL string, cfg MediaConfig) string {
	// If Supabase storage, extract path from URL
	if cfg.StorageType() == "supabase" {
		// URL format: https://project.supabase.co/storage/v1/object/public/bucket/path
		parts := strings.Split(publicURL, "/object/public/"+cfg.SupabaseBucket()+"/")
		if len(parts) > 1 {
			return parts[1]
		}
		return publicURL
	}

	// Local storage: remove base URL prefix
	path := strings.TrimPrefix(publicURL, cfg.BaseURL()+"/")
	return filepath.Join(cfg.UploadDir(), path)
}

// convertToStorageConfig converts MediaConfig to storage-compatible format
func convertToStorageConfig(cfg MediaConfig) MediaConfig {
	return cfg
}
