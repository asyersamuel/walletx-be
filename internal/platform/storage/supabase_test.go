package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

func TestSupabaseReadReceipt(t *testing.T) {
	for _, image := range [][]byte{
		[]byte("\x89PNG\r\n\x1a\n"), []byte("\xff\xd8\xff"), []byte("GIF89a"), []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
	} {
		requests := make(chan supabaseRequest, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests <- captureSupabaseRequest(r)
			// Octet-stream must not prevent identifying a supported image by bytes.
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(image)
		}))
		store := newTestSupabase(t, server)
		owner := uuid.New()
		reference := testReceiptReference(owner)
		got, err := store.ReadReceipt(context.Background(), owner, reference)
		server.Close()
		require.NoError(t, err)
		require.Equal(t, image, got)
		request := <-requests
		require.NoError(t, request.err)
		require.Equal(t, http.MethodGet, request.method)
		require.Equal(t, "/storage/v1/object/authenticated/receipts/"+reference, request.path)
		require.Empty(t, request.query)
		require.Empty(t, request.body)
		require.Equal(t, "Bearer "+testSupabaseKey, request.headers.Get("Authorization"))
		require.Equal(t, testSupabaseKey, request.headers.Get("apikey"))
	}
}

func TestSupabaseReadInvalidReferencesNeverSend(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	t.Cleanup(server.Close)
	store := newTestSupabase(t, server)
	owner := uuid.New()
	ref := testReceiptReference(owner)
	for _, invalid := range []string{
		"", testReceiptReference(uuid.New()), "https://example.com/" + ref,
		owner.String() + "/../" + ref, ref + "?token=secret", ref + "#fragment",
		strings.Replace(ref, ".png", ".exe", 1), strings.Replace(ref, "1791334800000", "0", 1),
		strings.Replace(ref, "1791334800000", "01791334800000", 1),
		owner.String() + "/" + uuid.Nil.String() + "_123.png",
	} {
		got, err := store.ReadReceipt(context.Background(), owner, invalid)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
		require.Nil(t, got)
	}
	_, err := store.ReadReceipt(nil, owner, ref)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	_, err = store.ReadReceipt(context.Background(), uuid.Nil, ref)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.ReadReceipt(ctx, owner, ref)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls.Load())
}

func TestSupabaseReadResponseValidationAndBodyLimits(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		image  []byte
		valid  bool
	}{
		{"empty", 200, nil, false},
		{"HTML pretending to be image", 200, []byte("<html>sensitive receipt</html>"), false},
		{"at size limit", 200, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, domain.MaxReceiptBytes-8)...), true},
		{"oversized", 200, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, domain.MaxReceiptBytes+100)...), false},
		{"partial content", 206, []byte("\x89PNG\r\n\x1a\n"), false},
		{"missing", 404, []byte("sensitive " + testSupabaseKey), false},
		{"unauthorized", 401, []byte("sensitive " + testSupabaseKey), false},
		{"forbidden", 403, []byte("sensitive " + testSupabaseKey), false},
		{"rate limited", 429, []byte("sensitive " + testSupabaseKey), false},
		{"provider failure", 500, bytes.Repeat([]byte("sensitive"), 20000), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedSupabaseBody{reader: bytes.NewReader(tc.image)}
			client := &http.Client{Timeout: time.Second, Transport: supabaseRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"image/png"}}, Body: body}, nil
			})}
			store, err := NewSupabase("https://project.supabase.co", testSupabaseKey, client)
			require.NoError(t, err)
			owner := uuid.New()
			got, err := store.ReadReceipt(context.Background(), owner, testReceiptReference(owner))
			require.True(t, body.closed)
			require.LessOrEqual(t, body.read, domain.MaxReceiptBytes+1)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, tc.image, got)
			} else {
				require.Error(t, err)
				require.Nil(t, got)
				require.NotContains(t, err.Error(), testSupabaseKey)
				require.NotContains(t, err.Error(), "sensitive")
				if tc.status != http.StatusOK {
					require.Contains(t, err.Error(), strconv.Itoa(tc.status))
					require.LessOrEqual(t, body.read, maxSupabaseResponseDrain)
				} else {
					require.ErrorIs(t, err, apperrors.ErrInternal)
				}
			}
		})
	}
}

type failingReceiptBody struct{ err error }

func (b failingReceiptBody) Read([]byte) (int, error) { return 0, b.err }

func TestSupabaseReadTransportAndBodyErrors(t *testing.T) {
	for _, bodyFailure := range []bool{false, true} {
		cause := errors.New("sensitive transport or body details")
		body := &trackedSupabaseBody{reader: failingReceiptBody{err: cause}}
		client := &http.Client{Timeout: time.Second, Transport: supabaseRoundTripper(func(*http.Request) (*http.Response, error) {
			if !bodyFailure {
				return nil, cause
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
		})}
		store, err := NewSupabase("https://project.supabase.co", testSupabaseKey, client)
		require.NoError(t, err)
		owner := uuid.New()
		got, err := store.ReadReceipt(context.Background(), owner, testReceiptReference(owner))
		require.Nil(t, got)
		require.ErrorIs(t, err, cause)
		require.NotContains(t, err.Error(), "sensitive")
		if bodyFailure {
			require.True(t, body.closed)
		}
	}
}

func TestSupabaseReadTimeoutAndCancellation(t *testing.T) {
	for _, cancellation := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancellation), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			t.Cleanup(server.Close)
			t.Cleanup(func() { close(release) })
			client := server.Client()
			client.Timeout = 100 * time.Millisecond
			if cancellation {
				client.Timeout = 5 * time.Second
			}
			store, err := NewSupabase(server.URL, testSupabaseKey, client)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancellation {
				go func() {
					select {
					case <-started:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			owner := uuid.New()
			got, err := store.ReadReceipt(ctx, owner, testReceiptReference(owner))
			require.Nil(t, got)
			want := context.DeadlineExceeded
			if cancellation {
				want = context.Canceled
			}
			require.ErrorIs(t, err, want)
		})
	}
}

const testSupabaseKey = "dummy-service-role-key"

type supabaseRequest struct {
	method, path, query string
	headers             http.Header
	body                []byte
	err                 error
}

func captureSupabaseRequest(r *http.Request) supabaseRequest {
	body, err := io.ReadAll(r.Body)
	return supabaseRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body, err}
}

func newTestSupabase(t *testing.T, server *httptest.Server) *Supabase {
	t.Helper()
	client := server.Client()
	client.Timeout = time.Second
	store, err := NewSupabase(server.URL+"/", testSupabaseKey, client)
	require.NoError(t, err)
	return store
}

func testReceiptReference(owner uuid.UUID) string {
	return owner.String() + "/" + uuid.NewString() + "_1791334800000.png"
}

func TestSupabaseConstructor(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	for _, tc := range []struct {
		name, url, key string
		client         *http.Client
	}{
		{"empty URL", "", testSupabaseKey, client},
		{"relative URL", "project.supabase.co", testSupabaseKey, client},
		{"malformed URL", "https://%", testSupabaseKey, client},
		{"unsupported scheme", "ftp://project.supabase.co", testSupabaseKey, client},
		{"insecure remote", "http://project.supabase.co", testSupabaseKey, client},
		{"URL credentials", "https://secret@project.supabase.co", testSupabaseKey, client},
		{"URL query", "https://project.supabase.co?secret=value", testSupabaseKey, client},
		{"empty query", "https://project.supabase.co?", testSupabaseKey, client},
		{"URL fragment", "https://project.supabase.co#secret", testSupabaseKey, client},
		{"URL path", "https://project.supabase.co/storage/v1", testSupabaseKey, client},
		{"empty key", "https://project.supabase.co", " ", client},
		{"invalid key", "https://project.supabase.co", "key\r\nInjected: value", client},
		{"nil client", "https://project.supabase.co", testSupabaseKey, nil},
		{"unbounded client", "https://project.supabase.co", testSupabaseKey, &http.Client{}},
		{"negative timeout", "https://project.supabase.co", testSupabaseKey, &http.Client{Timeout: -time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewSupabase(tc.url, tc.key, tc.client)
			require.Nil(t, store)
			require.ErrorIs(t, err, apperrors.ErrInvalidInput)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "Injected")
		})
	}
	store, err := NewSupabase("https://project.supabase.co/", testSupabaseKey, client)
	require.NoError(t, err)
	require.NotNil(t, store)
}

func TestSupabaseSaveReceipt(t *testing.T) {
	requests := make(chan supabaseRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureSupabaseRequest(r)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"Key":"receipts/provider-response-must-not-control-reference"}`)
	}))
	t.Cleanup(server.Close)
	store := newTestSupabase(t, server)
	owner := uuid.New()
	image := []byte("receipt image bytes")
	for _, tc := range []struct{ mime, ext string }{
		{"image/png", ".png"}, {"image/jpeg", ".jpg"}, {"image/gif", ".gif"}, {"image/webp", ".webp"},
	} {
		t.Run(tc.mime, func(t *testing.T) {
			before := time.Now().UnixMilli()
			ref, err := store.SaveReceipt(context.Background(), owner, tc.mime, "../../private-client-name.exe", image)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(ref, owner.String()+"/"))
			object := strings.TrimPrefix(ref, owner.String()+"/")
			parts := strings.Split(strings.TrimSuffix(object, tc.ext), "_")
			require.Len(t, parts, 2)
			id, err := uuid.Parse(parts[0])
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, id)
			require.Equal(t, id.String(), parts[0])
			stamp, err := strconv.ParseInt(parts[1], 10, 64)
			require.NoError(t, err)
			require.GreaterOrEqual(t, stamp, before)
			require.LessOrEqual(t, stamp, time.Now().UnixMilli())
			require.True(t, strings.HasSuffix(ref, tc.ext))
			require.NotContains(t, ref, "private-client-name")
			require.NotContains(t, ref, "receipts/")
			require.NotContains(t, ref, "://")
			request := <-requests
			require.NoError(t, request.err)
			require.Equal(t, http.MethodPost, request.method)
			require.Equal(t, "/storage/v1/object/receipts/"+ref, request.path)
			require.Empty(t, request.query)
			require.Equal(t, "Bearer "+testSupabaseKey, request.headers.Get("Authorization"))
			require.Equal(t, testSupabaseKey, request.headers.Get("apikey"))
			require.Equal(t, tc.mime, request.headers.Get("Content-Type"))
			require.Equal(t, "false", request.headers.Get("x-upsert"))
			require.Equal(t, image, request.body)
			second, err := store.SaveReceipt(context.Background(), owner, tc.mime, "same.png", image)
			require.NoError(t, err)
			require.NotEqual(t, ref, second)
			<-requests
		})
	}
}

func TestSupabaseInvalidInputsDoNotSendRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	t.Cleanup(server.Close)
	store := newTestSupabase(t, server)
	owner := uuid.New()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx   context.Context
		owner uuid.UUID
		mime  string
		image []byte
		want  error
	}{
		{nil, owner, "image/png", []byte{1}, apperrors.ErrInvalidInput},
		{context.Background(), uuid.Nil, "image/png", []byte{1}, apperrors.ErrInvalidInput},
		{context.Background(), owner, "text/plain", []byte{1}, apperrors.ErrInvalidInput},
		{context.Background(), owner, "image/png", nil, apperrors.ErrInvalidInput},
		{context.Background(), owner, "image/png", make([]byte, domain.MaxReceiptBytes+1), apperrors.ErrInvalidInput},
		{canceled, owner, "image/png", []byte{1}, context.Canceled},
	} {
		ref, err := store.SaveReceipt(tc.ctx, tc.owner, tc.mime, "receipt.png", tc.image)
		require.Empty(t, ref)
		require.ErrorIs(t, err, tc.want)
	}
	valid := testReceiptReference(owner)
	for _, ref := range []string{
		"", "../file.png", "/" + valid, "receipts/" + valid, "https://example.com/" + valid,
		"receipt://" + valid, testReceiptReference(uuid.New()), owner.String() + "/../" + valid,
		strings.ReplaceAll(valid, "/", "\\"), valid + "?query=1", valid + "#fragment", valid + "/extra",
		owner.String() + "/not-a-uuid_123.png", owner.String() + "/" + uuid.Nil.String() + "_123.png",
		owner.String() + "/" + uuid.NewString() + "_-1.png", owner.String() + "/" + uuid.NewString() + "_001.png",
		owner.String() + "/" + uuid.NewString() + "_123.exe",
	} {
		require.ErrorIs(t, store.DeleteReceipt(context.Background(), owner, ref), apperrors.ErrInvalidInput, ref)
	}
	require.ErrorIs(t, store.DeleteReceipt(nil, owner, valid), apperrors.ErrInvalidInput)
	require.ErrorIs(t, store.DeleteReceipt(context.Background(), uuid.Nil, valid), apperrors.ErrInvalidInput)
	require.ErrorIs(t, store.DeleteReceipt(canceled, owner, valid), context.Canceled)
	require.Zero(t, calls.Load())
}

func TestSupabaseDeleteReceipt(t *testing.T) {
	requests := make(chan supabaseRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureSupabaseRequest(r)
		// Batch deletion succeeds with an empty list when the object is absent.
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)
	store := newTestSupabase(t, server)
	owner := uuid.New()
	ref := testReceiptReference(owner)
	for range 2 {
		require.NoError(t, store.DeleteReceipt(context.Background(), owner, ref))
		request := <-requests
		require.NoError(t, request.err)
		require.Equal(t, http.MethodDelete, request.method)
		require.Equal(t, "/storage/v1/object/receipts", request.path)
		require.Equal(t, "Bearer "+testSupabaseKey, request.headers.Get("Authorization"))
		require.Equal(t, testSupabaseKey, request.headers.Get("apikey"))
		require.Equal(t, "application/json", request.headers.Get("Content-Type"))
		var body map[string][]string
		require.NoError(t, json.Unmarshal(request.body, &body))
		require.Equal(t, map[string][]string{"prefixes": {ref}}, body)
	}
}

func TestSupabaseHTTPFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 413, 429, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, strings.Repeat("sensitive-provider-body "+testSupabaseKey, 4096))
			}))
			t.Cleanup(server.Close)
			store := newTestSupabase(t, server)
			owner := uuid.New()
			ref, saveErr := store.SaveReceipt(context.Background(), owner, "image/png", "receipt.png", []byte{1})
			require.Empty(t, ref)
			deleteErr := store.DeleteReceipt(context.Background(), owner, testReceiptReference(owner))
			for _, err := range []error{saveErr, deleteErr} {
				require.Error(t, err)
				require.Contains(t, err.Error(), strconv.Itoa(status))
				require.NotContains(t, err.Error(), testSupabaseKey)
				require.NotContains(t, err.Error(), "sensitive-provider-body")
			}
		})
	}
}

func TestSupabaseDoesNotDependOnResponseJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"created without body", http.StatusCreated, ""},
		{"malformed JSON", http.StatusOK, "{invalid-json"},
		{"HTML error", http.StatusBadGateway, "<html>sensitive error</html>"},
		{"empty error", http.StatusServiceUnavailable, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			store := newTestSupabase(t, server)
			owner := uuid.New()
			ref, saveErr := store.SaveReceipt(context.Background(), owner, "image/png", "receipt.png", []byte{1})
			deleteErr := store.DeleteReceipt(context.Background(), owner, testReceiptReference(owner))
			if tc.status < http.StatusMultipleChoices {
				require.NoError(t, saveErr)
				require.NotEmpty(t, ref)
				require.NoError(t, deleteErr)
			} else {
				require.Empty(t, ref)
				require.Error(t, saveErr)
				require.Error(t, deleteErr)
			}
		})
	}
}

type trackedSupabaseBody struct {
	reader io.Reader
	read   int
	closed bool
}

func (b *trackedSupabaseBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += n
	return n, err
}

func (b *trackedSupabaseBody) Close() error { b.closed = true; return nil }

func TestSupabaseClosesAndBoundsResponseBodies(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		for _, deletion := range []bool{false, true} {
			t.Run(strconv.Itoa(status)+"/delete="+strconv.FormatBool(deletion), func(t *testing.T) {
				body := &trackedSupabaseBody{reader: strings.NewReader(strings.Repeat("x", 1<<20))}
				client := &http.Client{Timeout: time.Second, Transport: supabaseRoundTripper(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}, nil
				})}
				store, err := NewSupabase("https://project.supabase.co", testSupabaseKey, client)
				require.NoError(t, err)
				owner := uuid.New()
				if deletion {
					err = store.DeleteReceipt(context.Background(), owner, testReceiptReference(owner))
				} else {
					_, err = store.SaveReceipt(context.Background(), owner, "image/png", "receipt.png", []byte{1})
				}
				if status == http.StatusOK {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				require.True(t, body.closed)
				require.LessOrEqual(t, body.read, 64<<10)
			})
		}
	}
}

func TestSupabaseTimeoutAndCancellation(t *testing.T) {
	for _, deletion := range []bool{false, true} {
		for _, cancellation := range []bool{false, true} {
			t.Run(strconv.FormatBool(deletion)+"/cancel="+strconv.FormatBool(cancellation), func(t *testing.T) {
				started := make(chan struct{})
				release := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					close(started)
					select {
					case <-r.Context().Done():
					case <-release:
					}
				}))
				t.Cleanup(server.Close)
				t.Cleanup(func() { close(release) })
				client := server.Client()
				client.Timeout = 100 * time.Millisecond
				if cancellation {
					client.Timeout = 5 * time.Second
				}
				store, err := NewSupabase(server.URL, testSupabaseKey, client)
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if cancellation {
					go func() {
						select {
						case <-started:
							cancel()
						case <-ctx.Done():
						}
					}()
				}
				owner := uuid.New()
				if deletion {
					err = store.DeleteReceipt(ctx, owner, testReceiptReference(owner))
				} else {
					var ref string
					ref, err = store.SaveReceipt(ctx, owner, "image/png", "receipt.png", []byte{1})
					require.Empty(t, ref)
				}
				want := context.DeadlineExceeded
				if cancellation {
					want = context.Canceled
				}
				require.ErrorIs(t, err, want)
			})
		}
	}
}

type supabaseRoundTripper func(*http.Request) (*http.Response, error)

func (f supabaseRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSupabaseTransportFailure(t *testing.T) {
	connectionErr := errors.New("connection failed with sensitive transport details")
	client := &http.Client{Timeout: time.Second, Transport: supabaseRoundTripper(func(*http.Request) (*http.Response, error) { return nil, connectionErr })}
	store, err := NewSupabase("https://project.supabase.co", testSupabaseKey, client)
	require.NoError(t, err)
	owner := uuid.New()
	ref, saveErr := store.SaveReceipt(context.Background(), owner, "image/png", "receipt.png", []byte{1})
	require.Empty(t, ref)
	for _, err := range []error{saveErr, store.DeleteReceipt(context.Background(), owner, testReceiptReference(owner))} {
		require.ErrorIs(t, err, connectionErr)
		require.NotContains(t, err.Error(), "sensitive")
		require.NotContains(t, err.Error(), owner.String())
	}
}

func TestSupabaseRejectsRedirectWithoutMutatingClient(t *testing.T) {
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = time.Second
	store, err := NewSupabase(server.URL, testSupabaseKey, client)
	require.NoError(t, err)
	require.Nil(t, client.CheckRedirect)
	owner := uuid.New()
	ref, err := store.SaveReceipt(context.Background(), owner, "image/png", "receipt.png", []byte{1})
	require.Empty(t, ref)
	require.Error(t, err)
	require.Error(t, store.DeleteReceipt(context.Background(), owner, testReceiptReference(owner)))
	_, err = store.ReadReceipt(context.Background(), owner, testReceiptReference(owner))
	require.Error(t, err)
	require.Zero(t, forwarded.Load())
}
