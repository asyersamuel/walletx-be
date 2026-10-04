package aihttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"walletx-be/internal/modules/ai"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/response"
)

const maxJSONBytes = 1 << 20
const maxImageBytes = 10 << 20

type Handler struct {
	service ai.AIService
	logger  logger.Logger
}

func NewHandler(service ai.AIService, appLogger logger.Logger) *Handler {
	return &Handler{service: service, logger: appLogger}
}

// RegisterRoutes must be called on a group protected by JWT middleware.
func RegisterRoutes(protected *gin.RouterGroup, handler *Handler) {
	protected.POST("/ai/chat", handler.Chat)
	protected.POST("/ai/extract", handler.ExtractTransaction)
}

func userID(c *gin.Context) (uuid.UUID, bool) {
	value, _ := c.Get("user_id")
	raw, ok := value.(string)
	id, err := uuid.Parse(raw)
	if !ok || err != nil || id == uuid.Nil {
		response.Unauthorized(c, "User not authenticated")
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(c *gin.Context, target any) error {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return apperrors.ErrInvalidInput
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err != nil {
			return err
		}
		return apperrors.ErrInvalidInput
	}
	return nil
}

func (h *Handler) Chat(c *gin.Context) {
	requestLogger := h.logger.WithFields(map[string]interface{}{"operation": "ai_chat"})
	requestLogger.Info("Handling AI chat request")
	id, ok := userID(c)
	if !ok {
		return
	}
	var input ai.ChatRequest
	if err := decodeJSON(c, &input); err != nil {
		h.badRequest(c, err)
		return
	}
	input.UserID = id
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	stream, err := h.service.Chat(ctx, input)
	if err != nil {
		h.handleError(c, "chat", err)
		return
	}
	if stream == nil {
		h.handleError(c, "chat", apperrors.ErrInternal)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-stream:
			if ctx.Err() != nil {
				return
			}
			if !open {
				h.streamError(c, apperrors.ErrInternal)
				return
			}
			var payload any
			switch event.Kind {
			case ai.ChatEventThought, ai.ChatEventAnswer:
				payload = struct {
					Text string `json:"text"`
				}{event.Text}
			case ai.ChatEventComplete:
				if event.Message == nil {
					h.streamError(c, apperrors.ErrInternal)
					return
				}
				payload = struct {
					Message *ai.ChatMessage `json:"message"`
				}{event.Message}
			case ai.ChatEventError:
				h.streamError(c, event.Err)
				return
			default:
				h.streamError(c, apperrors.ErrInternal)
				return
			}
			if err := writeEvent(c, event.Kind, payload); err != nil {
				return
			}
			if event.Kind == ai.ChatEventComplete {
				return
			}
		}
	}
}

func writeEvent(c *gin.Context, kind ai.ChatEventKind, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", kind, data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func (h *Handler) streamError(c *gin.Context, err error) {
	if c.Request.Context().Err() != nil {
		return
	}
	h.logFailure("chat_stream", err)
	_ = writeEvent(c, ai.ChatEventError, gin.H{"message": "AI chat failed"})
}

type extractionRequest struct {
	Text     string `json:"text"`
	TimeZone string `json:"timeZone"`
}

func (h *Handler) ExtractTransaction(c *gin.Context) {
	id, ok := userID(c)
	if !ok {
		return
	}
	input := ai.ExtractionInput{UserID: id, ReferenceTime: time.Now(), TimeZone: "Asia/Jakarta"}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		response.Fail(c, "Invalid content type")
		return
	}
	switch mediaType {
	case "application/json":
		var body extractionRequest
		if err := decodeJSON(c, &body); err != nil {
			h.badRequest(c, err)
			return
		}
		input.Text = body.Text
		if body.TimeZone != "" {
			input.TimeZone = body.TimeZone
		}
	case "multipart/form-data":
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageBytes+maxJSONBytes)
		if err := c.Request.ParseMultipartForm(maxJSONBytes); err != nil {
			h.badRequest(c, err)
			return
		}
		defer c.Request.MultipartForm.RemoveAll()
		form := c.Request.MultipartForm
		for key, values := range form.Value {
			if (key != "text" && key != "timeZone") || len(values) != 1 {
				response.Fail(c, "Invalid multipart fields")
				return
			}
		}
		if len(form.File) != 1 || len(form.File["image"]) != 1 {
			response.Fail(c, "Exactly one image file is required")
			return
		}
		file := form.File["image"][0]
		if file.Size > maxImageBytes {
			response.FailWithStatus(c, http.StatusRequestEntityTooLarge, "Image exceeds 10 MiB")
			return
		}
		reader, err := file.Open()
		if err != nil {
			h.handleError(c, "read_image", err)
			return
		}
		image, err := io.ReadAll(io.LimitReader(reader, maxImageBytes+1))
		reader.Close()
		if err != nil {
			h.handleError(c, "read_image", err)
			return
		}
		if len(image) > maxImageBytes {
			response.FailWithStatus(c, http.StatusRequestEntityTooLarge, "Image exceeds 10 MiB")
			return
		}
		// Detect from bytes rather than trusting the upload's declared content type.
		input.Image = image
		input.MIMEType = http.DetectContentType(image)
		input.Text = c.PostForm("text")
		if zone := c.PostForm("timeZone"); zone != "" {
			input.TimeZone = zone
		}
	default:
		response.FailWithStatus(c, http.StatusUnsupportedMediaType, "Use application/json or multipart/form-data")
		return
	}
	result, err := h.service.ExtractTransaction(c.Request.Context(), input)
	if err != nil {
		h.handleError(c, "extract_transaction", err)
		return
	}
	response.Success(c, result, "Transaction extracted successfully")
}

func (h *Handler) badRequest(c *gin.Context, err error) {
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		response.FailWithStatus(c, http.StatusRequestEntityTooLarge, "Request body too large")
		return
	}
	response.Fail(c, "Invalid request body")
}

func (h *Handler) handleError(c *gin.Context, operation string, err error) {
	if c.Request.Context().Err() != nil {
		return
	}
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		response.Fail(c, "Invalid AI request")
	case errors.Is(err, apperrors.ErrUnauthorized):
		response.Unauthorized(c, "User not authenticated")
	default:
		h.logFailure(operation, err)
		response.Error(c, "AI operation failed")
	}
}

func (h *Handler) logFailure(operation string, err error) {
	// Dependency errors may contain prompts or database values. Record a safe
	// classification at this boundary, without copying their message into logs.
	fields := map[string]interface{}{"operation": operation, "error_type": fmt.Sprintf("%T", err)}
	var diagnostic interface{ SafeLogFields() map[string]interface{} }
	if errors.As(err, &diagnostic) {
		for key, value := range diagnostic.SafeLogFields() {
			fields[key] = value
		}
	}
	h.logger.WithError(apperrors.ErrInternal).WithFields(fields).Error("AI operation failed")
}
