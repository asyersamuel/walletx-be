package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

const (
	supabaseReceiptBucket    = "receipts"
	maxSupabaseResponseDrain = 64 << 10
)

// Supabase persists receipts in the receipts bucket. References are relative
// object paths; credentials and public URL construction stay outside the domain.
// The adapter returns failures for the existing service logging boundary.
type Supabase struct {
	baseURL        url.URL
	serviceRoleKey string
	client         *http.Client
}

var _ domain.ReceiptStorage = (*Supabase)(nil)

// NewSupabase requires a project origin, a server-side service-role key and a
// client with a positive timeout. It performs no network requests. HTTP is
// supported only on loopback for local Supabase and isolated tests.
// Configure the client before construction; its transport is shared, while
// redirects are disabled on a private client copy to protect credentials.
func NewSupabase(baseURL, serviceRoleKey string, client *http.Client) (*Supabase, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || u.Hostname() == "" || u.Opaque != "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.Contains(baseURL, "#") || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return nil, fmt.Errorf("supabase project origin is invalid: %w", apperrors.ErrInvalidInput)
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		loopback := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
		if u.Scheme != "http" || !loopback {
			return nil, fmt.Errorf("supabase project origin requires HTTPS or loopback HTTP: %w", apperrors.ErrInvalidInput)
		}
	}
	if strings.TrimSpace(serviceRoleKey) == "" || strings.IndexFunc(serviceRoleKey, func(r rune) bool { return r <= 32 || r >= 127 }) >= 0 {
		return nil, fmt.Errorf("supabase service-role key is required and must be a valid header value: %w", apperrors.ErrInvalidInput)
	}
	if client == nil || client.Timeout <= 0 {
		return nil, fmt.Errorf("supabase HTTP client requires a positive timeout: %w", apperrors.ErrInvalidInput)
	}
	u.Path = ""
	privateClient := *client
	privateClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Supabase{baseURL: *u, serviceRoleKey: serviceRoleKey, client: &privateClient}, nil
}

// SaveReceipt returns only the generated object path after a successful upload.
// Client filenames never determine the object path. Uploads are not retried:
// a transport failure can occur after storage accepted the object, leaving an
// uncertain outcome that requires reconciliation rather than blind retries.
func (s *Supabase) SaveReceipt(ctx context.Context, userID uuid.UUID, contentType, filename string, image []byte) (string, error) {
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
	objectID, err := uuid.NewRandom()
	if err != nil {
		return "", &supabaseOperationError{operation: "generate receipt object", cause: err}
	}
	reference := userID.String() + "/" + objectID.String() + "_" + strconv.FormatInt(time.Now().UnixMilli(), 10) + extension
	endpoint := s.objectEndpoint() + "/" + reference
	if err := s.send(ctx, http.MethodPost, endpoint, contentType, image, "upload receipt"); err != nil {
		return "", err
	}
	return reference, nil
}

// DeleteReceipt deletes an exact owned object using Supabase's batch endpoint.
// Missing objects yield a successful empty result; other HTTP failures,
// including a missing bucket, must remain visible to the cleanup boundary.
func (s *Supabase) DeleteReceipt(ctx context.Context, userID uuid.UUID, reference string) error {
	if ctx == nil || userID == uuid.Nil {
		return apperrors.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validSupabaseReceiptReference(userID, reference) {
		return apperrors.ErrInvalidInput
	}
	body, err := json.Marshal(struct {
		Prefixes []string `json:"prefixes"`
	}{Prefixes: []string{reference}})
	if err != nil {
		return &supabaseOperationError{operation: "prepare receipt deletion", cause: err}
	}
	return s.send(ctx, http.MethodDelete, s.objectEndpoint(), "application/json", body, "delete receipt")
}

func (s *Supabase) objectEndpoint() string {
	u := s.baseURL
	u.Path = "/storage/v1/object/" + supabaseReceiptBucket
	return u.String()
}

func (s *Supabase) send(ctx context.Context, method, endpoint, contentType string, body []byte, operation string) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return &supabaseOperationError{operation: operation, cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
	req.Header.Set("apikey", s.serviceRoleKey)
	req.Header.Set("Content-Type", contentType)
	if method == http.MethodPost {
		req.Header.Set("x-upsert", "false")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return &supabaseOperationError{operation: operation, cause: err}
	}
	defer response.Body.Close()
	// Response JSON is not needed to establish success or construct references.
	// Bounded draining permits connection reuse without retaining provider text.
	// Once successful headers arrive, a body read failure must not hide a saved
	// object from the caller and prevent the existing compensation lifecycle.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxSupabaseResponseDrain))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &supabaseOperationError{operation: operation, status: response.StatusCode}
	}
	return nil
}

func validSupabaseReceiptReference(owner uuid.UUID, reference string) bool {
	prefix := owner.String() + "/"
	if !strings.HasPrefix(reference, prefix) {
		return false
	}
	object := strings.TrimPrefix(reference, prefix)
	extension := path.Ext(object)
	if extension != ".png" && extension != ".jpg" && extension != ".gif" && extension != ".webp" {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(object, extension), "_")
	if len(parts) != 2 {
		return false
	}
	id, err := uuid.Parse(parts[0])
	if err != nil || id == uuid.Nil || id.String() != parts[0] {
		return false
	}
	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	return err == nil && timestamp > 0 && strconv.FormatInt(timestamp, 10) == parts[1]
}

// supabaseOperationError keeps provider bodies, URLs and transport error text
// out of ordinary error logging while preserving errors.Is/errors.As behavior.
type supabaseOperationError struct {
	operation string
	status    int
	cause     error
}

func (e *supabaseOperationError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("supabase %s failed (HTTP %d)", e.operation, e.status)
	}
	return "supabase " + e.operation + " failed"
}

func (e *supabaseOperationError) Unwrap() error { return e.cause }
