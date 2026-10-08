package gemini

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"google.golang.org/api/googleapi"
	"google.golang.org/genai"
)

// The pinned SDK returns genai.APIError values. Accept pointers and wrapped
// googleapi errors as well for structured classification.
func sdkAPIError(err error) (genai.APIError, bool) {
	var value genai.APIError
	if errors.As(err, &value) {
		return value, true
	}
	var pointer *genai.APIError
	if errors.As(err, &pointer) && pointer != nil {
		return *pointer, true
	}
	var googleError *googleapi.Error
	if errors.As(err, &googleError) && googleError != nil {
		value = genai.APIError{Code: googleError.Code, Message: googleError.Message}
		for _, detail := range googleError.Details {
			if item, ok := detail.(map[string]interface{}); ok {
				value.Details = append(value.Details, item)
			}
		}
		if len(googleError.Errors) > 0 {
			value.Details = append(value.Details, map[string]interface{}{
				"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
				"reason": googleError.Errors[0].Reason,
			})
		}
		return value, true
	}
	return genai.APIError{}, false
}

func nilSDKError(err error) bool {
	switch value := err.(type) {
	case *genai.APIError:
		return value == nil
	case *googleapi.Error:
		return value == nil
	default:
		return err == nil
	}
}

func addAPIDiagnostics(fields map[string]interface{}, apiError genai.APIError) {
	if apiError.Message != "" {
		fields["provider_exact_message"] = redactProviderMessage(apiError.Message)
	}
	details := make([]map[string]interface{}, 0)
	for _, detail := range apiError.Details {
		if len(details) == 8 {
			break
		}
		// ErrorInfo carries the permission/billing/key reason. Other detail types
		// may contain receipt text, user identities, stack traces or credentials.
		if detail["@type"] != "type.googleapis.com/google.rpc.ErrorInfo" {
			continue
		}
		item := map[string]interface{}{"type": "google.rpc.ErrorInfo"}
		for _, key := range []string{"reason", "domain"} {
			if text, ok := detail[key].(string); ok && text != "" {
				item[key] = redactProviderMessage(text)
			}
		}
		if reason, ok := item["reason"]; ok {
			if _, exists := fields["provider_error_reason"]; !exists {
				fields["provider_error_reason"] = reason
			}
		}
		if metadata, ok := detail["metadata"].(map[string]interface{}); ok {
			if service, ok := metadata["service"].(string); ok {
				item["service"] = redactProviderMessage(service)
			}
		}
		details = append(details, item)
	}
	if len(details) > 0 {
		fields["provider_details"] = details
	}
}

var providerMessageRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[^\s,;]+`),
	regexp.MustCompile(`(?i)\b(?:api[_ -]?key|access[_ -]?token|refresh[_ -]?token|authorization|password|secret)\s*[:=]\s*["']?[^\s,"';]+["']?`),
	regexp.MustCompile(`(?i)\b(?:https?|postgres(?:ql)?)://[^\s<>"']+`),
	regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`),
	regexp.MustCompile(`[A-Za-z0-9_\-]{32,}(?:\.[A-Za-z0-9_\-]+)*`),
}

// Normal diagnostic messages remain verbatim. Credential-like text is
// redacted and oversized messages are capped so provider output cannot flood logs.
func redactProviderMessage(message string) string {
	message = redactProviderDetail(message)
	runes := []rune(message)
	if len(runes) > 4096 {
		return string(runes[:4096]) + " [TRUNCATED]"
	}
	return message
}

// rawProviderLogDetail retains the SDK error chain without a length cap.
// API details/body may contain receipt contents, so exclude those payloads.
// Credential redaction is independent of length and applies to every log field.
func rawProviderLogDetail(cause error, raw string) string {
	var value genai.APIError
	var pointer *genai.APIError
	if errors.As(cause, &value) || (errors.As(cause, &pointer) && pointer != nil) {
		if pointer != nil {
			value = *pointer
		}
		safe := value
		if len(safe.Details) > 0 {
			safe.Details = []map[string]interface{}{{"redacted": "API details"}}
		}
		if safe.Status != "" && !safeDiagnosticStatus(safe.Status) {
			safe.Status = "[REDACTED]"
		}
		raw = strings.ReplaceAll(raw, value.Error(), safe.Error())
	}
	var googleError *googleapi.Error
	if errors.As(cause, &googleError) && googleError != nil && googleError.Body != "" {
		raw = strings.ReplaceAll(raw, googleError.Body, "[REDACTED]")
	}
	for i, pattern := range providerMessageRedactions {
		if i == 2 {
			// Preserve the HTTP target for network diagnosis while removing
			// credentials, query values and fragments from SDK URL errors.
			raw = pattern.ReplaceAllStringFunc(raw, func(target string) string {
				u, err := url.Parse(target)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
					return "[REDACTED]"
				}
				u.User = nil
				query := u.Query()
				for key := range query {
					query[key] = []string{"[REDACTED]"}
				}
				u.RawQuery = query.Encode()
				if u.Fragment != "" {
					u.Fragment = "[REDACTED]"
				}
				return u.String()
			})
		} else {
			raw = pattern.ReplaceAllString(raw, "[REDACTED]")
		}
	}
	return raw
}

func safeDiagnosticStatus(status string) bool {
	switch status {
	case "INVALID_ARGUMENT", "FAILED_PRECONDITION", "PERMISSION_DENIED", "UNAUTHENTICATED", "RESOURCE_EXHAUSTED", "NOT_FOUND", "INTERNAL", "UNAVAILABLE", "DEADLINE_EXCEEDED", "CANCELLED", "UNKNOWN":
		return true
	}
	return false
}

func redactProviderDetail(message string) string {
	for _, pattern := range providerMessageRedactions {
		message = pattern.ReplaceAllString(message, "[REDACTED]")
	}
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, message)
	return message
}
