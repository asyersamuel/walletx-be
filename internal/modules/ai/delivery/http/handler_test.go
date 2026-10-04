package aihttp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/ai"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
)

type stubService struct {
	chat    func(context.Context, ai.ChatRequest) (ai.ChatStream, error)
	extract func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error)
}

func (s stubService) Chat(ctx context.Context, r ai.ChatRequest) (ai.ChatStream, error) {
	return s.chat(ctx, r)
}
func (s stubService) ExtractTransaction(ctx context.Context, r ai.ExtractionInput) (ai.ExtractedTransaction, error) {
	return s.extract(ctx, r)
}

type nopLogger struct{}

func (nopLogger) Info(string)                                       {}
func (nopLogger) Warn(string)                                       {}
func (nopLogger) Error(string)                                      {}
func (nopLogger) Debug(string)                                      {}
func (l nopLogger) WithField(string, interface{}) logger.Logger     { return l }
func (l nopLogger) WithError(error) logger.Logger                   { return l }
func (l nopLogger) WithFields(map[string]interface{}) logger.Logger { return l }

type validator struct{ id string }

func (v validator) ValidateToken(context.Context, string) (*middleware.TokenClaims, error) {
	return &middleware.TokenClaims{UserID: v.id}, nil
}
func router(s ai.AIService, id string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1", middleware.AuthMiddleware(validator{id}))
	RegisterRoutes(group, NewHandler(s, nopLogger{}))
	return r
}
func request(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer test-token")
	return r
}

func TestChatSSE(t *testing.T) {
	id := uuid.New()
	message := &ai.ChatMessage{Role: ai.ChatRoleAssistant, Parts: []ai.ChatPart{{Text: "**answer**\nnext", ThoughtSignature: []byte{1, 2}}}}
	events := []ai.ChatEvent{{Kind: ai.ChatEventThought, Text: "thinking"}, {Kind: ai.ChatEventAnswer, Text: "**answer**\nnext"}, {Kind: ai.ChatEventComplete, Message: message}, {Kind: ai.ChatEventAnswer, Text: "must not send"}}
	var serviceCtx context.Context
	s := stubService{chat: func(ctx context.Context, r ai.ChatRequest) (ai.ChatStream, error) {
		require.Equal(t, id, r.UserID)
		require.True(t, r.Personalized)
		require.True(t, r.IsThinking)
		require.Len(t, r.History, 1)
		serviceCtx = ctx
		ch := make(chan ai.ChatEvent, len(events))
		for _, event := range events {
			ch <- event
		}
		close(ch)
		return ch, nil
	}}
	w := httptest.NewRecorder()
	router(s, id.String()).ServeHTTP(w, request("POST", "/api/v1/ai/chat", `{"prompt":"hello","personalized":true,"isThinking":true,"history":[{"role":"user","parts":[{"text":"previous"}]}]}`))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	require.Equal(t, "keep-alive", w.Header().Get("Connection"))
	require.True(t, w.Flushed)
	frames := strings.Split(strings.TrimSpace(w.Body.String()), "\n\n")
	require.Len(t, frames, 3)
	require.Contains(t, frames[0], "event: thought")
	require.Contains(t, frames[1], `data: {"text":"**answer**\nnext"}`)
	var final struct {
		Message ai.ChatMessage `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.SplitN(frames[2], "data: ", 2)[1]), &final))
	require.Equal(t, *message, final.Message)
	require.ErrorIs(t, serviceCtx.Err(), context.Canceled)
}

func TestChatFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body  string
		err         error
		events      []ai.ChatEvent
		status      int
		streamError bool
	}{
		{name: "invalid JSON", body: `{`, status: 400},
		{name: "identity injection", body: `{"prompt":"x","user_id":"foreign"}`, status: 400},
		{name: "unknown config", body: `{"prompt":"x","matchCount":100}`, status: 400},
		{name: "trailing JSON", body: `{"prompt":"x"}{}`, status: 400},
		{name: "oversized JSON", body: `{"prompt":"` + strings.Repeat("x", maxJSONBytes) + `"}`, status: 413},
		{name: "validation", body: `{"prompt":"x"}`, err: apperrors.ErrInvalidInput, status: 400},
		{name: "setup failure", body: `{"prompt":"x"}`, err: errors.New("secret provider payload"), status: 500},
		{name: "nil stream", body: `{"prompt":"x"}`, status: 500},
		{name: "stream failure", body: `{"prompt":"x"}`, events: []ai.ChatEvent{{Kind: ai.ChatEventError, Err: errors.New("secret provider payload")}}, status: 200, streamError: true},
		{name: "premature EOF", body: `{"prompt":"x"}`, events: []ai.ChatEvent{}, status: 200, streamError: true},
		{name: "malformed complete", body: `{"prompt":"x"}`, events: []ai.ChatEvent{{Kind: ai.ChatEventComplete}}, status: 200, streamError: true},
		{name: "unknown event", body: `{"prompt":"x"}`, events: []ai.ChatEvent{{Kind: "bad\nevent"}}, status: 200, streamError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			s := stubService{chat: func(context.Context, ai.ChatRequest) (ai.ChatStream, error) {
				called = true
				if tc.events == nil {
					return nil, tc.err
				}
				ch := make(chan ai.ChatEvent, len(tc.events))
				for _, e := range tc.events {
					ch <- e
				}
				close(ch)
				return ch, nil
			}}
			w := httptest.NewRecorder()
			router(s, uuid.NewString()).ServeHTTP(w, request("POST", "/api/v1/ai/chat", tc.body))
			require.Equal(t, tc.status, w.Code)
			require.NotContains(t, w.Body.String(), "secret provider payload")
			if tc.streamError {
				require.Contains(t, w.Body.String(), "event: error")
				require.NotContains(t, w.Body.String(), "event: complete")
			} else {
				require.NotEqual(t, "text/event-stream", w.Header().Get("Content-Type"))
			}
			if tc.status == 413 || tc.name == "invalid JSON" || tc.name == "identity injection" || tc.name == "unknown config" || tc.name == "trailing JSON" {
				require.False(t, called)
			}
		})
	}
}

func TestAuthentication(t *testing.T) {
	for _, path := range []string{"/api/v1/ai/chat", "/api/v1/ai/extract"} {
		for _, identity := range []string{"", "invalid", uuid.Nil.String()} {
			w := httptest.NewRecorder()
			router(stubService{}, identity).ServeHTTP(w, request("POST", path, `{}`))
			require.Equal(t, 401, w.Code)
		}
		req := request("POST", path, `{}`)
		req.Header.Del("Authorization")
		w := httptest.NewRecorder()
		router(stubService{}, uuid.NewString()).ServeHTTP(w, req)
		require.Equal(t, 401, w.Code)
	}
}

func TestChatFlushAndDisconnect(t *testing.T) {
	canceled := make(chan struct{})
	s := stubService{chat: func(ctx context.Context, _ ai.ChatRequest) (ai.ChatStream, error) {
		ch := make(chan ai.ChatEvent)
		go func() {
			defer close(ch)
			select {
			case ch <- ai.ChatEvent{Kind: ai.ChatEventAnswer, Text: "first"}:
			case <-ctx.Done():
			}
			<-ctx.Done()
			close(canceled)
		}()
		return ch, nil
	}}
	server := httptest.NewServer(router(s, uuid.NewString()))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/v1/ai/chat", strings.NewReader(`{"prompt":"x"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	res, err := server.Client().Do(req)
	require.NoError(t, err)
	reader := bufio.NewReader(res.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "event: answer\n", line)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, line, `"first"`)
	res.Body.Close()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not cancel service")
	}
}

func TestChatCancellationWithIdleStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s := stubService{chat: func(context.Context, ai.ChatRequest) (ai.ChatStream, error) {
		cancel()
		return make(chan ai.ChatEvent), nil
	}}
	r := router(s, uuid.NewString())
	w := httptest.NewRecorder()
	go func() {
		defer close(done)
		r.ServeHTTP(w, request("POST", "/api/v1/ai/chat", `{"prompt":"x"}`).WithContext(ctx))
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler waits on idle stream after cancellation")
	}
	require.NotContains(t, w.Body.String(), "event: error")
}

func TestExtractionJSON(t *testing.T) {
	id := uuid.New()
	category := uuid.New()
	s := stubService{extract: func(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		require.Equal(t, id, input.UserID)
		require.Equal(t, "paid coffee today", input.Text)
		require.Equal(t, "Europe/London", input.TimeZone)
		require.WithinDuration(t, time.Now(), input.ReferenceTime, time.Second)
		require.Empty(t, input.Image)
		return ai.ExtractedTransaction{Amount: 5, Merchant: "Coffee", CategoryID: category, Date: "2026-10-03"}, nil
	}}
	w := httptest.NewRecorder()
	router(s, id.String()).ServeHTTP(w, request("POST", "/api/v1/ai/extract", `{"text":"paid coffee today","timeZone":"Europe/London"}`))
	require.Equal(t, 200, w.Code)
	var body struct {
		Data ai.ExtractedTransaction `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, category, body.Data.CategoryID)
}

func upload(t *testing.T, images int, size int, text string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for i := 0; i < images; i++ {
		part, err := writer.CreateFormFile("image", "receipt.png")
		require.NoError(t, err)
		_, err = part.Write(append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{'x'}, size)...))
		require.NoError(t, err)
	}
	require.NoError(t, writer.WriteField("timeZone", "Asia/Jakarta"))
	if text != "" {
		require.NoError(t, writer.WriteField("text", text))
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest("POST", "/api/v1/ai/extract", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test")
	return req
}

func TestExtractionUpload(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		images, size, status int
		text                 string
	}{
		{name: "image", images: 1, size: 10, status: 200},
		{name: "both inputs", images: 1, size: 10, text: "paid coffee", status: 400},
		{name: "missing file", status: 400},
		{name: "duplicate image", images: 2, size: 10, status: 400},
		{name: "image limit", images: 1, size: maxImageBytes, status: 413},
		{name: "body limit", images: 1, size: maxImageBytes + maxJSONBytes, status: 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			s := stubService{extract: func(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
				called = true
				require.Equal(t, "image/png", input.MIMEType)
				require.NotEmpty(t, input.Image)
				require.Equal(t, "Asia/Jakarta", input.TimeZone)
				if input.Text != "" {
					return ai.ExtractedTransaction{}, apperrors.ErrInvalidInput
				}
				return ai.ExtractedTransaction{}, nil
			}}
			w := httptest.NewRecorder()
			router(s, uuid.NewString()).ServeHTTP(w, upload(t, tc.images, tc.size, tc.text))
			require.Equal(t, tc.status, w.Code)
			if tc.status != 200 && tc.name != "both inputs" {
				require.False(t, called)
			}
		})
	}
}

func TestExtractionErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
		err                     error
		status                  int
	}{
		{name: "invalid JSON", body: `{`, contentType: "application/json", status: 400},
		{name: "identity override", body: `{"text":"x","userID":"foreign"}`, contentType: "application/json", status: 400},
		{name: "unsupported", body: "text", contentType: "text/plain", status: 415},
		{name: "bad multipart", body: "bad", contentType: "multipart/form-data", status: 400},
		{name: "validation", body: `{"text":"x"}`, contentType: "application/json", err: apperrors.ErrInvalidInput, status: 400},
		{name: "provider", body: `{"text":"x"}`, contentType: "application/json", err: errors.New("private receipt"), status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := stubService{extract: func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error) {
				return ai.ExtractedTransaction{}, tc.err
			}}
			req := request("POST", "/api/v1/ai/extract", tc.body)
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			router(s, uuid.NewString()).ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.NotContains(t, w.Body.String(), "private receipt")
		})
	}
}

type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header     { return w.header }
func (*failingWriter) WriteHeader(int)           {}
func (*failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (*failingWriter) Flush()                    {}
func TestChatWriteFailureCancelsService(t *testing.T) {
	var ctx context.Context
	s := stubService{chat: func(c context.Context, _ ai.ChatRequest) (ai.ChatStream, error) {
		ctx = c
		ch := make(chan ai.ChatEvent, 1)
		ch <- ai.ChatEvent{Kind: ai.ChatEventAnswer, Text: "answer"}
		return ch, nil
	}}
	router(s, uuid.NewString()).ServeHTTP(&failingWriter{header: make(http.Header)}, request("POST", "/api/v1/ai/chat", `{"prompt":"x"}`))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}
