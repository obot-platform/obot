package llmcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionCredential(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers http.Header
		valid   bool
		token   string
	}{
		{
			name:  "configured key mode",
			valid: true,
		},
		{
			name:    "subscription",
			headers: http.Header{"Authorization": {"bEaReR Sk-AnT-OaT-fixture"}, "User-Agent": {"claude-cli/2"}},
			valid:   true,
			token:   "bEaReR Sk-AnT-OaT-fixture",
		},
		{
			name:    "duplicate case variants",
			headers: http.Header{"Authorization": {"Bearer sk-ant-oat-a"}, "authorization": {"Bearer sk-ant-oat-b"}, "User-Agent": {"claude-cli/2"}},
		},
		{
			name:    "empty authorization",
			headers: http.Header{"Authorization": {""}},
		},
		{
			name:    "empty authorization values",
			headers: http.Header{"Authorization": {}},
		},
		{
			name:    "repeated UA",
			headers: http.Header{"Authorization": {"Bearer sk-ant-oat-a"}, "User-Agent": {"claude-cli/2", "claude-cli/2"}},
		},
		{
			name:    "CLI UA with comment",
			headers: http.Header{"Authorization": {"Bearer sk-ant-oat-a"}, "User-Agent": {"claude-cli/2 (external, cli)"}},
			valid:   true,
			token:   "Bearer sk-ant-oat-a",
		},
		{
			name:    "combined auth",
			headers: http.Header{"Authorization": {"Bearer sk-ant-oat-a, Bearer sk-ant-oat-b"}, "User-Agent": {"claude-cli/2"}},
		},
		{
			name:    "space in token",
			headers: http.Header{"Authorization": {"Bearer sk-ant-oat-a other"}, "User-Agent": {"claude-cli/2"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, valid := subscriptionCredential(tc.headers)
			require.Equal(t, tc.valid, valid)
			require.Equal(t, tc.token, token.token)
		})
	}
	for _, name := range []string{"X-Api-Key", "Api-Key", "X-Goog-Api-Key"} {
		for _, value := range []string{"", "fixture"} {
			token, valid := subscriptionCredential(http.Header{name: {value}})
			require.False(t, valid, name)
			require.Empty(t, token.token)
		}
	}
}

func TestCodexSubscriptionRejected(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
	}{
		{
			method: http.MethodPost,
			path:   "/v1/responses",
		},
		{
			method: http.MethodGet,
			path:   "/v1/models?client_version=0.162.1",
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"model":"openai/test","input":"hi"}`))
			r.Header.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJmaXh0dXJlIn0.c2ln")
			r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
			w := httptest.NewRecorder()
			// Unsupported credentials must be refused before reaching inference or a catalog handler.
			(&Engine{}).ServeHTTP(w, r)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "permission_denied")
		})
	}
}

func TestSubscriptionDestination(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		subscriptionReply(w, r)
	}))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("configured-key")))
	e, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
	subscriptionTransport(t, e, server)

	for _, tc := range []struct {
		name   string
		config schemas.ProviderConfig
		proxy  string
		absent bool
		safe   bool
	}{
		{
			name: "default",
			safe: true,
		},
		{
			name:   "case and port",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: "https://API.ANTHROPIC.COM:443/"}},
			safe:   true,
		},
		{
			name:   "missing provider",
			absent: true,
		},
		{
			name:   "different host",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: "https://api.anthropic.com.example"}},
		},
		{
			name:   "http",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: "http://api.anthropic.com"}},
		},
		{
			name:   "userinfo",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: "https://user@api.anthropic.com"}},
		},
		{
			name:   "port",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: "https://api.anthropic.com:444"}},
		},
		{
			name:   "invalid URL",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: "https://api.anthropic.com:%"}},
		},
		{
			name:   "custom adapter",
			config: schemas.ProviderConfig{CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.Anthropic}},
		},
		{
			name:   "insecure network",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{InsecureSkipVerify: true}},
		},
		{
			name:   "network CA",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{CACertPEM: schemas.NewSecretVar("fixture")}},
		},
		{
			name:   "unresolved network CA",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{CACertPEM: schemas.NewSecretVar("env.OBOT_UNRESOLVED_SUBSCRIPTION_CA")}},
		},
		{
			name:   "empty CA",
			config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{CACertPEM: schemas.NewSecretVar("")}},
			safe:   true,
		},
		{
			name:   "proxy CA",
			config: schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, CACertPEM: schemas.NewSecretVar("fixture")}},
		},
		{
			name:   "unresolved proxy CA",
			config: schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, CACertPEM: schemas.NewSecretVar("env.OBOT_UNRESOLVED_SUBSCRIPTION_CA")}},
		},
		{
			name:   "insecure provider proxy",
			config: schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, SkipTLSVerify: true}},
		},
		{
			name:   "verified proxy",
			config: schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://proxy.example")}},
			safe:   true,
		},
		{
			name:  "inherited insecure proxy",
			proxy: `{"enabled":true,"enable_for_inference":true,"url":"http://proxy.example","skip_tls_verify":true}`,
		},
		{
			name:  "disabled insecure global proxy",
			proxy: `{"enabled":false,"enable_for_inference":true,"url":"http://proxy.example","skip_tls_verify":true}`,
			safe:  true,
		},
		{
			name:  "non inference global proxy",
			proxy: `{"enabled":true,"enable_for_inference":false,"url":"http://proxy.example","skip_tls_verify":true}`,
			safe:  true,
		},
		{
			name:   "provider overrides insecure global proxy",
			config: schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://provider.example")}},
			proxy:  `{"enabled":true,"enable_for_inference":true,"url":"http://global.example","skip_tls_verify":true}`,
			safe:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &llmcoreclient.Config{
				Providers: map[schemas.ModelProvider]schemas.ProviderConfig{},
				Settings:  map[string]json.RawMessage{"proxy": json.RawMessage(tc.proxy)},
			}
			if !tc.absent {
				cfg.Providers[schemas.Anthropic] = tc.config
			}
			require.Equal(t, tc.safe, subscriptionDestination(cfg))

			a := &account{}
			a.config.Store(cfg)
			ctx := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
			ctx.SetValue(subscriptionTokenKey{}, subscriptionCredentialValue{token: "Bearer sk-ant-oat-fixture"})
			req := &schemas.BifrostRequest{ChatRequest: &schemas.BifrostChatRequest{Provider: schemas.Anthropic, Model: "test"}}
			denied := prepareSubscription(ctx, req, a)
			require.Equal(t, tc.safe, denied == nil)
			require.Equal(t, tc.safe, ctx.Value(schemas.BifrostContextKeySkipKeySelection))
			if !tc.safe {
				require.Equal(t, http.StatusForbidden, denied.Error.EffectiveHTTPStatus())
				require.True(t, *denied.Error.AllowFallbacks)
				require.Empty(t, ctx.Value(schemas.BifrostContextKeyExtraHeaders))
			}

			// Keep the recording runtime fixed while varying the policy snapshot:
			// a missed refusal would produce a real outbound recorded request.
			e.account.config.Store(cfg)
			result := subscriptionCall(t.Context(), e, "/v1/messages", "anthropic/test", false, true, nil, nil)
			if tc.safe {
				require.Equal(t, http.StatusOK, result.Code, result.Body.String())
				require.Equal(t, subscriptionFixture, (<-received).Get("Authorization"))
			} else {
				require.Equal(t, http.StatusForbidden, result.Code, result.Body.String())
				require.Empty(t, received)
			}
		})
	}
	for _, name := range []string{"Authorization", "x-API-key", "api-key", "X-Goog-Api-Key", "ChatGPT-Account-ID", "X-Obot-Api-Key"} {
		require.False(t, subscriptionDestination(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{
			schemas.Anthropic: {NetworkConfig: schemas.NetworkConfig{ExtraHeaders: map[string]string{name: ""}}},
		}}), name)
	}
}

func TestSubscriptionAttemptIsolation(t *testing.T) {
	a := &account{}
	a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Anthropic: {}}})
	ctx := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	ctx.SetValue(subscriptionTokenKey{}, subscriptionCredentialValue{token: "Bearer sk-ant-oat-fixture"})
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, map[string][]string{"x-stainless-lang": {"go", "typescript"}})
	for _, provider := range []schemas.ModelProvider{schemas.Anthropic, schemas.OpenAI, "custom-anthropic", schemas.Anthropic} {
		req := &schemas.BifrostRequest{ChatRequest: &schemas.BifrostChatRequest{Provider: provider, Model: "test"}}
		denied := prepareSubscription(ctx, req, a)
		extra := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
		require.Equal(t, []string{"go", "typescript"}, extra["x-stainless-lang"])
		if provider == schemas.Anthropic {
			require.Nil(t, denied)
			require.True(t, ctx.Value(schemas.BifrostContextKeySkipKeySelection).(bool))
			require.Equal(t, []string{"Bearer sk-ant-oat-fixture"}, extra["authorization"])
		} else {
			require.NotNil(t, denied)
			require.False(t, ctx.Value(schemas.BifrostContextKeySkipKeySelection).(bool))
			require.NotContains(t, extra, "authorization")
		}
	}
}

func TestSubscriptionContextKeepsCredentialsPrivate(t *testing.T) {
	e := &Engine{account: &account{}}
	e.account.config.Store(&llmcoreclient.Config{})
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	r.Header.Set("Authorization", "Bearer sk-ant-oat-fixture")
	ctx, cancel := e.requestContext(r)
	defer cancel()
	require.NotContains(t, ctx.Value(schemas.BifrostContextKeyRequestHeaders), "authorization")
	require.NotContains(t, ctx.Value(schemas.BifrostContextKeyExtraHeaders), "authorization")
}
