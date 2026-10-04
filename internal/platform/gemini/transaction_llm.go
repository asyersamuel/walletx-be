package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/genai"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

// TransactionLLM generates synchronous responses; only the agent executes tools.
type TransactionLLM struct {
	client *genai.Client
	model  string
}

var _ domain.LLM = (*TransactionLLM)(nil)

func NewTransactionLLM(client *genai.Client, model string) *TransactionLLM {
	return &TransactionLLM{client: client, model: strings.TrimSpace(model)}
}

// TransactionLLM shares the SDK client owned by the embedding integration.
func (c *EmbeddingClient) TransactionLLM(model string) *TransactionLLM {
	if c == nil {
		return NewTransactionLLM(nil, model)
	}
	return NewTransactionLLM(c.client, model)
}

func transactionFunctions() []*genai.FunctionDeclaration {
	str := func(description string) *genai.Schema {
		return &genai.Schema{Type: genai.TypeString, Description: description}
	}
	return []*genai.FunctionDeclaration{
		{Name: "record_transaction", Description: "Record a transaction explicitly requested by the user. Do not invent missing details.", Parameters: &genai.Schema{
			Type: genai.TypeObject, Required: []string{"merchant_name", "amount", "transaction_date", "category"},
			Properties: map[string]*genai.Schema{
				"merchant_name": str("Merchant name"), "amount": {Type: genai.TypeNumber, Description: "Strictly positive amount"},
				"transaction_date": str("RFC3339 timestamp with explicit timezone"), "category": str("Exact user category name from trusted context"), "notes": str("Optional notes"),
			},
		}},
		{Name: "query_financial_history", Description: "Search financial history; semantic results are not exhaustive ledger totals.", Parameters: &genai.Schema{
			Type: genai.TypeObject, Required: []string{"question"}, Properties: map[string]*genai.Schema{
				"question": str("Financial history search question"), "match_threshold": {Type: genai.TypeNumber},
				"start_date": str("Optional inclusive RFC3339 timestamp"), "end_date": str("Optional inclusive RFC3339 timestamp"), "limit": {Type: genai.TypeInteger},
			},
		}},
	}
}

func (g *TransactionLLM) Generate(ctx context.Context, prompt string) (domain.LLMResponse, error) {
	if ctx == nil || strings.TrimSpace(prompt) == "" {
		return domain.LLMResponse{}, fmt.Errorf("transaction generation requires context and prompt: %w", apperrors.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return domain.LLMResponse{}, err
	}
	if g == nil || g.client == nil || g.client.Models == nil || g.model == "" {
		return domain.LLMResponse{}, fmt.Errorf("transaction Gemini gateway is not configured: %w", apperrors.ErrInternal)
	}
	functions := transactionFunctions()
	response, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(prompt), &genai.GenerateContentConfig{
		CandidateCount:    1,
		SystemInstruction: genai.NewContentFromText("You are WalletX's transaction assistant. Use at most one declared function per request. Never claim an action has been executed. If required details, a category, or relative-date context are missing, ask for clarification instead of inventing them.", genai.RoleUser),
		Tools:             []*genai.Tool{{FunctionDeclarations: functions}},
		ToolConfig:        &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto}},
	})
	if err != nil {
		if ctx.Err() != nil {
			return domain.LLMResponse{}, ctx.Err()
		}
		return domain.LLMResponse{}, providerFailure("generate transaction tool response", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.LLMResponse{}, err
	}
	return parseTransactionResponse(response, functions)
}

func parseTransactionResponse(response *genai.GenerateContentResponse, functions []*genai.FunctionDeclaration) (domain.LLMResponse, error) {
	invalid := func() (domain.LLMResponse, error) {
		return domain.LLMResponse{}, fmt.Errorf("invalid transaction Gemini response: %w", apperrors.ErrInternal)
	}
	if response == nil || len(response.Candidates) != 1 || (response.PromptFeedback != nil && response.PromptFeedback.BlockReason != "" && response.PromptFeedback.BlockReason != "BLOCK_REASON_UNSPECIFIED") {
		return invalid()
	}
	candidate := response.Candidates[0]
	if candidate == nil || candidate.Content == nil || candidate.Content.Role != "model" || candidate.FinishReason != genai.FinishReasonStop {
		return invalid()
	}
	var text strings.Builder
	var call *domain.LLMToolCall
	for _, part := range candidate.Content.Parts {
		if part == nil {
			return invalid()
		}
		if part.Thought {
			if part.FunctionCall != nil {
				return invalid()
			}
			continue
		}
		if part.FunctionCall != nil {
			if call != nil {
				return invalid()
			}
			var schema *genai.Schema
			for _, f := range functions {
				if f.Name == part.FunctionCall.Name {
					schema = f.Parameters
					break
				}
			}
			if schema == nil || !validTransactionArguments(part.FunctionCall.Args, schema) {
				return invalid()
			}
			arguments, err := json.Marshal(part.FunctionCall.Args)
			if err != nil || len(arguments) > maxChatResponseBytes {
				return invalid()
			}
			call = &domain.LLMToolCall{Name: part.FunctionCall.Name, Arguments: arguments}
		}
		if len(part.Text) > maxChatResponseBytes-text.Len() {
			return invalid()
		}
		text.WriteString(part.Text)
	}
	if call != nil {
		return domain.LLMResponse{ToolCall: call}, nil
	}
	if strings.TrimSpace(text.String()) == "" {
		return invalid()
	}
	return domain.LLMResponse{Text: text.String()}, nil
}

func validTransactionArguments(args map[string]any, schema *genai.Schema) bool {
	for _, key := range schema.Required {
		if _, ok := args[key]; !ok {
			return false
		}
	}
	for key, value := range args {
		field, ok := schema.Properties[key]
		if !ok {
			return false
		}
		switch field.Type {
		case genai.TypeString:
			str, ok := value.(string)
			if !ok || (key != "notes" && strings.TrimSpace(str) == "") {
				return false
			}
			if key == "transaction_date" || key == "start_date" || key == "end_date" {
				if _, err := time.Parse(time.RFC3339, str); err != nil {
					return false
				}
			}
		case genai.TypeNumber, genai.TypeInteger:
			number, ok := value.(float64)
			if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
				return false
			}
			if field.Type == genai.TypeInteger && math.Trunc(number) != number {
				return false
			}
			if key == "amount" && number <= 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
