package utils

import (
	"mime/multipart"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateFile(t *testing.T) {
	type args struct {
		file      *multipart.FileHeader
		mediaType MediaType
		cfg       MediaConfig
	}
	tests := []struct {
		// Menguji validasi file image yang memenuhi semua persyaratan
		name    string
		args    args
		wantErr bool
	}{
		{
			// Menguji validasi file image yang memenuhi semua persyaratan
			name: "Valid JPG image",
			args: args{
				file:      createMockFileHeader("photo.jpg", 1*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji validasi file video yang memenuhi semua persyaratan
			name: "Valid MP4 video",
			args: args{
				file:      createMockFileHeader("video.mp4", 10*1024*1024),
				mediaType: MediaTypeVideo,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji validasi image pada batas size limit yang diizinkan
			name: "Image at exact size limit",
			args: args{
				file:      createMockFileHeader("photo.jpg", 5*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji validasi video pada batas size limit yang diizinkan
			name: "Video at exact size limit",
			args: args{
				file:      createMockFileHeader("video.mp4", 50*1024*1024),
				mediaType: MediaTypeVideo,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji error ketika image exceeds maximum size
			name: "Image exceeds max size",
			args: args{
				file:      createMockFileHeader("photo.jpg", 6*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: true,
		},
		{
			// Menguji error ketika video exceeds maximum size
			name: "Video exceeds max size",
			args: args{
				file:      createMockFileHeader("video.mp4", 51*1024*1024),
				mediaType: MediaTypeVideo,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: true,
		},
		{
			// Menguji error untuk format image yang tidak diizinkan
			name: "Invalid image extension GIF",
			args: args{
				file:      createMockFileHeader("image.gif", 1*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: true,
		},
		{
			// Menguji error untuk format video yang tidak diizinkan
			name: "Invalid video extension MOV",
			args: args{
				file:      createMockFileHeader("video.mov", 10*1024*1024),
				mediaType: MediaTypeVideo,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: true,
		},
		{
			// Menguji validasi image dengan extension PNG yang diizinkan
			name: "Valid PNG image",
			args: args{
				file:      createMockFileHeader("photo.png", 2*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji validasi image JPEG dengan uppercase extension
			name: "Uppercase JPG extension",
			args: args{
				file:      createMockFileHeader("photo.JPG", 1*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji validasi file dengan multiple dots dalam filename
			name: "Multiple dots in filename",
			args: args{
				file:      createMockFileHeader("my.photo.jpg", 1*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji validasi file dengan size 0 bytes
			name: "Zero byte file",
			args: args{
				file:      createMockFileHeader("empty.jpg", 0),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: false,
		},
		{
			// Menguji error untuk format image yang di-blacklist (SVG)
			name: "SVG image not allowed",
			args: args{
				file:      createMockFileHeader("vector.svg", 1*1024*1024),
				mediaType: MediaTypeImage,
				cfg:       createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFile(tt.args.file, tt.args.mediaType, tt.args.cfg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDetectMediaType(t *testing.T) {
	type args struct {
		filename string
		cfg      MediaConfig
	}
	tests := []struct {
		// Menguji deteksi jenis media untuk file dengan extension JPG
		name string
		args args
		want MediaType
	}{
		{
			// Menguji deteksi jenis media untuk file dengan extension JPG
			name: "JPG image detection",
			args: args{
				filename: "photo.jpg",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeImage,
		},
		{
			// Menguji deteksi jenis media untuk file dengan extension MP4
			name: "MP4 video detection",
			args: args{
				filename: "video.mp4",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeVideo,
		},
		{
			// Menguji deteksi jenis media untuk file dengan extension uppercase
			name: "Uppercase extension detection",
			args: args{
				filename: "PHOTO.JPG",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeImage,
		},
		{
			// Menguji deteksi jenis media untuk file dengan multiple dots
			name: "Multiple dots in filename",
			args: args{
				filename: "my.photo.jpg",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeImage,
		},
		{
			// Menguji deteksi jenis media untuk file dengan extension tidak didukung
			name: "Unsupported extension GIF",
			args: args{
				filename: "image.gif",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeUnknown,
		},
		{
			// Menguji deteksi jenis media untuk file tanpa extension
			name: "No extension returns unknown",
			args: args{
				filename: "file",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeUnknown,
		},
		{
			// Menguji deteksi jenis media untuk file dengan extension kosong
			name: "Empty filename",
			args: args{
				filename: "",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeUnknown,
		},
		{
			// Menguji deteksi jenis media untuk file PNG
			name: "PNG image detection",
			args: args{
				filename: "photo.png",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeImage,
		},
		{
			// Menguji deteksi jenis media untuk file JPEG
			name: "JPEG image detection",
			args: args{
				filename: "photo.jpeg",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeImage,
		},
		{
			// Menguji deteksi jenis media untuk file dengan extension acak
			name: "Random extension returns unknown",
			args: args{
				filename: "document.pdf",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeUnknown,
		},
		{
			// Menguji deteksi untuk extension case-insensitive
			name: "Case insensitive PNG detection",
			args: args{
				filename: "Photo.PNG",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeImage,
		},
		{
			// Menguji deteksi untuk filename dengan path
			name: "Filename with path separator",
			args: args{
				filename: "/uploads/photo.jpg",
				cfg:      createMockMediaConfig(5*1024*1024, 50*1024*1024),
			},
			want: MediaTypeImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectMediaType(tt.args.filename, tt.args.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetFileFromURL(t *testing.T) {
	type args struct {
		publicURL string
		cfg       MediaConfig
	}
	tests := []struct {
		// Menguji ekstraksi path file dari URL Supabase
		name string
		args args
		want string
	}{
		{
			// Menguji ekstraksi path file dari URL Supabase
			name: "Supabase public URL",
			args: args{
				publicURL: "https://project.supabase.co/storage/v1/object/public/bucket/2024/01/01/photo.jpg",
				cfg:       createMockSupabaseMediaConfig("https://project.supabase.co/storage/v1", "bucket", "key"),
			},
			want: "2024/01/01/photo.jpg",
		},
		{
			// Menguji ekstraksi path file dari URL lokal
			name: "Local storage URL",
			args: args{
				publicURL: "http://localhost/uploads/2024/01/01/file.jpg",
				cfg:       createMockLocalMediaConfig("/uploads", "http://localhost"),
			},
			want: "uploads/2024/01/01/file.jpg",
		},
		{
			// Menguji ekstraksi path dari URL Supabase dengan nested path
			name: "Supabase URL with nested path",
			args: args{
				publicURL: "https://xyz.supabase.co/storage/v1/object/public/avatars/users/photo.jpg",
				cfg:       createMockSupabaseMediaConfig("https://xyz.supabase.co/storage/v1", "avatars", "key"),
			},
			want: "users/photo.jpg",
		},
		{
			// Menguji ekstraksi path dari URL tanpa bucket match
			name: "URL without matching bucket pattern",
			args: args{
				publicURL: "https://other.com/path/to/file.jpg",
				cfg:       createMockSupabaseMediaConfig("https://project.supabase.co/storage/v1", "bucket", "key"),
			},
			want: "https://other.com/path/to/file.jpg",
		},
		{
			// Menguji ekstraksi path dari URL lokal tanpa prefix
			name: "Local URL without base prefix",
			args: args{
				publicURL: "/uploads/image.png",
				cfg:       createMockLocalMediaConfig("/uploads", "http://localhost"),
			},
			want: "uploads/image.png",
		},
		{
			// Menguji ekstraksi path dengan empty URL untuk local storage
			name: "Empty URL returns upload directory",
			args: args{
				publicURL: "",
				cfg:       createMockLocalMediaConfig("/uploads", "http://localhost"),
			},
			want: "uploads",
		},
		{
			// Menguji ekstraksi path dari URL Supabase dengan query parameters
			name: "Supabase URL may contain query params",
			args: args{
				publicURL: "https://project.supabase.co/storage/v1/object/public/bucket/image.jpg?token=abc",
				cfg:       createMockSupabaseMediaConfig("https://project.supabase.co/storage/v1", "bucket", "key"),
			},
			want: "image.jpg?token=abc",
		},
		{
			// Menguji ekstraksi path untuk filename dengan special characters
			name: "Filename with special characters",
			args: args{
				publicURL: "https://project.supabase.co/storage/v1/object/public/bucket/my photo (1).jpg",
				cfg:       createMockSupabaseMediaConfig("https://project.supabase.co/storage/v1", "bucket", "key"),
			},
			want: "my photo (1).jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetFileFromURL(tt.args.publicURL, tt.args.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func createMockFileHeader(filename string, size int64) *multipart.FileHeader {
	return &multipart.FileHeader{
		Filename: filename,
		Size:     size,
		Header:   make(map[string][]string),
	}
}

func createMockMediaConfig(maxImageSize, maxVideoSize int64) MediaConfig {
	return MediaConfig{
		config: mockConfig{
			maxImageSize: maxImageSize,
			maxVideoSize: maxVideoSize,
		},
		AllowedImages: []string{".jpg", ".jpeg", ".png"},
		AllowedVideos: []string{".mp4"},
	}
}

func createMockLocalMediaConfig(uploadDir, baseURL string) MediaConfig {
	return MediaConfig{
		config: mockConfig{
			storageType: "local",
			uploadDir:   uploadDir,
			baseURL:     baseURL,
		},
		AllowedImages: []string{".jpg", ".jpeg", ".png"},
		AllowedVideos: []string{".mp4"},
	}
}

func createMockSupabaseMediaConfig(supabaseURL, bucket, supabaseKey string) MediaConfig {
	return MediaConfig{
		config: mockConfig{
			storageType:        "supabase",
			supabaseStorageURL: supabaseURL,
			supabaseBucket:     bucket,
			supabaseStorageKey: supabaseKey,
		},
		AllowedImages: []string{".jpg", ".jpeg", ".png"},
		AllowedVideos: []string{".mp4"},
	}
}

type mockConfig struct {
	storageType        string
	uploadDir          string
	baseURL            string
	maxImageSize       int64
	maxVideoSize       int64
	supabaseStorageURL string
	supabaseStorageKey string
	supabaseBucket     string
}

func (m mockConfig) StorageType() string        { return m.storageType }
func (m mockConfig) UploadDir() string          { return m.uploadDir }
func (m mockConfig) MaxImageSize() int64        { return m.maxImageSize }
func (m mockConfig) MaxVideoSize() int64        { return m.maxVideoSize }
func (m mockConfig) BaseURL() string            { return m.baseURL }
func (m mockConfig) SupabaseStorageURL() string { return m.supabaseStorageURL }
func (m mockConfig) SupabaseStorageKey() string { return m.supabaseStorageKey }
func (m mockConfig) SupabaseBucket() string     { return m.supabaseBucket }

var _ MediaConfigInterface = (*mockConfig)(nil)

type MediaConfigInterface interface {
	StorageType() string
	UploadDir() string
	MaxImageSize() int64
	MaxVideoSize() int64
	BaseURL() string
	SupabaseStorageURL() string
	SupabaseStorageKey() string
	SupabaseBucket() string
}

func init() {
	_ = time.Now
}
