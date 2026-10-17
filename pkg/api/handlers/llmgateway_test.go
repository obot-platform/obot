package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestAllowedBifrostAPIPath(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		escaped string
		want    string
		wantOK  bool
	}{
		{
			name:    "provider list",
			raw:     "providers",
			escaped: "/api/llm-gateway/admin/providers",
			want:    "providers",
			wantOK:  true,
		},
		{
			name:    "provider keys",
			raw:     "providers/openai/keys",
			escaped: "/api/llm-gateway/admin/providers/openai/keys",
			want:    "providers/openai/keys",
			wantOK:  true,
		},
		{
			name:    "routing rules",
			raw:     "governance/routing-rules",
			escaped: "/api/llm-gateway/admin/governance/routing-rules",
			want:    "governance/routing-rules",
			wantOK:  true,
		},
		{
			name:    "virtual keys are not exposed",
			raw:     "governance/virtual-keys",
			escaped: "/api/llm-gateway/admin/governance/virtual-keys",
		},
		{
			name:    "plugins are not exposed",
			raw:     "plugins/obot-guardrails-input",
			escaped: "/api/llm-gateway/admin/plugins/obot-guardrails-input",
		},
		{
			name:    "the response cache is not exposed",
			raw:     "cache/clear",
			escaped: "/api/llm-gateway/admin/cache/clear",
		},
		{
			name:    "hidden api",
			raw:     "mcp/clients",
			escaped: "/api/llm-gateway/admin/mcp/clients",
		},
		{
			name:    "dot segments",
			raw:     "providers/../session/login",
			escaped: "/api/llm-gateway/admin/providers/../session/login",
		},
		{
			name:    "encoded dot segments",
			raw:     "providers/../mcp/clients",
			escaped: "/api/llm-gateway/admin/providers/%2e%2e/mcp/clients",
		},
		{
			name:    "encoded slash",
			raw:     "providers/../mcp/clients",
			escaped: "/api/llm-gateway/admin/providers%2F..%2Fmcp%2Fclients",
		},
		{
			name:    "prefix lookalike",
			raw:     "providersx",
			escaped: "/api/llm-gateway/admin/providersx",
		},
		{
			name:    "double slash",
			raw:     "providers//openai",
			escaped: "/api/llm-gateway/admin/providers//openai",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := allowedBifrostAPIPath(tt.raw, tt.escaped)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRejectUnsafeConfig(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "plain key",
			body: `{"name":"k","value":"sk-123","models":["*"]}`,
		},
		{
			name:    "env reference",
			body:    `{"name":"k","value":"env.OBOT_SERVER_DSN"}`,
			wantErr: true,
		},
		{
			name:    "nested vault reference",
			body:    `{"azure_key_config":{"endpoint":"vault.secret/path"}}`,
			wantErr: true,
		},
		{
			name:    "object env reference",
			body:    `{"value":{"ref":"HOME","type":"env"}}`,
			wantErr: true,
		},
		{
			name:    "from_env flag",
			body:    `{"value":{"env_var":"HOME","from_env":true}}`,
			wantErr: true,
		},
		{
			name:    "reference in array",
			body:    `{"headers":["env.SECRET"]}`,
			wantErr: true,
		},
		{
			name:    "type in another case",
			body:    `{"name":"k","value":{"value":"","ref":"OBOT_SERVER_DSN","TYPE":"env"}}`,
			wantErr: true,
		},
		{
			name:    "from_env in another case",
			body:    `{"name":"k","value":{"value":"","env_var":"OBOT_SERVER_DSN","FROM_ENV":true}}`,
			wantErr: true,
		},
		{
			name:    "escaped env reference",
			body:    `{"name":"k","value":"\u0065nv.OBOT_SERVER_DSN"}`,
			wantErr: true,
		},
		{
			name:    "vault reference by prefix",
			body:    `{"keys":[{"value":{"value":"","ref":"vault.secret/path"}}]}`,
			wantErr: true,
		},
		{
			name:    "from_env without a variable",
			body:    `{"name":"k","value":{"value":"x","From_Env":true}}`,
			wantErr: true,
		},
		{
			name: "a key as the gateway lists it",
			body: `{"name":"k","value":{"value":"sk-p************************abcd","type":"plain_text"},"models":["*"]}`,
		},
		{
			name: "region",
			body: `{"vertex_key_config":{"project_id":"p","region":"us-central1"}}`,
		},
		{
			name:    "region that changes the host",
			body:    `{"vertex_key_config":{"project_id":"p","region":"attacker.example/x?"}}`,
			wantErr: true,
		},
		{
			name:    "region object that changes the host",
			body:    `{"vertex_key_config":{"project_id":"p","Region":{"value":"attacker.example#"}}}`,
			wantErr: true,
		},
		{
			name: "not json",
			body: `hello`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectUnsafeConfig([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEngineError(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		body        string
		wantMessage string
	}{
		{
			name:        "engine error object",
			code:        http.StatusNotFound,
			body:        `{"is_bifrost_error":false,"status_code":404,"error":{"message":"Provider not found: not found"},"extra_fields":{"routing_info":{}}}`,
			wantMessage: "Provider not found: not found",
		},
		{
			name:        "top-level message",
			code:        http.StatusBadRequest,
			body:        `{"message":"bifrost could not parse the request"}`,
			wantMessage: "LLM gateway could not parse the request",
		},
		{
			name:        "plain text",
			code:        http.StatusConflict,
			body:        "Provider openai already exists\n",
			wantMessage: "Provider openai already exists",
		},
		{
			name:        "empty body",
			code:        http.StatusBadGateway,
			body:        "",
			wantMessage: "Bad Gateway",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var httpErr *types.ErrHTTP
			if err := engineError(tt.code, []byte(tt.body)); !errors.As(err, &httpErr) {
				t.Fatalf("engineError returned %T, want *types.ErrHTTP", err)
			}
			if httpErr.Code != tt.code || httpErr.Message != tt.wantMessage {
				t.Errorf("engineError = %d %q, want %d %q", httpErr.Code, httpErr.Message, tt.code, tt.wantMessage)
			}
		})
	}
}

// A change the gateway's store refused because it was busy is sent again until it goes
// through, and given up on, with the store's answer, once retrying takes too long.
func TestServeRetryingBusyStore(t *testing.T) {
	respond := func(code int, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rec.WriteHeader(code)
		_, _ = rec.WriteString(body)
		return rec
	}
	const busy = `{"error":{"message":"failed to update provider key in store: database is locked"}}`

	calls := 0
	rec, err := serveRetryingBusyStore(context.Background(), func() (*httptest.ResponseRecorder, error) {
		calls++
		if calls < 3 {
			return respond(http.StatusInternalServerError, busy), nil
		}
		return respond(http.StatusOK, `{}`), nil
	})
	if err != nil || rec.Code != http.StatusOK || calls != 3 {
		t.Fatalf("got %v, %d after %d calls; want 200 on the third", err, rec.Code, calls)
	}

	calls = 0
	rec, _ = serveRetryingBusyStore(context.Background(), func() (*httptest.ResponseRecorder, error) {
		calls++
		return respond(http.StatusInternalServerError, `{"error":{"message":"failed to start"}}`), nil
	})
	if rec.Code != http.StatusInternalServerError || calls != 1 {
		t.Fatalf("another failure was retried: %d calls", calls)
	}

	old := busyStoreRetryTimeout
	busyStoreRetryTimeout = 500 * time.Millisecond
	defer func() { busyStoreRetryTimeout = old }()
	calls = 0
	start := time.Now()
	rec, _ = serveRetryingBusyStore(context.Background(), func() (*httptest.ResponseRecorder, error) {
		calls++
		return respond(http.StatusInternalServerError, busy), nil
	})
	if rec.Code != http.StatusInternalServerError || calls < 2 || time.Since(start) > 2*time.Second {
		t.Fatalf("got %d after %d calls in %s; want the busy answer once the retry time runs out", rec.Code, calls, time.Since(start))
	}
}
