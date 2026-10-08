package gemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/genai"
)

func TestSDKDiagnosticsUnwrapSupportedErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cause   error
		message string
	}{
		{"genai value", genai.APIError{Code: 403, Status: "PERMISSION_DENIED", Message: "Consumer invalid"}, "Consumer invalid"},
		{"genai pointer", &genai.APIError{Code: 403, Message: "Billing not enabled"}, "Billing not enabled"},
		{"googleapi pointer", &googleapi.Error{Code: 403, Message: "API key not valid", Body: "private body", Header: http.Header{"Authorization": {"secret"}}}, "API key not valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := providerFailure("extract", fmt.Errorf("wrapped: %w", tc.cause))
			var diagnostic *operationError
			require.ErrorAs(t, err, &diagnostic)
			fields := diagnostic.SafeLogFields()
			require.Equal(t, 403, fields["provider_http_code"])
			require.Equal(t, tc.message, fields["provider_exact_message"])
			switch tc.cause.(type) {
			case genai.APIError:
				var original genai.APIError
				require.ErrorAs(t, err, &original)
				require.Equal(t, tc.cause, original)
			default:
				require.ErrorIs(t, err, tc.cause)
			}
			serialized := fmt.Sprint(fields)
			require.NotContains(t, serialized, "private body")
			require.NotContains(t, serialized, "Authorization")
		})
	}
}

func TestSDKDiagnosticsCapturePermissionDetailsFromHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		writeJSONResponse(t, w, map[string]interface{}{"error": map[string]interface{}{
			"code": 403, "status": "PERMISSION_DENIED", "message": "Permission denied: Consumer invalid",
			"details": []map[string]interface{}{{
				"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "CONSUMER_INVALID", "domain": "googleapis.com",
				"metadata": map[string]interface{}{"service": "generativelanguage.googleapis.com", "consumer": "private identity", "api_key": "private credential"},
			}, {"@type": "type.googleapis.com/google.rpc.DebugInfo", "detail": "private receipt"}},
		}})
	})
	_, err := p.ExtractTransaction(context.Background(), extractionFixture())
	var diagnostic *operationError
	require.ErrorAs(t, err, &diagnostic)
	fields := diagnostic.SafeLogFields()
	require.Equal(t, "Permission denied: Consumer invalid", fields["provider_exact_message"])
	require.Equal(t, diagnostic.cause.Error(), diagnostic.ProviderRawErrorDetail)
	require.Contains(t, fields["provider_raw_error_detail"], "Permission denied: Consumer invalid")
	require.Equal(t, "CONSUMER_INVALID", fields["provider_error_reason"])
	require.Equal(t, "PERMISSION_DENIED", fields["provider_status"])
	details := fields["provider_details"].([]map[string]interface{})
	require.Len(t, details, 1)
	require.Equal(t, "generativelanguage.googleapis.com", details[0]["service"])
	require.NotContains(t, fmt.Sprint(fields), "private")
}

func TestSDKDiagnosticsGoogleAPIReasonAndUnknownErrors(t *testing.T) {
	diagnostic := &operationError{cause: &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "billingNotEnabled"}}}}
	require.Equal(t, "billingNotEnabled", diagnostic.SafeLogFields()["provider_error_reason"])
	var nilGenai *genai.APIError
	var nilGoogle *googleapi.Error
	for _, cause := range []error{nil, nilGenai, nilGoogle, errors.New("private network information"), context.Canceled} {
		require.NotPanics(t, func() {
			fields := (&operationError{cause: cause}).SafeLogFields()
			require.NotContains(t, fields, "provider_exact_message")
			require.NotContains(t, fields, "provider_details")
		})
	}
}

func TestProviderMessageRedactionAndBounds(t *testing.T) {
	for _, message := range []string{"API key not valid", "Billing not enabled", "Consumer invalid"} {
		require.Equal(t, message, redactProviderMessage(message))
	}
	message := "API key=private-key Bearer private-token https://example.test/?key=private user@example.test " + strings.Repeat("a", 39)
	redacted := redactProviderMessage(message)
	for _, secret := range []string{"private-key", "private-token", "example.test", "user@example.test", strings.Repeat("a", 39)} {
		require.NotContains(t, redacted, secret)
	}
	require.Contains(t, redacted, "[REDACTED]")
	require.Contains(t, redactProviderMessage(strings.Repeat("word ", 2000)), "[TRUNCATED]")
}
