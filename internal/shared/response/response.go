package response

import (
	"net/http"
	"time"

	"walletx-be/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

// APIResponse represents the standard API response format
// Following JSON:API specification best practices
type APIResponse struct {
	Status    string      `json:"status"`              // "success", "error", or "fail"
	Message   string      `json:"message"`              // Human-readable message
	Data      interface{} `json:"data,omitempty"`      // Response payload (omitted if null)
	Errors    interface{} `json:"errors,omitempty"`     // Error details (only for errors/failures)
	Meta      interface{} `json:"meta,omitempty"`       // Metadata (pagination, etc.)
	Timestamp string      `json:"timestamp,omitempty"`   // Response timestamp (ISO 8601)
}

// Response status constants
const (
	StatusSuccess = "success" // Operation succeeded
	StatusError   = "error"   // Server error (500)
	StatusFail    = "fail"    // Client error (400-499)
)

// Success sends a successful response with data
func Success(c *gin.Context, data interface{}, message string) {
	if message == "" {
		message = "Operation completed successfully"
	}

	c.JSON(http.StatusOK, APIResponse{
		Status:    StatusSuccess,
		Data:      data,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// SuccessWithStatus sends a successful response with custom HTTP status
func SuccessWithStatus(c *gin.Context, httpStatus int, data interface{}, message string) {
	if message == "" {
		message = "Operation completed successfully"
	}

	c.JSON(httpStatus, APIResponse{
		Status:    StatusSuccess,
		Data:      data,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// SuccessWithPagination sends a successful response with pagination metadata
func SuccessWithPagination(c *gin.Context, data interface{}, message string, meta pagination.Meta) {
	if message == "" {
		message = "Operation completed successfully"
	}

	c.JSON(http.StatusOK, APIResponse{
		Status:    StatusSuccess,
		Data:      data,
		Message:   message,
		Meta:      meta,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Error sends an error response (500 level errors - server issues)
func Error(c *gin.Context, message string) {
	if message == "" {
		message = "Internal server error"
	}

	c.JSON(http.StatusInternalServerError, APIResponse{
		Status:    StatusError,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorWithDetails sends an error response with error details
func ErrorWithDetails(c *gin.Context, message string, details interface{}) {
	if message == "" {
		message = "Internal server error"
	}

	c.JSON(http.StatusInternalServerError, APIResponse{
		Status:    StatusError,
		Message:   message,
		Errors:    details,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorWithStatus sends an error response with custom HTTP status
func ErrorWithStatus(c *gin.Context, httpStatus int, message string) {
	if message == "" {
		message = "An error occurred"
	}

	c.JSON(httpStatus, APIResponse{
		Status:    StatusError,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Fail sends a fail response (400 level errors - client issues)
func Fail(c *gin.Context, message string) {
	if message == "" {
		message = "Bad request"
	}

	c.JSON(http.StatusBadRequest, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// FailWithStatus sends a fail response with custom HTTP status
func FailWithStatus(c *gin.Context, httpStatus int, message string) {
	if message == "" {
		message = "Request failed"
	}

	c.JSON(httpStatus, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// FailWithDetails sends a fail response with error details
func FailWithDetails(c *gin.Context, message string, details interface{}) {
	if message == "" {
		message = "Request failed"
	}

	c.JSON(http.StatusBadRequest, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Errors:    details,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ValidationError sends a validation error response
func ValidationError(c *gin.Context, message string) {
	if message == "" {
		message = "Validation failed"
	}

	c.JSON(http.StatusBadRequest, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Unauthorized sends an unauthorized response
func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "Unauthorized access"
	}

	c.JSON(http.StatusUnauthorized, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Forbidden sends a forbidden response
func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = "Access forbidden"
	}

	c.JSON(http.StatusForbidden, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// NotFound sends a not found response
func NotFound(c *gin.Context, message string) {
	if message == "" {
		message = "Resource not found"
	}

	c.JSON(http.StatusNotFound, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
