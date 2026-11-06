package llmengine

import (
	"encoding/json"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestAdminKeyUpdateAndConfirmedDestinationChange(t *testing.T) {
	e, ctx := startTestEngine(t)
	upstream := mockOpenAI(t)
	defer upstream.Close()
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai"}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "primary", "value": "sk-private-long-secret", "models": []string{"*"}, "weight": 1}))))
	var listed struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, testResult(&listed, "keys")(e.ListProviderKeys(ctx, "openai")))
	require.Len(t, listed.Keys, 1)
	key := listed.Keys[0]
	key["name"] = "renamed"
	require.NoError(t, testMutation(ctx, e, nil)(e.UpdateKey(ctx, "openai", key["id"].(string), testJSON(t, key))))
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, "sk-private-long-secret", cfg.Config.Keys[schemas.OpenAI][0].Value.GetValue())
	before := cfg
	change := map[string]any{"network_config": map[string]any{"base_url": upstream.URL, "allow_private_network": true}}
	require.Error(t, testMutation(ctx, e, nil)(e.UpdateProvider(ctx, "openai", testJSON(t, change), false)))
	cfg, err = e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, before, cfg)
	require.Len(t, cfg.Config.Keys[schemas.OpenAI], 1)
	require.NoError(t, testMutation(ctx, e, nil)(e.UpdateProvider(ctx, "openai", testJSON(t, change), true)))
	cfg, err = e.store.Load(ctx)
	require.NoError(t, err)
	require.Empty(t, cfg.Config.Keys[schemas.OpenAI])
	require.Equal(t, upstream.URL, cfg.Config.Providers[schemas.OpenAI].NetworkConfig.BaseURL)
}

func TestPricingAndRoutingValidationBeforeCommit(t *testing.T) {
	e, ctx := startTestEngine(t)
	before, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Error(t, testMutation(ctx, e, nil, "pricing_override")(e.CreatePricingOverride(ctx, testJSON(t, map[string]any{"name": "invalid", "scope_kind": "unknown", "match_type": "exact", "pattern": "gpt-4o", "patch": map[string]any{}}))))
	after, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, testMutation(ctx, e, nil, "pricing_override")(e.CreatePricingOverride(ctx, testJSON(t, map[string]any{"name": "global", "scope_kind": "global", "match_type": "exact", "pattern": "gpt-4o", "request_types": []string{"chat_completion"}, "patch": map[string]any{}}))))
	require.False(t, usesUnsupportedBudgetVariable(`headers["request"] == "budget_used"`))
	require.True(t, usesUnsupportedBudgetVariable(`budget_used > 0.5`))
	require.True(t, usesUnsupportedBudgetVariable(`request > 0`))
}

func TestGlobalProxyPasswordRoundTrip(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.UpdateProxyConfig(ctx, testJSON(t, map[string]any{"enabled": false, "type": "http", "url": "http://proxy.example.com:3128", "password": "private-proxy-password"})))
	var proxy map[string]any
	require.NoError(t, testResult(&proxy, "")(e.GetProxyConfig(ctx)))
	require.NotEqual(t, "private-proxy-password", proxy["password"])
	proxy["timeout"] = 20
	require.NoError(t, e.UpdateProxyConfig(ctx, testJSON(t, proxy)))
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Contains(t, string(cfg.Config.Settings["proxy"]), "private-proxy-password")
}

func TestUpdateClientConfigInitializesWritableSettings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prior json.RawMessage
	}{
		{
			name: "missing",
		},
		{
			name:  "null",
			prior: json.RawMessage(`null`),
		},
		{
			name:  "existing fields",
			prior: json.RawMessage(`{"log_retention_days":7}`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := llmcoreclient.Config{Settings: map[string]json.RawMessage{"client": tc.prior}}
			out, err := updateClientConfig(&cfg, []byte(`{"client_config":{"max_request_body_size_mb":4}}`))
			require.NoError(t, err)
			require.Equal(t, float64(4), out["max_request_body_size_mb"])
			if tc.name == "existing fields" {
				require.Equal(t, float64(7), out["log_retention_days"])
			}
			var persisted map[string]any
			require.NoError(t, json.Unmarshal(cfg.Settings["client"], &persisted))
			require.Equal(t, out, persisted)
		})
	}
}
