package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/request"
	"walletx-be/internal/shared/response"
)

// registerStorageTestRoutes exposes only the configured storage port for manual
// development testing. The caller must gate registration on DevMode and use a
// JWT-protected router group. No transaction records or AI calls are involved.
func registerStorageTestRoutes(group *gin.RouterGroup, store domain.ReceiptStorage, appLogger logger.Logger) {
	h := &storageTestHandler{store: store, logger: appLogger}
	routes := group.Group("/dev/storage/receipts")
	routes.POST("", h.upload)
	routes.DELETE("", h.delete)
}

type storageTestHandler struct {
	store  domain.ReceiptStorage
	logger logger.Logger
}

func (h *storageTestHandler) upload(c *gin.Context) {
	owner, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	// Allow multipart overhead while keeping the file itself capped at 10 MiB.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, domain.MaxReceiptBytes+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		var limitErr *http.MaxBytesError
		if errors.As(err, &limitErr) {
			response.FailWithStatus(c, http.StatusRequestEntityTooLarge, "Receipt upload is too large")
		} else {
			response.Fail(c, "Expected multipart/form-data with a receipt file")
		}
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	headers := c.Request.MultipartForm.File["receipt"]
	if len(headers) != 1 {
		response.Fail(c, "Exactly one receipt file is required")
		return
	}
	header := headers[0]
	if header.Size > domain.MaxReceiptBytes {
		response.FailWithStatus(c, http.StatusRequestEntityTooLarge, "Receipt image exceeds 10 MiB")
		return
	}
	if header.Size <= 0 {
		response.Fail(c, "Receipt image is empty")
		return
	}
	file, err := header.Open()
	if err != nil {
		response.Fail(c, "Cannot read receipt image")
		return
	}
	defer file.Close()
	image, err := io.ReadAll(io.LimitReader(file, domain.MaxReceiptBytes+1))
	if err != nil || len(image) == 0 {
		response.Fail(c, "Cannot read receipt image")
		return
	}
	if len(image) > domain.MaxReceiptBytes {
		response.FailWithStatus(c, http.StatusRequestEntityTooLarge, "Receipt image exceeds 10 MiB")
		return
	}
	contentType := http.DetectContentType(image)
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		response.FailWithStatus(c, http.StatusUnsupportedMediaType, "Supported receipt types: JPEG, PNG, GIF, WebP")
		return
	}
	reference, err := h.store.SaveReceipt(c.Request.Context(), owner, contentType, header.Filename, image)
	if err != nil {
		h.storageError(c, err, "test_save_receipt")
		return
	}
	if strings.TrimSpace(reference) == "" {
		h.storageError(c, apperrors.ErrInternal, "test_save_receipt")
		return
	}
	response.SuccessWithStatus(c, http.StatusCreated, gin.H{"path": reference}, "Receipt uploaded successfully")
}

func (h *storageTestHandler) delete(c *gin.Context) {
	owner, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var body struct {
		Path string `json:"path"`
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
		response.Fail(c, "Expected JSON with a receipt path")
		return
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		response.Fail(c, "Expected a single JSON object")
		return
	}
	if err := h.store.DeleteReceipt(c.Request.Context(), owner, body.Path); err != nil {
		h.storageError(c, err, "test_delete_receipt")
		return
	}
	response.Success(c, gin.H{"path": body.Path}, "Receipt deleted successfully")
}

func (h *storageTestHandler) storageError(c *gin.Context, err error, operation string) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		response.Fail(c, "Invalid receipt input or path for authenticated user")
	case errors.Is(err, context.Canceled):
		response.FailWithStatus(c, http.StatusRequestTimeout, "Storage request canceled")
	default:
		// Use a safe classification rather than logging arbitrary provider text.
		safe := apperrors.ErrInternal
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) {
			safe = context.DeadlineExceeded
			status = http.StatusGatewayTimeout
		}
		h.logger.WithError(safe).WithFields(map[string]interface{}{"operation": operation, "dependency": "storage"}).Error("storage test operation failed")
		response.ErrorWithStatus(c, status, "Storage operation failed")
	}
}
