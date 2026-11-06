package llmcore_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

var (
	messagesRequestModes = []struct {
		path   string
		stream bool
	}{
		{
			path:   "/v1/messages",
			stream: false,
		},
		{
			path:   "/v1/messages",
			stream: true,
		},
		{
			path:   "/anthropic/v1/messages",
			stream: false,
		},
		{
			path:   "/anthropic/v1/messages",
			stream: true,
		},
		{
			path:   "/v1/messages/count_tokens",
			stream: false,
		},
		{
			path:   "/anthropic/v1/messages/count_tokens",
			stream: false,
		},
	}
)

func TestMessagesInputErrorsAreBadRequests(t *testing.T) {
	engine, err := llmcore.New(t.Context(), newTestStore(t), llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	for _, api := range messagesRequestModes {
		for _, tt := range []struct {
			name  string
			field string
		}{
			{
				name:  "missing messages",
				field: "",
			},
			{
				name:  "null messages",
				field: `,"messages":null`,
			},
			{
				name:  "empty messages",
				field: `,"messages":[]`,
			},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t/%s", api.path, api.stream, tt.name), func(t *testing.T) {
				body := fmt.Sprintf(`{"model":"anthropic/claude-test","max_tokens":8,"stream":%t%s}`, api.stream, tt.field)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(body)))
				require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				require.Equal(t, "application/json", response.Header().Get("Content-Type"))
				require.Contains(t, response.Body.String(), `"type":"error"`)
				require.Contains(t, response.Body.String(), `"type":"invalid_request_error"`)
			})
		}
	}
}

func TestMessagesUpstreamFailuresRemainServerErrors(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		// This text matches a core validation error. Status classification must
		// use Bifrost's metadata rather than infer caller errors from the message.
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"responses not provided for responses request"}}`)
	}))
	defer upstream.Close()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{
		Config: schemas.ProviderConfig{
			NetworkConfig: schemas.NetworkConfig{
				BaseURL:             upstream.URL,
				AllowPrivateNetwork: true,
				MaxRetries:          1,
				RetryBackoffInitial: time.Millisecond,
				RetryBackoffMax:     time.Millisecond,
			},
		},
		Keys: []schemas.Key{{ID: "key", Name: "key", Value: *schemas.NewSecretVar("secret"), Models: schemas.WhiteList{"*"}, Weight: 1}},
	}))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
	for _, api := range messagesRequestModes {
		t.Run(fmt.Sprintf("%s/stream=%t", api.path, api.stream), func(t *testing.T) {
			before := calls.Load()
			body := fmt.Sprintf(`{"model":"anthropic/claude-test","max_tokens":8,"messages":[{"role":"user","content":"hello"}],"stream":%t}`, api.stream)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(body)))
			require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
			require.Greater(t, calls.Load(), before, "valid Messages input must reach the provider")
			require.Contains(t, response.Body.String(), `"type":"api_error"`)
			require.Contains(t, response.Body.String(), `"type":"error"`)
		})
	}
}
