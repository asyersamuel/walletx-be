package gemini

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderRawErrorRetainsChainWithoutTruncation(t *testing.T) {
	for _, cause := range []error{
		fmt.Errorf("SDK request failed: %w", context.DeadlineExceeded),
		&net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "i/o timeout", Name: "generativelanguage.googleapis.com", IsTimeout: true}},
		&net.OpError{Op: "read", Net: "tcp", Err: errors.New("i/o timeout")},
		&net.DNSError{Err: "no such host", Name: "generativelanguage.googleapis.com"},
		errors.New(strings.Repeat("SDK transport detail; ", 1000) + "tail: context deadline exceeded"),
	} {
		err := providerFailure("extract transaction with Gemini", cause, "gemini-test")
		var diagnostic *operationError
		require.ErrorAs(t, err, &diagnostic)
		require.Equal(t, cause.Error(), diagnostic.ProviderRawErrorDetail)
		require.Equal(t, cause.Error(), diagnostic.SafeLogFields()["provider_raw_error_detail"])
		require.ErrorIs(t, err, cause)
		require.NotContains(t, diagnostic.SafeLogFields()["provider_raw_error_detail"], "[TRUNCATED]")
	}
}

func TestProviderRawURLDetailRedactsCredentialsAndRetainsTarget(t *testing.T) {
	cause := &url.Error{Op: "Post", URL: "https://user:password@generativelanguage.googleapis.com/v1beta/models/gemini-test:generateContent?key=private-key", Err: fmt.Errorf("waiting for response headers: %w", context.DeadlineExceeded)}
	var diagnostic *operationError
	require.ErrorAs(t, providerFailure("extract", cause), &diagnostic)
	require.Equal(t, cause.Error(), diagnostic.ProviderRawErrorDetail)
	detail := diagnostic.SafeLogFields()["provider_raw_error_detail"].(string)
	require.Contains(t, detail, "generativelanguage.googleapis.com/v1beta/models/gemini-test:generateContent")
	require.Contains(t, detail, "waiting for response headers: context deadline exceeded")
	require.NotContains(t, detail, "private-key")
	require.NotContains(t, detail, "user:password")
}

func TestProviderRawDetailNilCauses(t *testing.T) {
	var diagnostic *operationError
	require.ErrorAs(t, providerFailure("extract", nil), &diagnostic)
	require.Empty(t, diagnostic.ProviderRawErrorDetail)
	require.NotContains(t, diagnostic.SafeLogFields(), "provider_raw_error_detail")
}
