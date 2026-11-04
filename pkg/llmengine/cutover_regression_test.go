package llmengine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/modeldata"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	"github.com/stretchr/testify/require"
)

func TestProviderWriteMethodsCannotBypassGuards(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, nil))
	var key schemas.Key
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "primary", "value": "secret"}, &key))
	before, err := e.store.Load(ctx)
	require.NoError(t, err)
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{
			method: http.MethodPost,
			path:   "/api/providers/openai",
			body:   `{"network_config":{"base_url":"http://169.254.169.254"}}`,
		},
		{
			method: http.MethodPost,
			path:   "/api/providers/openai/keys/" + key.ID,
			body:   `{"azure_key_config":{"endpoint":"http://169.254.169.254"}}`,
		},
		{
			method: http.MethodPut,
			path:   "/api/providers/openai/keys",
			body:   `{"name":"unvalidated","value":"secret"}`,
		},
		{
			method: http.MethodPut,
			path:   "/api/providers/openai/keys/" + key.ID + "/extra",
			body:   `{"name":"unvalidated"}`,
		},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			code, body := do(t, e, tc.method, tc.path, tc.body)
			require.Contains(t, []int{http.StatusMethodNotAllowed, http.StatusNotFound}, code, body)
			after, err := e.store.Load(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestDeletionResponsesAreJSON(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, nil))
	var rule struct {
		Rule struct {
			ID string `json:"id"`
		} `json:"rule"`
	}
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/governance/routing-rules", map[string]any{"name": "test", "cel_expression": "true", "targets": []map[string]any{{"provider": "openai", "model": "gpt-4o-mini", "weight": 1}}}, &rule))
	require.NotEmpty(t, rule.Rule.ID)
	for _, path := range []string{"/api/providers/openai", "/api/governance/routing-rules/" + rule.Rule.ID} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		var ack map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ack))
		require.Equal(t, "deleted", ack["status"])
	}
}

func TestPricingUpdatePreservesOmittedPatch(t *testing.T) {
	e, ctx := startTestEngine(t)
	var out struct {
		Override configPricingOverride `json:"pricing_override"`
	}
	patch := `{"input_cost_per_token":0.001,"output_cost_per_token":0.002}`
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/governance/pricing-overrides", map[string]any{"name": "custom", "scope_kind": "global", "match_type": "exact", "pattern": "gpt-4o-mini", "request_types": []string{"chat_completion"}, "patch": json.RawMessage(patch)}, &out))
	path := "/api/governance/pricing-overrides/" + out.Override.ID
	require.NoError(t, e.Do(ctx, http.MethodPut, path, map[string]any{"name": "renamed"}, &out))
	require.JSONEq(t, patch, out.Override.PricingPatchJSON)
	require.NoError(t, e.Do(ctx, http.MethodPut, path, map[string]any{"patch": map[string]any{"input_cost_per_token": 0.003}}, &out))
	require.JSONEq(t, `{"input_cost_per_token":0.003}`, out.Override.PricingPatchJSON)
	require.NoError(t, e.Do(ctx, http.MethodPut, path, map[string]any{"patch": map[string]any{}}, &out))
	require.JSONEq(t, `{}`, out.Override.PricingPatchJSON)
}

func TestModelListingsIncludeCatalogMetadata(t *testing.T) {
	e, ctx := startTestEngine(t)
	modeldatatest.Write(t, e.opts.DataDir)
	// The pricing fixture has limits and prices; add modalities to cover capability backfill.
	pricingPath, _ := modeldata.Paths(e.opts.DataDir)
	raw, err := os.ReadFile(pricingPath)
	require.NoError(t, err)
	var entries map[string]map[string]any
	require.NoError(t, json.Unmarshal(raw, &entries))
	entries["gpt-4o-mini"]["architecture"] = map[string]any{"input_modalities": []string{"text", "image"}, "output_modalities": []string{"text"}}
	raw, err = json.Marshal(entries)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pricingPath, raw, 0o600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	up := mockOpenAI(t)
	t.Cleanup(up.Close)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}, nil))
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "primary", "value": "secret", "aliases": map[string]any{"friendly": map[string]any{"model_id": "gpt-4o-mini"}}}, nil))
	for _, path := range []string{"/v1/models", "/v1/models?provider=openai"} {
		var out struct {
			Data []schemas.Model `json:"data"`
		}
		require.NoError(t, e.Do(ctx, http.MethodGet, path, nil, &out))
		for _, id := range []string{"openai/gpt-4o-mini", "openai/friendly"} {
			var found bool
			for _, m := range out.Data {
				if m.ID != id {
					continue
				}
				found = true
				require.NotNil(t, m.Pricing, id)
				require.NotNil(t, m.ContextLength)
				require.NotNil(t, m.MaxOutputTokens)
				require.EqualValues(t, 128000, *m.ContextLength)
				require.EqualValues(t, 16384, *m.MaxOutputTokens)
				require.NotNil(t, m.NormalizedName)
				require.NotNil(t, m.Architecture)
				require.Equal(t, []string{"text", "image"}, m.Architecture.InputModalities)
			}
			require.True(t, found, id)
		}
	}
	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	for _, m := range models.Models {
		if m.ID == "openai/gpt-4o-mini" || m.ID == "openai/friendly" {
			cost, ok := CostFromMetadata(m.Metadata)
			require.True(t, ok)
			require.Equal(t, 0.15, cost.Input)
			require.Equal(t, 0.6, cost.Output)
		}
	}
}
