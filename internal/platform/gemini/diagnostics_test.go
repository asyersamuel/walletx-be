package gemini

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestOperationErrorSafeLogFields(t *testing.T) {
	const private = "private-prompt-and-api-key"
	for _, tc := range []struct {
		name   string
		cause  error
		code   int
		status string
		reason string
	}{
		{"restricted model", genai.APIError{Code: 404, Status: "NOT_FOUND", Message: "This model is no longer available to new users. api_key=" + private}, 404, "NOT_FOUND", "model_access_restricted"},
		{"pointer SDK error", &genai.APIError{Code: 429, Status: "RESOURCE_EXHAUSTED", Message: "api_key=" + private, Details: []map[string]any{{"private": private}}}, 429, "RESOURCE_EXHAUSTED", "quota_or_rate_limit"},
		{"wrapped SDK error", fmt.Errorf("%w", genai.APIError{Code: 403, Status: "PERMISSION_DENIED", Message: "api_key=" + private}), 403, "PERMISSION_DENIED", "authentication_or_permissions"},
		{"invalid request", genai.APIError{Code: 400, Status: "INVALID_ARGUMENT", Message: "api_key=" + private}, 400, "INVALID_ARGUMENT", "invalid_request"},
		{"provider unavailable", genai.APIError{Code: 503, Status: "UNAVAILABLE", Message: "api_key=" + private}, 503, "UNAVAILABLE", "provider_unavailable"},
		{"untrusted status", genai.APIError{Code: 404, Status: private, Message: "api_key=" + private}, 404, "", "model_or_endpoint_not_found"},
		{"timeout", context.DeadlineExceeded, 0, "", "timeout"},
		{"cancellation", context.Canceled, 0, "", "cancelled"},
		{"network error", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New(private)}, 0, "", "network_error"},
		{"SDK error", errors.New(private), 0, "", "sdk_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := providerFailure("stream Gemini chat", tc.cause)
			var diagnostic interface{ SafeLogFields() map[string]interface{} }
			require.ErrorAs(t, fmt.Errorf("wrapped: %w", err), &diagnostic)
			fields := diagnostic.SafeLogFields()
			require.Equal(t, "gemini", fields["dependency"])
			require.Equal(t, "stream Gemini chat", fields["provider_operation"])
			require.Equal(t, tc.reason, fields["provider_reason"])
			if tc.code != 0 {
				require.Equal(t, tc.code, fields["provider_http_code"])
			} else {
				require.NotContains(t, fields, "provider_http_code")
			}
			if tc.status != "" {
				require.Equal(t, tc.status, fields["provider_status"])
			} else {
				require.NotContains(t, fields, "provider_status")
			}
			require.NotContains(t, fmt.Sprint(fields), private)
			require.NotContains(t, err.Error(), private)
		})
	}
}
