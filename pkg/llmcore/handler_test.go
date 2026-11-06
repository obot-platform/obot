package llmcore_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/llmcore"
	"github.com/stretchr/testify/require"
)

type interruptedBody struct{}

func TestEndpointRegistration(t *testing.T) {
	e, err := llmcore.New(t.Context(), newTestStore(t), llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })

	for _, path := range []string{
		"/v1/responses",
		"/v1/messages",
		"/anthropic/v1/messages",
		"/v1/chat/completions",
		"/v1/responses/input_tokens",
		"/v1/messages/count_tokens",
		"/anthropic/v1/messages/count_tokens",
	} {
		t.Run(path, func(t *testing.T) {
			// Invalid JSON exercises dispatch and the API's error format without an upstream.
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{")))
			require.Equal(t, http.StatusBadRequest, w.Code)
			if strings.Contains(path, "/messages") {
				require.Contains(t, w.Body.String(), `"type":"error"`)
			} else {
				require.NotContains(t, w.Body.String(), `"type":"error"`)
			}
		})
	}

	for _, tt := range []struct {
		method string
		path   string
		status int
	}{
		{
			method: http.MethodPost,
			path:   "/v1/embeddings",
			status: http.StatusNotFound,
		},
		{
			method: http.MethodPost,
			path:   "/v1/responses/unknown",
			status: http.StatusNotFound,
		},
		{
			method: http.MethodGet,
			path:   "/v1/responses",
			status: http.StatusMethodNotAllowed,
		},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			require.Equal(t, tt.status, w.Code)
		})
	}
}

func (interruptedBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (interruptedBody) Close() error             { return nil }

func TestBodyReadErrors(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["client"] = json.RawMessage(`{"max_request_body_size_mb":1}`)
		return nil
	}))
	e, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1/messages", "/v1/messages/count_tokens", "/v1/responses/input_tokens"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Body = interruptedBody{}
			w := httptest.NewRecorder()
			e.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "could not read request body")

			req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat(" ", (1<<20)+1)))
			w = httptest.NewRecorder()
			e.ServeHTTP(w, req)
			require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		})
	}
}

func TestRequestLimitDoesNotOverflow(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["client"] = json.RawMessage(`{"max_request_body_size_mb":9223372036854775807}`)
		return nil
	}))
	e, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	require.Equal(t, int64(math.MaxInt64), e.MaxRequestBodyBytes())
}
