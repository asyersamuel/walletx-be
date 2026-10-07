package gemini

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"google.golang.org/genai"
)

// stripJSONMarkdown accepts a single outer JSON/plain code fence. It does not
// search prose for JSON or remove backticks from JSON string values; strict
// decoding still rejects unknown/duplicate fields and trailing content.
func stripJSONMarkdown(text string) (string, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text, nil
	}
	opening, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return "", errors.New("invalid JSON markdown fence")
	}
	language := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(opening, "```")))
	if language != "" && language != "json" {
		return "", errors.New("invalid JSON markdown fence")
	}
	closing := strings.LastIndex(rest, "\n")
	if closing < 0 || strings.TrimSpace(rest[closing+1:]) != "```" {
		return "", errors.New("invalid JSON markdown fence")
	}
	return strings.TrimSpace(rest[:closing]), nil
}

type extractionParseError struct {
	cause  error
	reason string
}

func (e *extractionParseError) Error() string { return e.reason }
func (e *extractionParseError) Unwrap() error { return e.cause }

// Reasons describe the validation failure without copying provider output.
func extractionParseReason(err error) string {
	var syntax *json.SyntaxError
	var wrongType *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return "extraction_json_syntax_error"
	case errors.As(err, &wrongType):
		return "extraction_json_type_mismatch"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "extraction_json_incomplete"
	}
	switch err.Error() {
	case "invalid JSON markdown fence":
		return "extraction_invalid_markdown_fence"
	case "extraction must be a JSON object":
		return "extraction_not_json_object"
	case "unexpected extraction field":
		return "extraction_unknown_field"
	case "duplicate extraction field":
		return "extraction_duplicate_field"
	case "required extraction field is null":
		return "extraction_required_field_null"
	case "extraction is missing required fields":
		return "extraction_missing_fields"
	case "extraction contains trailing data":
		return "extraction_trailing_data"
	case "invalid extracted transaction values":
		return "extraction_invalid_values"
	case "extracted category is not allowed":
		return "extraction_category_not_allowed"
	case "invalid extracted calendar date":
		return "extraction_invalid_date"
	default:
		return "extraction_invalid_json"
	}
}

func safeGeminiModel(model string) bool {
	if !strings.HasPrefix(model, "gemini-") || len(model) > 128 {
		return false
	}
	for _, c := range model {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func generationExtractionFailure(response *genai.GenerateContentResponse, cause error, model string) error {
	reason := "extraction_invalid_generation_response"
	if response != nil && response.PromptFeedback != nil && response.PromptFeedback.BlockReason != "" && response.PromptFeedback.BlockReason != genai.BlockedReasonUnspecified {
		reason = "extraction_prompt_blocked"
	} else if response != nil && len(response.Candidates) == 1 && response.Candidates[0] != nil {
		switch string(response.Candidates[0].FinishReason) {
		case "MAX_TOKENS":
			reason = "extraction_output_token_limit"
		case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT":
			reason = "extraction_generation_blocked"
		}
	}
	return providerFailure("validate Gemini extraction response", &extractionParseError{cause: cause, reason: reason}, model)
}
