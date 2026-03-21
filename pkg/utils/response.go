package utils

import (
	"net/http"
	"time"

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

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// Response status constants
const (
	StatusSuccess = "success" // Operation succeeded
	StatusError   = "error"   // Server error (500)
	StatusFail    = "fail"    // Client error (400-499)
)

// SuccessResponse sends a successful response with data
func SuccessResponse(c *gin.Context, data interface{}, message string) {
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

// SuccessResponseWithStatus sends a successful response with custom HTTP status
func SuccessResponseWithStatus(c *gin.Context, httpStatus int, data interface{}, message string) {
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

// SuccessResponseWithPagination sends a successful response with pagination metadata
func SuccessResponseWithPagination(c *gin.Context, data interface{}, message string, pagination PaginationMeta) {
	if message == "" {
		message = "Operation completed successfully"
	}
	
	c.JSON(http.StatusOK, APIResponse{
		Status:    StatusSuccess,
		Data:      data,
		Message:   message,
		Meta:      pagination,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorResponse sends an error response (500 level errors - server issues)
func ErrorResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Internal server error"
	}
	
	c.JSON(http.StatusInternalServerError, APIResponse{
		Status:    StatusError,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorResponseWithDetails sends an error response with error details
func ErrorResponseWithDetails(c *gin.Context, message string, errors interface{}) {
	if message == "" {
		message = "Internal server error"
	}
	
	c.JSON(http.StatusInternalServerError, APIResponse{
		Status:    StatusError,
		Message:   message,
		Errors:    errors,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorResponseWithStatus sends an error response with custom HTTP status
func ErrorResponseWithStatus(c *gin.Context, httpStatus int, message string) {
	if message == "" {
		message = "An error occurred"
	}
	
	c.JSON(httpStatus, APIResponse{
		Status:    StatusError,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// FailResponse sends a fail response (400 level errors - client issues)
func FailResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Bad request"
	}
	
	c.JSON(http.StatusBadRequest, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// FailResponseWithStatus sends a fail response with custom HTTP status
func FailResponseWithStatus(c *gin.Context, httpStatus int, message string) {
	if message == "" {
		message = "Request failed"
	}
	
	c.JSON(httpStatus, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// FailResponseWithDetails sends a fail response with error details
func FailResponseWithDetails(c *gin.Context, message string, errors interface{}) {
	if message == "" {
		message = "Request failed"
	}
	
	c.JSON(http.StatusBadRequest, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Errors:    errors,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ValidationErrorResponse sends a validation error response
func ValidationErrorResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Validation failed"
	}
	
	c.JSON(http.StatusBadRequest, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ValidationErrorResponseWithDetails sends a validation error response with field-level errors
func ValidationErrorResponseWithDetails(c *gin.Context, message string, errors interface{}) {
	if message == "" {
		message = "Validation failed"
	}
	
	c.JSON(http.StatusBadRequest, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Errors:    errors,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// UnauthorizedResponse sends an unauthorized response
func UnauthorizedResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Unauthorized access"
	}
	
	c.JSON(http.StatusUnauthorized, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ForbiddenResponse sends a forbidden response
func ForbiddenResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Access forbidden"
	}
	
	c.JSON(http.StatusForbidden, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// NotFoundResponse sends a not found response
func NotFoundResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Resource not found"
	}
	
	c.JSON(http.StatusNotFound, APIResponse{
		Status:    StatusFail,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// CalculatePaginationMeta calculates pagination metadata from total count, limit, and offset
func CalculatePaginationMeta(total int, limit int, offset int) PaginationMeta {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	
	page := (offset / limit) + 1
	if offset == 0 {
		page = 1
	}
	
	totalPages := (total + limit - 1) / limit // Ceiling division
	if totalPages == 0 {
		totalPages = 1
	}
	
	return PaginationMeta{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
