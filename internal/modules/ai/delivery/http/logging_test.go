package aihttp

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
)

type safeDiagnosticError struct{}

func (safeDiagnosticError) Error() string { return "secret provider response" }
func (safeDiagnosticError) SafeLogFields() map[string]interface{} {
	return map[string]interface{}{"provider_http_code": 404, "provider_status": "NOT_FOUND", "provider_reason": "model_access_restricted"}
}

type capturedLogger struct {
	nopLogger
	fields  map[string]interface{}
	err     error
	message string
}

func (l *capturedLogger) WithError(err error) logger.Logger { l.err = err; return l }
func (l *capturedLogger) WithFields(fields map[string]interface{}) logger.Logger {
	l.fields = fields
	return l
}
func (l *capturedLogger) Error(message string) { l.message = message }

func TestLogFailurePreservesSafeDiagnostics(t *testing.T) {
	for _, cause := range []error{fmt.Errorf("wrapped: %w", safeDiagnosticError{}), errors.New("secret provider response")} {
		captured := &capturedLogger{}
		h := NewHandler(nil, captured)
		h.logFailure("chat_stream", cause)
		require.Equal(t, "chat_stream", captured.fields["operation"])
		require.Equal(t, "AI operation failed", captured.message)
		require.ErrorIs(t, captured.err, apperrors.ErrInternal)
		if errors.As(cause, new(safeDiagnosticError)) {
			require.Equal(t, 404, captured.fields["provider_http_code"])
			require.Equal(t, "NOT_FOUND", captured.fields["provider_status"])
		} else {
			require.NotContains(t, captured.fields, "provider_http_code")
		}
		require.NotContains(t, fmt.Sprint(captured.fields), "secret provider response")
		require.NotContains(t, captured.err.Error(), "secret provider response")
	}
}
