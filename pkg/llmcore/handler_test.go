package llmcore_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/llmcore"
	"github.com/stretchr/testify/require"
)

func TestEndpointRegistration(t *testing.T) {
	e, err := llmcore.New(t.Context(), newTestStore(t), llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(e.Shutdown)

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
