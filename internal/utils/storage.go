package utils

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StorageInterface defines the interface for storage backends
type StorageInterface interface {
	SaveFile(file *multipart.FileHeader, subDir string) (*MediaInfo, error)
	DeleteFile(filePath string) error
	GetFileURL(filePath string) string
}

// LocalStorage implements local filesystem storage
type LocalStorage struct {
	config MediaConfig
}

// SupabaseStorage implements Supabase Storage
type SupabaseStorage struct {
	config MediaConfig
	client *http.Client
}

// NewStorage creates the appropriate storage backend based on configuration
func NewStorage(cfg MediaConfig) StorageInterface {
	if cfg.StorageType() == "supabase" {
		if cfg.SupabaseStorageURL() == "" || cfg.SupabaseStorageKey() == "" {
			// Fallback to local if Supabase config is missing
			return &LocalStorage{config: cfg}
		}
		return &SupabaseStorage{
			config: cfg,
			client: &http.Client{Timeout: 30 * time.Second},
		}
	}
	return &LocalStorage{config: cfg}
}

// ============================================================================
// Local Storage Implementation
// ============================================================================

func (s *LocalStorage) SaveFile(file *multipart.FileHeader, subDir string) (*MediaInfo, error) {
	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Detect media type
	mediaType := DetectMediaType(file.Filename, s.config)
	if mediaType == MediaTypeUnknown {
		return nil, fmt.Errorf("unsupported file type")
	}

	// Validate file
	if err := ValidateFile(file, mediaType, s.config); err != nil {
		return nil, err
	}

	// Create directory structure: uploads/subDir/YYYY/MM/DD/
	now := time.Now()
	dateDir := filepath.Join(
		s.config.UploadDir(),
		subDir,
		fmt.Sprintf("%d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()),
	)

	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	// Generate unique filename
	hash := md5.Sum([]byte(fmt.Sprintf("%s%d", file.Filename, time.Now().UnixNano())))
	hashStr := fmt.Sprintf("%x", hash)[:8]

	ext := filepath.Ext(file.Filename)
	baseName := strings.TrimSuffix(file.Filename, ext)
	baseName = strings.ReplaceAll(baseName, " ", "_")
	newFilename := fmt.Sprintf("%d_%s_%s%s", now.Unix(), hashStr, baseName, ext)

	// Full path where file will be stored
	storedPath := filepath.Join(dateDir, newFilename)

	// Create destination file
	dst, err := os.Create(storedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// Copy file content
	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	// Generate public URL
	publicPath := strings.TrimPrefix(storedPath, s.config.UploadDir()+string(filepath.Separator))
	publicPath = strings.ReplaceAll(publicPath, string(filepath.Separator), "/")
	publicURL := s.config.BaseURL() + "/" + publicPath

	// Determine content type
	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		if mediaType == MediaTypeImage {
			contentType = "image/" + strings.TrimPrefix(ext, ".")
		} else {
			contentType = "video/" + strings.TrimPrefix(ext, ".")
		}
	}

	return &MediaInfo{
		Filename:    file.Filename,
		StoredPath:  storedPath,
		PublicURL:   publicURL,
		MediaType:   mediaType,
		Size:        file.Size,
		ContentType: contentType,
	}, nil
}

func (s *LocalStorage) DeleteFile(filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

func (s *LocalStorage) GetFileURL(filePath string) string {
	// Remove base directory and convert to URL path
	path := strings.TrimPrefix(filePath, s.config.UploadDir()+string(filepath.Separator))
	path = strings.ReplaceAll(path, string(filepath.Separator), "/")
	return s.config.BaseURL() + "/" + path
}

// ============================================================================
// Supabase Storage Implementation
// ============================================================================

func (s *SupabaseStorage) SaveFile(file *multipart.FileHeader, subDir string) (*MediaInfo, error) {
	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Detect media type
	mediaType := DetectMediaType(file.Filename, s.config)
	if mediaType == MediaTypeUnknown {
		return nil, fmt.Errorf("unsupported file type")
	}

	// Validate file
	if err := ValidateFile(file, mediaType, s.config); err != nil {
		return nil, err
	}

	// Read file content into memory
	fileContent, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	// Generate unique filename
	now := time.Now()
	hash := md5.Sum([]byte(fmt.Sprintf("%s%d", file.Filename, time.Now().UnixNano())))
	hashStr := fmt.Sprintf("%x", hash)[:8]

	ext := filepath.Ext(file.Filename)
	baseName := strings.TrimSuffix(file.Filename, ext)
	baseName = strings.ReplaceAll(baseName, " ", "_")
	
	// Create path structure: subDir/YYYY/MM/DD/filename
	datePath := fmt.Sprintf("%s/%d/%02d/%02d", subDir, now.Year(), now.Month(), now.Day())
	newFilename := fmt.Sprintf("%d_%s_%s%s", now.Unix(), hashStr, baseName, ext)
	filePath := datePath + "/" + newFilename

	// Upload to Supabase Storage
	publicURL, err := s.uploadToSupabase(filePath, fileContent, file.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("failed to upload to Supabase: %w", err)
	}

	// Determine content type
	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		if mediaType == MediaTypeImage {
			contentType = "image/" + strings.TrimPrefix(ext, ".")
		} else {
			contentType = "video/" + strings.TrimPrefix(ext, ".")
		}
	}

	return &MediaInfo{
		Filename:    file.Filename,
		StoredPath:  filePath, // Store the Supabase path
		PublicURL:   publicURL,
		MediaType:   mediaType,
		Size:        file.Size,
		ContentType: contentType,
	}, nil
}

func (s *SupabaseStorage) uploadToSupabase(filePath string, fileContent []byte, contentType string) (string, error) {
	// Supabase Storage upload endpoint
	uploadURL := fmt.Sprintf("%s/object/%s/%s", s.config.SupabaseStorageURL(), s.config.SupabaseBucket(), filePath)

	// Create request
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(fileContent))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseStorageKey())
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true") // Allow overwriting if file exists

	// Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var uploadResp struct {
		Key string `json:"Key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		// If response is not JSON, construct URL manually using our filePath
		publicURL := fmt.Sprintf("%s/object/public/%s/%s", s.config.SupabaseStorageURL(), s.config.SupabaseBucket(), filePath)
		return publicURL, nil
	}

	// Construct public URL
	// Supabase might return Key with or without bucket name prefix
	// Use the Key from response if it's not empty, otherwise use our filePath
	keyPath := uploadResp.Key
	if keyPath == "" {
		keyPath = filePath
	} else {
		// Remove bucket name prefix if it exists in the Key
		// strings.TrimPrefix is safe - if prefix doesn't exist, returns original string
		keyPath = strings.TrimPrefix(keyPath, s.config.SupabaseBucket()+"/")
	}
	
	publicURL := fmt.Sprintf("%s/object/public/%s/%s", s.config.SupabaseStorageURL(), s.config.SupabaseBucket(), keyPath)
	return publicURL, nil
}

func (s *SupabaseStorage) DeleteFile(filePath string) error {
	// Remove base URL prefix if present
	path := filePath
	if strings.HasPrefix(path, s.config.SupabaseStorageURL()) {
		// Extract path from full URL
		path = strings.TrimPrefix(path, s.config.SupabaseStorageURL()+"/object/public/"+s.config.SupabaseBucket()+"/")
	}

	// Supabase Storage delete endpoint
	deleteURL := fmt.Sprintf("%s/object/%s/%s", s.config.SupabaseStorageURL(), s.config.SupabaseBucket(), path)

	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseStorageKey())

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *SupabaseStorage) GetFileURL(filePath string) string {
	// If already a full URL, return as is
	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		return filePath
	}
	// Construct public URL
	return fmt.Sprintf("%s/object/public/%s/%s", s.config.SupabaseStorageURL(), s.config.SupabaseBucket(), filePath)
}

