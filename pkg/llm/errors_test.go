package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tidwall/gjson"
)

func TestWriteError(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		header   map[string]string
		code     int
		wantPath string
		wantType string
	}{
		{
			name:     "chat completions",
			path:     "/api/llm-proxy/v1/chat/completions",
			code:     http.StatusUnauthorized,
			wantPath: "error.type",
			wantType: "authentication_error",
		},
		{
			name:     "responses",
			path:     "/api/llm-proxy/v1/responses",
			code:     http.StatusForbidden,
			wantPath: "error.type",
			wantType: "permission_denied",
		},
		{
			name:     "messages",
			path:     "/api/llm-proxy/v1/messages",
			code:     http.StatusNotFound,
			wantPath: "error.type",
			wantType: "not_found_error",
		},
		{
			name:     "former anthropic route",
			path:     "/api/llm-proxy/anthropic/v1/messages/count_tokens",
			code:     http.StatusUnauthorized,
			wantPath: "error.type",
			wantType: "authentication_error",
		},
		{
			name:     "anthropic client listing models",
			path:     "/api/llm-proxy/v1/models",
			header:   map[string]string{"anthropic-version": "2023-06-01"},
			code:     http.StatusForbidden,
			wantPath: "error.type",
			wantType: "permission_error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			WriteError(rec, req, "no access", tt.code)
			body := rec.Body.Bytes()
			if rec.Code != tt.code || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("got %d %s", rec.Code, rec.Header().Get("Content-Type"))
			}
			if got := gjson.GetBytes(body, tt.wantPath).String(); got != tt.wantType {
				t.Fatalf("%s = %q, want %q in %s", tt.wantPath, got, tt.wantType, body)
			}
			if gjson.GetBytes(body, "error.message").String() != "no access" {
				t.Fatalf("message missing from %s", body)
			}
			anthropic := gjson.GetBytes(body, "type").String() == "error"
			if anthropic != (ErrorDialect(req) == DialectAnthropicMessages) {
				t.Fatalf("body %s isn't in the request's format", body)
			}
		})
	}
}
