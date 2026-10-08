// Package storage implements durable receipt storage adapters.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

// Local stores private images beneath a persistent directory. References are
// receipt://owner/object.ext, not public URLs or paths into the server filesystem.
// Mount this directory on persistent storage when running in a container.
type Local struct {
	directory string
}

var _ domain.ReceiptStorage = (*Local)(nil)
var _ domain.ReceiptReader = (*Local)(nil)

func NewLocal(directory string) (*Local, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("receipt storage directory is required: %w", apperrors.ErrInvalidInput)
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve receipt storage directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return nil, fmt.Errorf("initialize receipt storage directory: %w", err)
	}
	return &Local{directory: abs}, nil
}

func (s *Local) SaveReceipt(ctx context.Context, userID uuid.UUID, contentType, filename string, image []byte) (string, error) {
	if ctx == nil || userID == uuid.Nil || len(image) == 0 || len(image) > domain.MaxReceiptBytes {
		return "", apperrors.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	extension := receiptExtension(contentType)
	if extension == "" {
		return "", apperrors.ErrInvalidInput
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	owner := userID.String()
	if err := root.MkdirAll(owner, 0700); err != nil {
		return "", err
	}
	// Client filenames never control filesystem paths or object keys.
	object := uuid.NewString()
	temporary := filepath.Join(owner, object+".part")
	final := filepath.Join(owner, object+extension)
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer func() {
		file.Close()
		_ = root.Remove(temporary)
	}()
	if _, err := file.Write(image); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := root.Rename(temporary, final); err != nil {
		return "", err
	}
	return "receipt://" + owner + "/" + object + extension, nil
}

// ReadReceipt resolves a reference after restart without exposing the directory.
// This is also the retrieval path available to a future extraction worker.
func (s *Local) ReadReceipt(ctx context.Context, userID uuid.UUID, reference string) ([]byte, error) {
	name, err := receiptPath(ctx, userID, reference)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenInRoot(s.directory, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	image, err := io.ReadAll(io.LimitReader(file, domain.MaxReceiptBytes+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateStoredReceipt(image); err != nil {
		return nil, err
	}
	return image, nil
}

func (s *Local) DeleteReceipt(ctx context.Context, userID uuid.UUID, reference string) error {
	name, err := receiptPath(ctx, userID, reference)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(name); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func receiptPath(ctx context.Context, userID uuid.UUID, reference string) (string, error) {
	if ctx == nil || userID == uuid.Nil {
		return "", apperrors.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	u, err := url.Parse(reference)
	if err != nil || u.Scheme != "receipt" || u.Host != userID.String() || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", apperrors.ErrInvalidInput
	}
	name := strings.TrimPrefix(u.Path, "/")
	extension := filepath.Ext(name)
	if extension != ".png" && extension != ".jpg" && extension != ".gif" && extension != ".webp" {
		return "", apperrors.ErrInvalidInput
	}
	object := strings.TrimSuffix(name, extension)
	id, err := uuid.Parse(object)
	if err != nil || id == uuid.Nil || id.String() != object {
		return "", apperrors.ErrInvalidInput
	}
	return filepath.Join(userID.String(), name), nil
}

func receiptExtension(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
