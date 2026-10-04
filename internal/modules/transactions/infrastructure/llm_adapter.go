package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

const maxLLMResponseBytes = 1 << 20

// LLMGenerator is the synchronous boundary implemented by the Gemini gateway.
// SDK types do not cross this interface.
type LLMGenerator interface {
	Generate(context.Context, string) (domain.LLMResponse, error)
}

type LLMAdapter struct{ generator LLMGenerator }

var _ domain.LLM = (*LLMAdapter)(nil)

func NewLLMAdapter(generator LLMGenerator) *LLMAdapter {
	return &LLMAdapter{generator: generator}
}

func (a *LLMAdapter) Generate(ctx context.Context, prompt string) (domain.LLMResponse, error) {
	if ctx == nil || strings.TrimSpace(prompt) == "" {
		return domain.LLMResponse{}, fmt.Errorf("invalid transaction LLM request: %w", apperrors.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return domain.LLMResponse{}, err
	}
	if a == nil || a.generator == nil {
		return domain.LLMResponse{}, fmt.Errorf("transaction LLM gateway is not configured: %w", apperrors.ErrInternal)
	}
	response, err := a.generator.Generate(ctx, prompt)
	if err != nil {
		return domain.LLMResponse{}, fmt.Errorf("generate transaction LLM response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.LLMResponse{}, err
	}
	if response.ToolCall != nil {
		call := response.ToolCall
		var args map[string]json.RawMessage
		if (call.Name != "record_transaction" && call.Name != "query_financial_history") || len(call.Arguments) > maxLLMResponseBytes || json.Unmarshal(call.Arguments, &args) != nil || args == nil {
			return domain.LLMResponse{}, fmt.Errorf("invalid transaction tool call: %w", apperrors.ErrInternal)
		}
		return domain.LLMResponse{ToolCall: &domain.LLMToolCall{Name: call.Name, Arguments: append(json.RawMessage(nil), call.Arguments...)}}, nil
	}
	if strings.TrimSpace(response.Text) == "" || len(response.Text) > maxLLMResponseBytes {
		return domain.LLMResponse{}, fmt.Errorf("invalid transaction LLM text: %w", apperrors.ErrInternal)
	}
	return domain.LLMResponse{Text: response.Text}, nil
}
