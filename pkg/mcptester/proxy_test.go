package mcptester

import (
	"context"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
)

func TestNewLLMProxyRequestPreservesIdentityAndAuditAttribution(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	inbound := make(http.Header)
	inbound.Set("Authorization", "Bearer user-token")
	inbound.Add("Cookie", "obot-session=user-session")
	inbound.Set("User-Agent", "browser-agent")

	req, err := NewLLMProxyRequest(ctx, "https://obot.example/base", ResolvedModel{
		ID:        "m1default",
		Target:    "gpt-test",
		Provider:  "openai-model-provider",
		Dialect:   llmtypes.DialectOpenAIResponses,
		ProxyPath: "openai",
		APIPath:   "/v1/responses",
	}, []byte(`{"model":"gpt-test","stream":true}`), inbound)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := req.URL.String(), "https://obot.example/base/api/llm-proxy/openai/v1/responses"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer user-token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := req.Header.Get("Cookie"); got != "obot-session=user-session" {
		t.Fatalf("Cookie = %q", got)
	}
	if got := req.UserAgent(); got != types.MCPTesterClientName {
		t.Fatalf("User-Agent = %q, want %q", got, types.MCPTesterClientName)
	}
	if got := req.Header.Get("anthropic-version"); got != "" {
		t.Fatalf("anthropic-version = %q, want empty for OpenAI dialect", got)
	}

	cancel()
	if req.Context().Err() != context.Canceled {
		t.Fatalf("request context error = %v, want context.Canceled", req.Context().Err())
	}
}

func TestNewLLMProxyRequestSetsAnthropicVersion(t *testing.T) {
	req, err := NewLLMProxyRequest(t.Context(), "https://obot.example", ResolvedModel{
		ID:        "m1default",
		Target:    "claude-test",
		Provider:  "anthropic-model-provider",
		Dialect:   llmtypes.DialectAnthropicMessages,
		ProxyPath: "anthropic",
		APIPath:   "/v1/messages",
	}, []byte(`{"model":"claude-test","stream":true}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("anthropic-version"); got != anthropicVersion {
		t.Fatalf("anthropic-version = %q, want %q", got, anthropicVersion)
	}
}
