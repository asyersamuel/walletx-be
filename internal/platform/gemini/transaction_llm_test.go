package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
	apperrors "walletx-be/internal/shared/errors"
)

func TestTransactionLLMGenerate(t *testing.T) {
	for _, tc := range []struct {
		name, parts, tool, text string
		invalid                 bool
	}{
		{"record", `[{"functionCall":{"name":"record_transaction","args":{"merchant_name":"Coffee","amount":25000,"transaction_date":"2026-10-04T00:00:00+07:00","category":"Food","notes":"Morning coffee"}}}]`, "record_transaction", "", false},
		{"history", `[{"functionCall":{"name":"query_financial_history","args":{"question":"coffee","match_threshold":0.5,"limit":10,"start_date":"2026-10-01T00:00:00Z","end_date":"2026-10-04T00:00:00Z"}}}]`, "query_financial_history", "", false},
		{"history required only", `[{"functionCall":{"name":"query_financial_history","args":{"question":"coffee"}}}]`, "query_financial_history", "", false},
		{"plain text", `[{"thought":true,"text":"private"},{"text":"Hello"},{"text":" world"}]`, "", "Hello world", false},
		{"unknown tool", `[{"functionCall":{"name":"delete_all","args":{}}}]`, "", "", true},
		{"missing argument", `[{"functionCall":{"name":"record_transaction","args":{"amount":25}}}]`, "", "", true},
		{"wrong type", `[{"functionCall":{"name":"query_financial_history","args":{"question":"coffee","limit":"10"}}}]`, "", "", true},
		{"fractional limit", `[{"functionCall":{"name":"query_financial_history","args":{"question":"coffee","limit":1.5}}}]`, "", "", true},
		{"invalid date", `[{"functionCall":{"name":"query_financial_history","args":{"question":"coffee","start_date":"today"}}}]`, "", "", true},
		{"unexpected field", `[{"functionCall":{"name":"query_financial_history","args":{"question":"coffee","user_id":"foreign"}}}]`, "", "", true},
		{"multiple calls", `[{"functionCall":{"name":"query_financial_history","args":{"question":"coffee"}}},{"functionCall":{"name":"query_financial_history","args":{"question":"tea"}}}]`, "", "", true},
		{"thought only", `[{"thought":true,"text":"private"}]`, "", "", true},
		{"empty parts", `[]`, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/v1beta/models/transaction-test:generateContent", r.URL.Path)
				require.Equal(t, http.MethodPost, r.Method)
				var body struct {
					Contents   []*genai.Content  `json:"contents"`
					Tools      []*genai.Tool     `json:"tools"`
					ToolConfig *genai.ToolConfig `json:"toolConfig"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "hello", body.Contents[0].Parts[0].Text)
				require.Len(t, body.Tools, 1)
				require.Len(t, body.Tools[0].FunctionDeclarations, 2)
				record := body.Tools[0].FunctionDeclarations[0]
				require.Equal(t, "record_transaction", record.Name)
				require.ElementsMatch(t, []string{"merchant_name", "amount", "transaction_date", "category"}, record.Parameters.Required)
				require.Len(t, record.Parameters.Properties, 5)
				require.Equal(t, genai.TypeNumber, record.Parameters.Properties["amount"].Type)
				require.Equal(t, genai.TypeString, record.Parameters.Properties["notes"].Type)
				history := body.Tools[0].FunctionDeclarations[1]
				require.Equal(t, "query_financial_history", history.Name)
				require.Equal(t, []string{"question"}, history.Parameters.Required)
				require.Len(t, history.Parameters.Properties, 5)
				require.Equal(t, genai.TypeInteger, history.Parameters.Properties["limit"].Type)
				require.Equal(t, transactionFunctions(), body.Tools[0].FunctionDeclarations)
				require.Equal(t, genai.FunctionCallingConfigModeAuto, body.ToolConfig.FunctionCallingConfig.Mode)
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":` + tc.parts + `}}]}`))
				require.NoError(t, err)
			}))
			defer server.Close()
			client, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: "test-key", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"}})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			got, err := NewTransactionLLM(client, "transaction-test").Generate(ctx, "hello")
			require.Equal(t, 1, calls)
			if tc.invalid {
				require.ErrorIs(t, err, apperrors.ErrInternal)
				require.Empty(t, got.Text)
				require.Nil(t, got.ToolCall)
				return
			}
			require.NoError(t, err)
			if tc.tool == "" {
				require.Equal(t, tc.text, got.Text)
				require.Nil(t, got.ToolCall)
				return
			}
			require.NotNil(t, got.ToolCall)
			require.Equal(t, tc.tool, got.ToolCall.Name)
			var fixture []struct {
				FunctionCall *genai.FunctionCall `json:"functionCall"`
			}
			require.NoError(t, json.Unmarshal([]byte(tc.parts), &fixture))
			wantArgs, err := json.Marshal(fixture[0].FunctionCall.Args)
			require.NoError(t, err)
			require.JSONEq(t, string(wantArgs), string(got.ToolCall.Arguments))
		})
	}
}

func TestTransactionLLMInvalidResponses(t *testing.T) {
	for _, response := range []*genai.GenerateContentResponse{
		nil, {}, {Candidates: []*genai.Candidate{nil}},
		{PromptFeedback: &genai.GenerateContentResponsePromptFeedback{BlockReason: genai.BlockedReasonSafety}},
		{Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonMaxTokens, Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "partial"}}}}}},
		{Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonStop, Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: strings.Repeat("x", maxChatResponseBytes+1)}}}}}},
	} {
		_, err := parseTransactionResponse(response, transactionFunctions())
		require.ErrorIs(t, err, apperrors.ErrInternal)
	}
}

func TestTransactionLLMConfigurationAndCancellation(t *testing.T) {
	_, err := NewTransactionLLM(nil, "test").Generate(context.Background(), "hello")
	require.ErrorIs(t, err, apperrors.ErrInternal)
	_, err = NewTransactionLLM(nil, "test").Generate(nil, "hello")
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	_, err = NewTransactionLLM(nil, "test").Generate(context.Background(), " ")
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewTransactionLLM(nil, "test").Generate(ctx, "hello")
	require.ErrorIs(t, err, context.Canceled)
}

func TestTransactionLLMProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"private prompt"}}`))
	}))
	defer server.Close()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: "test", HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL}})
	require.NoError(t, err)
	_, err = NewTransactionLLM(client, "test").Generate(context.Background(), "hello")
	require.ErrorIs(t, err, apperrors.ErrInternal)
	var apiErr genai.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 400, apiErr.Code)
	require.NotContains(t, err.Error(), "private prompt")
}

func TestTransactionLLMCancelDuringRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer server.Close()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: "test", HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL}})
	require.NoError(t, err)
	_, err = NewTransactionLLM(client, "test").Generate(ctx, "hello")
	require.ErrorIs(t, err, context.Canceled)
}
