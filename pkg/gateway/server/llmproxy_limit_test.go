package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// limitedBackend is a backend that accepts request bodies up to limit bytes. The proxy must
// refuse larger bodies before it does anything else with the request.
type limitedBackend struct {
	limit int64
}

func (b limitedBackend) modelProviderName() string { return system.LLMGatewayModelProvider }

func (b limitedBackend) upstreamURL(*http.Request) (url.URL, llmtypes.Dialect, error) {
	return url.URL{Scheme: "http", Host: "gateway.invalid"}, llmtypes.DialectOpenAIChatCompletions, nil
}

func (b limitedBackend) transport() (http.RoundTripper, error) {
	return nil, nil
}

func (b limitedBackend) decorateRequest(*llmProviderProxy, api.Context, *v1.Model) (gatewayRequest, error) {
	return nil, nil
}

func (b limitedBackend) maxRequestBodyBytes() int64 { return b.limit }

func TestProxyRefusesRequestsOverTheSizeLimit(t *testing.T) {
	const limit = 1 << 20
	big := `{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("x", limit) + `"}]}`
	tests := []struct {
		name          string
		path          string
		contentLength bool
		wantType      string
	}{
		{
			name:          "openai with a content length",
			path:          "v1/chat/completions",
			contentLength: true,
			wantType:      "invalid_request_error",
		},
		{
			name:     "openai streamed body",
			path:     "v1/responses",
			wantType: "invalid_request_error",
		},
		{
			name:          "anthropic with a content length",
			path:          "v1/messages",
			contentLength: true,
			wantType:      "request_too_large",
		},
		{
			name:     "anthropic streamed body",
			path:     "v1/messages",
			wantType: "request_too_large",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader = strings.NewReader(big)
			if !tt.contentLength {
				// Hide the length, as a chunked request does.
				body = io.MultiReader(body)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+tt.path, body)
			if !tt.contentLength {
				req.ContentLength = -1
			}
			req.SetPathValue("path", tt.path)
			rec := httptest.NewRecorder()
			l := &llmProviderProxy{backend: limitedBackend{limit: limit}, modelProvider: &v1.ModelProvider{}}
			if err := l.proxy(api.Context{ResponseWriter: rec, Request: req}); err != nil {
				t.Fatalf("proxy() error = %v", err)
			}
			if rec.Code != http.StatusRequestEntityTooLarge || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("got %d %s, want 413 application/json", rec.Code, rec.Header().Get("Content-Type"))
			}
			var resp struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Error.Type != tt.wantType || !strings.Contains(resp.Error.Message, "1 MB") {
				t.Fatalf("unexpected body %s", rec.Body.String())
			}
		})
	}
}
