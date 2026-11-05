package llmcore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestCallerCredentialModeAndDestination(t *testing.T) {
	for _, tt := range []struct {
		name        string
		ua          string
		auth        []string
		apiKey      string
		destination string
		allowed     bool
	}{
		{
			name:    "supported default",
			ua:      "claude-cli/2.1.168",
			auth:    []string{"Bearer sk-ant-oat-fixture"},
			allowed: true,
		},
		{
			name: "missing user agent",
			auth: []string{"Bearer sk-ant-oat-fixture"},
		},
		{
			name: "other user agent",
			ua:   "other-client",
			auth: []string{"Bearer sk-ant-oat-fixture"},
		},
		{
			name:   "api key only",
			ua:     "claude-cli/2.1.168",
			apiKey: "fixture-key",
		},
		{
			name:   "mixed credentials",
			ua:     "claude-cli/2.1.168",
			auth:   []string{"Bearer sk-ant-oat-fixture"},
			apiKey: "fixture-key",
		},
		{
			name: "ambiguous authorization",
			ua:   "claude-cli/2.1.168",
			auth: []string{"Bearer sk-ant-oat-fixture", "Bearer sk-ant-oat-other"},
		},
		{
			name: "unsupported bearer",
			ua:   "claude-cli/2.1.168",
			auth: []string{"Bearer configured-fixture"},
		},
		{
			name:        "unsafe destination",
			ua:          "claude-cli/2.1.168",
			auth:        []string{"Bearer sk-ant-oat-fixture"},
			destination: "http://recording.invalid",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := &account{}
			a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{
				schemas.Anthropic: {NetworkConfig: schemas.NetworkConfig{BaseURL: tt.destination}},
			}})
			engine := &Engine{account: a}
			r := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			r.Header.Set("User-Agent", tt.ua)
			r.Header["Authorization"] = tt.auth
			r.Header.Set("X-Api-Key", tt.apiKey)
			ctx, cancel := engine.requestContext(r)
			defer cancel()
			req := &schemas.BifrostRequest{RequestType: schemas.ChatCompletionRequest, ChatRequest: &schemas.BifrostChatRequest{Provider: schemas.Anthropic, Model: "fixture"}}
			_, short, err := (&credentialPlugin{account: a}).PreLLMHook(ctx, req)
			require.NoError(t, err)
			if tt.allowed {
				require.Nil(t, short)
				require.Equal(t, true, ctx.Value(schemas.BifrostContextKeySkipKeySelection))
				require.Equal(t, tt.auth, ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)["authorization"])
			} else {
				require.NotNil(t, short, "unsupported credentials must stop before key selection")
				require.Equal(t, http.StatusForbidden, *short.Error.StatusCode)
				require.Equal(t, "permission_denied", *short.Error.Error.Type)
				require.True(t, *short.Error.AllowFallbacks)
			}
		})
	}
}

func TestSubscriptionDestinationPolicy(t *testing.T) {
	unresolved := schemas.NewSecretVar("env.OBOT_TEST_UNRESOLVED_CA")
	require.True(t, unresolved.IsSet())
	require.Empty(t, unresolved.GetValue())
	for _, tt := range []struct {
		name    string
		network schemas.NetworkConfig
		proxy   *schemas.ProxyConfig
		global  string
		custom  *schemas.CustomProviderConfig
		allowed bool
	}{
		{
			name:    "configured API key header",
			network: schemas.NetworkConfig{ExtraHeaders: map[string]string{"X-Api-Key": "fixture"}},
		},
		{
			name:    "configured mixed case authorization header",
			network: schemas.NetworkConfig{ExtraHeaders: map[string]string{"aUtHoRiZaTiOn": "fixture"}},
		},
		{
			name:    "empty configured auth header",
			network: schemas.NetworkConfig{ExtraHeaders: map[string]string{"x-api-key": ""}},
		},
		{
			name:    "configured non-auth header",
			network: schemas.NetworkConfig{ExtraHeaders: map[string]string{"Anthropic-Version": "2023-06-01"}},
			allowed: true,
		},

		{
			name:    "default",
			allowed: true,
		},
		{
			name:    "explicit HTTPS",
			network: schemas.NetworkConfig{BaseURL: "https://api.anthropic.com"},
			allowed: true,
		},
		{
			name:    "explicit port and path",
			network: schemas.NetworkConfig{BaseURL: "https://api.anthropic.com:443/v1"},
			allowed: true,
		},
		{
			name:    "HTTP",
			network: schemas.NetworkConfig{BaseURL: "http://api.anthropic.com"},
		},
		{
			name:    "other host",
			network: schemas.NetworkConfig{BaseURL: "https://alternate.invalid"},
		},
		{
			name:    "userinfo",
			network: schemas.NetworkConfig{BaseURL: "https://user@api.anthropic.com"},
		},
		{
			name:    "alternate port",
			network: schemas.NetworkConfig{BaseURL: "https://api.anthropic.com:8443"},
		},
		{
			name:    "malformed URL",
			network: schemas.NetworkConfig{BaseURL: "://invalid"},
		},
		{
			name:    "network verification bypass",
			network: schemas.NetworkConfig{InsecureSkipVerify: true},
		},
		{
			name:    "network CA",
			network: schemas.NetworkConfig{CACertPEM: schemas.NewSecretVar("fixture-ca")},
		},
		{
			name:    "unresolved network CA",
			network: schemas.NetworkConfig{CACertPEM: unresolved},
		},
		{
			name:    "empty network CA placeholder",
			network: schemas.NetworkConfig{CACertPEM: &schemas.SecretVar{}},
			allowed: true,
		},
		{
			name:    "verified proxy",
			proxy:   &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://proxy.invalid")},
			allowed: true,
		},
		{
			name:  "proxy verification bypass",
			proxy: &schemas.ProxyConfig{Type: schemas.HTTPProxy, SkipTLSVerify: true},
		},
		{
			name:  "proxy CA",
			proxy: &schemas.ProxyConfig{Type: schemas.HTTPProxy, CACertPEM: schemas.NewSecretVar("fixture-ca")},
		},
		{
			name:  "unresolved proxy CA",
			proxy: &schemas.ProxyConfig{Type: schemas.HTTPProxy, CACertPEM: unresolved},
		},
		{
			name:    "empty proxy CA placeholder",
			proxy:   &schemas.ProxyConfig{Type: schemas.HTTPProxy, CACertPEM: &schemas.SecretVar{}},
			allowed: true,
		},
		{
			name:   "global proxy bypass",
			global: `{"enabled":true,"enable_for_inference":true,"url":"http://proxy.invalid","skip_tls_verify":true}`,
		},
		{
			name:   "empty provider proxy inherits unsafe global",
			proxy:  &schemas.ProxyConfig{},
			global: `{"enabled":true,"enable_for_inference":true,"url":"http://proxy.invalid","skip_tls_verify":true}`,
		},
		{
			name:   "NoProxy inherits unsafe global",
			proxy:  &schemas.ProxyConfig{Type: schemas.NoProxy},
			global: `{"enabled":true,"enable_for_inference":true,"url":"http://proxy.invalid","skip_tls_verify":true}`,
		},
		{
			name:    "provider override replaces unsafe global",
			proxy:   &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://provider.invalid")},
			global:  `{"enabled":true,"enable_for_inference":true,"url":"http://proxy.invalid","skip_tls_verify":true}`,
			allowed: true,
		},
		{
			name:    "disabled global is ineffective",
			global:  `{"enabled":false,"enable_for_inference":true,"url":"http://proxy.invalid","skip_tls_verify":true}`,
			allowed: true,
		},
		{
			name:   "canonical custom Anthropic override",
			custom: &schemas.CustomProviderConfig{BaseProviderType: schemas.Anthropic},
		},
		{
			name:   "canonical foreign base override",
			custom: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := &account{}
			a.config.Store(&llmcoreclient.Config{
				Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Anthropic: {
					NetworkConfig:        tt.network,
					ProxyConfig:          tt.proxy,
					CustomProviderConfig: tt.custom,
				}},
				Settings: map[string]json.RawMessage{"proxy": json.RawMessage(tt.global)},
			})
			ctx, cancel := (&Engine{account: a}).requestContext(subscriptionRequest())
			defer cancel()
			_, short, err := (&credentialPlugin{account: a}).PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
			require.NoError(t, err)
			require.Equal(t, tt.allowed, short == nil)
		})
	}
	require.False(t, (*account)(nil).subscriptionDestinationSafe())
	require.False(t, (&account{}).subscriptionDestinationSafe())
	a := &account{}
	a.config.Store(&llmcoreclient.Config{})
	require.False(t, a.subscriptionDestinationSafe())
}

func subscriptionRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("User-Agent", "claude-cli/2.1.168")
	req.Header.Set("Authorization", "Bearer sk-ant-oat-fixture")
	return req
}

func subscriptionAttempt(provider schemas.ModelProvider) *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{Provider: provider, Model: "fixture"},
	}
}

func TestSubscriptionAttemptRecoveryAndOwnership(t *testing.T) {
	a := &account{}
	safe := &llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Anthropic: {}}}
	a.config.Store(safe)
	engine := &Engine{account: a}
	ctx, cancel := engine.requestContext(subscriptionRequest())
	defer cancel()
	plugin := &credentialPlugin{account: a}
	original := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
	for _, provider := range []schemas.ModelProvider{schemas.Anthropic, schemas.OpenAI, "custom-anthropic", schemas.Anthropic} {
		_, short, err := plugin.PreLLMHook(ctx, subscriptionAttempt(provider))
		require.NoError(t, err)
		require.Equal(t, provider == schemas.Anthropic, short == nil)
		headers := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
		if short != nil {
			require.NotContains(t, headers, "authorization")
		} else {
			require.Equal(t, original["authorization"], headers["authorization"])
		}
		require.Equal(t, []string{"Bearer sk-ant-oat-fixture"}, original["authorization"], "refusal must not mutate a published header snapshot")
	}

	// Safety is not cached in the context. Pending gates refuse both values,
	// and a repaired configuration permits the original admitted token again.
	a.transitioning.Store(schemas.Anthropic, true)
	_, short, _ := plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NotNil(t, short)
	require.Equal(t, http.StatusServiceUnavailable, *short.Error.StatusCode)
	require.Equal(t, "api_error", *short.Error.Error.Type)
	a.transitioning.Delete(schemas.Anthropic)
	a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Anthropic: {NetworkConfig: schemas.NetworkConfig{BaseURL: "http://alternate.invalid"}}}})
	_, short, _ = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NotNil(t, short)
	a.config.Store(safe)
	_, short, _ = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.Nil(t, short)

	// An owned HTTP lifetime cannot override Bifrost's reserved authentication
	// controls. These writes model direct caller overrides, outside plugin hooks.
	ctx.SetValue(schemas.BifrostContextKeySkipKeySelection, false)
	_, short, _ = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NotNil(t, short)
	ctx.SetValue(schemas.BifrostContextKeySkipKeySelection, true)
	ctx.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{ID: "override"})
	_, short, _ = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NotNil(t, short)
	ctx.SetValue(schemas.BifrostContextKeyDirectKey, nil)

	foreign := &account{}
	foreign.config.Store(safe)
	_, short, _ = (&credentialPlugin{account: foreign}).PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NotNil(t, short, "another engine cannot borrow the lease")
	cancel()
	cancel()
	_, short, _ = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NotNil(t, short, "canceled ownership cannot be reused")
	require.True(t, a.subscriptionMu.TryLock(), "owned cancel must release exactly once")
	a.subscriptionMu.Unlock()

	// Direct core contexts have no HTTP owner to release a lifetime lease.
	direct := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	direct.SetValue(schemas.BifrostContextKeyExtraHeaders, original)
	direct.SetValue(schemas.BifrostContextKeyUserAgent, "claude-cli/2.1.168")
	for _, skip := range []bool{false, true} {
		direct.SetValue(schemas.BifrostContextKeySkipKeySelection, skip)
		_, short, _ = plugin.PreLLMHook(direct, subscriptionAttempt(schemas.Anthropic))
		require.NotNil(t, short)
	}
}

func TestSubscriptionBearerSyntax(t *testing.T) {
	for _, tt := range []struct {
		value   string
		allowed bool
	}{
		{
			value:   "Bearer sk-ant-oat-fixture",
			allowed: true,
		},
		{
			value:   "bearer sk-ant-oat-fixture_1+/=",
			allowed: true,
		},
		{
			value: "Bearer sk-ant-oat-fixture,Bearer other",
		},
		{
			value: "Bearer sk-ant-oat-fixture other",
		},
		{
			value: "Bearer sk-ant-oat-fixture\x00",
		},
		{
			value: "Bearer sk-ant-oat-fixture\n",
		},
		{
			value: "Bearer sk-ant-oat=fixture",
		},
	} {
		require.Equal(t, tt.allowed, subscriptionBearer(tt.value))
	}
}
