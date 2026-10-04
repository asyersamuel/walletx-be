package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

type llmGeneratorFunc func(context.Context, string) (domain.LLMResponse, error)

func (f llmGeneratorFunc) Generate(ctx context.Context, prompt string) (domain.LLMResponse, error) {
	return f(ctx, prompt)
}

func TestLLMAdapterGenerate(t *testing.T) {
	cause := errors.New("provider failed")
	for _, tc := range []struct {
		name        string
		response    domain.LLMResponse
		cause, want error
	}{
		{name: "text", response: domain.LLMResponse{Text: "**Answer**"}},
		{name: "record", response: domain.LLMResponse{Text: "ignored", ToolCall: &domain.LLMToolCall{Name: "record_transaction", Arguments: json.RawMessage(`{"amount":25000}`)}}},
		{name: "history", response: domain.LLMResponse{ToolCall: &domain.LLMToolCall{Name: "query_financial_history", Arguments: json.RawMessage(`{"question":"coffee"}`)}}},
		{name: "failure", cause: cause, want: cause},
		{name: "empty", want: apperrors.ErrInternal},
		{name: "oversized", response: domain.LLMResponse{Text: strings.Repeat("x", maxLLMResponseBytes+1)}, want: apperrors.ErrInternal},
		{name: "unknown tool", response: domain.LLMResponse{ToolCall: &domain.LLMToolCall{Name: "delete_all", Arguments: json.RawMessage(`{}`)}}, want: apperrors.ErrInternal},
		{name: "invalid JSON", response: domain.LLMResponse{ToolCall: &domain.LLMToolCall{Name: "record_transaction", Arguments: json.RawMessage(`{`)}}, want: apperrors.ErrInternal},
		{name: "array arguments", response: domain.LLMResponse{ToolCall: &domain.LLMToolCall{Name: "record_transaction", Arguments: json.RawMessage(`[]`)}}, want: apperrors.ErrInternal},
		{name: "null arguments", response: domain.LLMResponse{ToolCall: &domain.LLMToolCall{Name: "record_transaction", Arguments: json.RawMessage(`null`)}}, want: apperrors.ErrInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			calls := 0
			generator := llmGeneratorFunc(func(got context.Context, prompt string) (domain.LLMResponse, error) {
				calls++
				require.Equal(t, ctx, got)
				require.Equal(t, "hello", prompt)
				return tc.response, tc.cause
			})
			got, err := NewLLMAdapter(generator).Generate(ctx, "hello")
			require.Equal(t, 1, calls)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				require.Equal(t, domain.LLMResponse{}, got)
				return
			}
			require.NoError(t, err)
			if tc.response.ToolCall == nil {
				require.Equal(t, tc.response.Text, got.Text)
				return
			}
			require.Empty(t, got.Text)
			require.Equal(t, tc.response.ToolCall, got.ToolCall)
			got.ToolCall.Arguments[0] = 'x'
			require.Equal(t, byte('{'), tc.response.ToolCall.Arguments[0])
		})
	}
}

func TestLLMAdapterValidationAndCancellation(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		prompt string
		want   error
	}{
		{"nil context", nil, "hello", apperrors.ErrInvalidInput},
		{"empty prompt", context.Background(), " \n", apperrors.ErrInvalidInput},
		{"cancelled", cancelled, "hello", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generator := llmGeneratorFunc(func(context.Context, string) (domain.LLMResponse, error) {
				t.Fatal("unexpected generation")
				return domain.LLMResponse{}, nil
			})
			_, err := NewLLMAdapter(generator).Generate(tc.ctx, tc.prompt)
			require.ErrorIs(t, err, tc.want)
		})
	}
	_, err := NewLLMAdapter(nil).Generate(context.Background(), "hello")
	require.ErrorIs(t, err, apperrors.ErrInternal)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator := llmGeneratorFunc(func(context.Context, string) (domain.LLMResponse, error) {
		cancel()
		return domain.LLMResponse{Text: "answer"}, nil
	})
	_, err = NewLLMAdapter(generator).Generate(ctx, "hello")
	require.ErrorIs(t, err, context.Canceled)
}
