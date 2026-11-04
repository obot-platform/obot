package llmengine

import (
	"net/http"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/require"
)

func TestAdminKeyUpdateAndConfirmedDestinationChange(t *testing.T) {
	e, ctx := startTestEngine(t)
	upstream := mockOpenAI(t)
	defer upstream.Close()
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, nil))
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "primary", "value": "sk-private-long-secret", "models": []string{"*"}, "weight": 1}, nil))
	var listed struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, e.Do(ctx, http.MethodGet, "/api/providers/openai/keys", nil, &listed))
	require.Len(t, listed.Keys, 1)
	key := listed.Keys[0]
	key["name"] = "renamed"
	require.NoError(t, e.Do(ctx, http.MethodPut, "/api/providers/openai/keys/"+key["id"].(string), key, nil))
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, "sk-private-long-secret", cfg.Config.Keys[schemas.OpenAI][0].Value.GetValue())
	before := cfg
	change := map[string]any{"network_config": map[string]any{"base_url": upstream.URL, "allow_private_network": true}}
	require.Error(t, e.Do(ctx, http.MethodPut, "/api/providers/openai", change, nil))
	cfg, err = e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, before, cfg)
	require.Len(t, cfg.Config.Keys[schemas.OpenAI], 1)
	require.NoError(t, e.Do(ctx, http.MethodPut, "/api/providers/openai?removeKeys=true", change, nil))
	cfg, err = e.store.Load(ctx)
	require.NoError(t, err)
	require.Empty(t, cfg.Config.Keys[schemas.OpenAI])
	require.Equal(t, upstream.URL, cfg.Config.Providers[schemas.OpenAI].NetworkConfig.BaseURL)
}

func TestPricingAndRoutingValidationBeforeCommit(t *testing.T) {
	e, ctx := startTestEngine(t)
	before, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Error(t, e.Do(ctx, http.MethodPost, "/api/governance/pricing-overrides", map[string]any{"name": "invalid", "scope_kind": "unknown", "match_type": "exact", "pattern": "gpt-4o", "patch": map[string]any{}}, nil))
	after, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/governance/pricing-overrides", map[string]any{"name": "global", "scope_kind": "global", "match_type": "exact", "pattern": "gpt-4o", "request_types": []string{"chat_completion"}, "patch": map[string]any{}}, nil))
	require.False(t, usesUnsupportedBudgetVariable(`headers["request"] == "budget_used"`))
	require.True(t, usesUnsupportedBudgetVariable(`budget_used > 0.5`))
	require.True(t, usesUnsupportedBudgetVariable(`request > 0`))
}

func TestGlobalProxyPasswordRoundTrip(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.Do(ctx, http.MethodPut, "/api/proxy-config", map[string]any{"enabled": false, "type": "http", "url": "http://proxy.example.com:3128", "password": "private-proxy-password"}, nil))
	var proxy map[string]any
	require.NoError(t, e.Do(ctx, http.MethodGet, "/api/proxy-config", nil, &proxy))
	require.NotEqual(t, "private-proxy-password", proxy["password"])
	proxy["timeout"] = 20
	require.NoError(t, e.Do(ctx, http.MethodPut, "/api/proxy-config", proxy, nil))
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Contains(t, string(cfg.Config.Settings["proxy"]), "private-proxy-password")
}
