package llmengine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"

	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/modeldata"

	"github.com/stretchr/testify/require"
)

func TestModelDiscoveryOmitsCatalogDeprecatedModels(t *testing.T) {
	e, ctx := startTestEngine(t)
	path, _ := modeldata.Paths(e.opts.DataDir)
	require.NoError(t, os.WriteFile(path, []byte(`{
		"babbage-002":{"provider":"text-completion-openai","mode":"completion","is_deprecated":true},
		"old-chat":{"provider":"openai","mode":"chat","is_deprecated":true},
		"current":{"provider":"openai","mode":"chat"},
		"future-retirement":{"provider":"openai","mode":"chat","deprecation_date":"2099-01-01"}
	}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "babbage-002"}, {"id": "old-chat"}, {"id": "current"}, {"id": "unknown-new"}, {"id": "future-retirement"},
		}})
	}))
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "test", "value": "secret"}))))
	var listed struct{ Data []schemas.Model }
	require.NoError(t, testInference(ctx, t, e, http.MethodGet, "/v1/models", nil, &listed))
	ids := []string{}
	for _, model := range listed.Data {
		ids = append(ids, model.ID)
	}
	require.ElementsMatch(t, []string{"openai/current", "openai/unknown-new", "openai/future-retirement"}, ids)
	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 3)
	var admin struct{ Models []struct{ Name string } }
	require.NoError(t, testResult(&admin, "models")(e.ListCatalogModels(ctx, "openai", "")))
	for _, model := range admin.Models {
		require.NotEqual(t, "babbage-002", model.Name)
		require.NotEqual(t, "old-chat", model.Name)
	}
}

func TestBedrockDiscoveryOmitsUnavailableModels(t *testing.T) {
	for _, tc := range []struct {
		name           string
		model          string
		status         string
		inferenceTypes []string
	}{
		{
			name:           "legacy",
			model:          "ai21.jamba-1-5-large-v1:0",
			status:         "LEGACY",
			inferenceTypes: []string{"ON_DEMAND"},
		},
		{
			name:           "provisioned only",
			model:          "amazon.nova-lite-v1:0:24k",
			status:         "ACTIVE",
			inferenceTypes: []string{"PROVISIONED"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			var unavailable atomic.Bool
			up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/foundation-models" {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
					return
				}
				status := "ACTIVE"
				inferenceTypes := []string{"ON_DEMAND", "PROVISIONED"}
				if unavailable.Load() {
					status = tc.status
					inferenceTypes = tc.inferenceTypes
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"modelSummaries": []map[string]any{
					{"modelId": tc.model, "modelName": "Test model", "modelLifecycle": map[string]any{"status": status}, "inferenceTypesSupported": inferenceTypes},
				}})
			}))
			t.Cleanup(up.Close)
			require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "bedrock", "network_config": map[string]any{"insecure_skip_verify": true, "allow_private_network": true}}))))
			require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "bedrock", testJSON(t, map[string]any{
				"name": "test", "value": "test-bearer", "bedrock_key_config": map[string]any{
					"region": "us-east-1", "endpoints": map[string]any{"control_plane": up.URL, "mantle": up.URL},
				},
			}))))
			models, err := e.ListModels(ctx)
			require.NoError(t, err)
			require.Len(t, models.Models, 1, "models supporting both inference types remain available")
			unavailable.Store(true)
			var listed struct{ Data []schemas.Model }
			require.NoError(t, testInference(ctx, t, e, http.MethodGet, "/v1/models", nil, &listed))
			require.Empty(t, listed.Data, "unavailable models must disappear from public discovery")
			models, err = e.ListModels(ctx)
			require.NoError(t, err)
			require.Empty(t, models.Models, "last-known fallback must not resurrect the unavailable model")
			var admin struct{ Models []struct{ Name string } }
			require.NoError(t, testResult(&admin, "models")(e.ListCatalogModels(ctx, "bedrock", "")))
			require.Empty(t, admin.Models)
		})
	}
}

func TestCachedAliasesOmitDeprecatedTargets(t *testing.T) {
	e, ctx := startTestEngine(t)
	path, _ := modeldata.Paths(e.opts.DataDir)
	require.NoError(t, os.WriteFile(path, []byte(`{
		"old-chat":{"provider":"openai","mode":"chat"},
		"current":{"provider":"openai","mode":"chat"}
	}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	var empty atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		models := []map[string]string{}
		if !empty.Load() {
			models = append(models, map[string]string{"id": "old-chat"}, map[string]string{"id": "current"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": models})
	}))
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{
		"provider":       "openai",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true},
	}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{
		"name": "test", "value": "secret",
		"aliases": map[string]any{"friendly": map[string]any{"model_id": "old-chat"}},
	}))))
	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 3)

	// An empty live listing reuses cached models after the target's pricing metadata changes.
	empty.Store(true)
	require.NoError(t, os.WriteFile(path, []byte(`{
		"old-chat":{"provider":"openai","mode":"chat","is_deprecated":true},
		"current":{"provider":"openai","mode":"chat"}
	}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	models, err = e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 1, "neither the deprecated target nor its cached alias may return")
	require.Equal(t, "openai/current", models.Models[0].ID, "available cached models remain discoverable")
}

func TestModelAvailabilityRecoversOnSuccessfulListing(t *testing.T) {
	availability := &modelAvailability{unavailable: map[string]bool{}}
	ctx := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	defer ctx.Cancel()
	ctx.SetValue(modelAvailabilityProviderKey{}, schemas.OpenAI)
	response := &schemas.BifrostResponse{
		ListModelsResponse: &schemas.BifrostListModelsResponse{
			Data: []schemas.Model{{
				ID:           "openai/test",
				IsDeprecated: true,
			}},
		},
	}
	_, _, err := availability.PostLLMHook(ctx, response, nil)
	require.NoError(t, err)
	require.True(t, availability.isUnavailable("openai/test"))

	response.ListModelsResponse.Data[0].IsDeprecated = false
	_, _, err = availability.PostLLMHook(ctx, response, &schemas.BifrostError{})
	require.NoError(t, err)
	require.True(t, availability.isUnavailable("openai/test"), "failed listings must retain the previous status")
	_, _, err = availability.PostLLMHook(ctx, &schemas.BifrostResponse{
		ListModelsResponse: &schemas.BifrostListModelsResponse{},
	}, nil)
	require.NoError(t, err)
	require.True(t, availability.isUnavailable("openai/test"), "absent models must retain their previous status")

	_, _, err = availability.PostLLMHook(ctx, response, nil)
	require.NoError(t, err)
	require.False(t, availability.isUnavailable("openai/test"), "a successful active listing must clear stale deprecation")
}

func TestHTTPModelListingOmitsAliasesWithActiveNamesAndDeprecatedTargets(t *testing.T) {
	e, ctx := startTestEngine(t)
	path, _ := modeldata.Paths(e.opts.DataDir)
	require.NoError(t, os.WriteFile(path, []byte(`{
		"retired":{"provider":"openai","mode":"chat","is_deprecated":true},
		"friendly":{"provider":"openai","mode":"chat"}
	}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"retired"}]}`))
	}))
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{
		"provider":       "openai",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true},
	}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{
		"name":    "test",
		"value":   "secret",
		"aliases": map[string]any{"friendly": map[string]any{"model_id": "retired"}},
	}))))
	for _, path := range []string{"/v1/models", "/v1/models?provider=openai"} {
		var listed struct{ Data []schemas.Model }
		require.NoError(t, testInference(ctx, t, e, http.MethodGet, path, nil, &listed))
		require.Empty(t, listed.Data, "an active advertised name must not hide a deprecated target")
	}
	var catalog struct{ Models []struct{ Name string } }
	require.NoError(t, testResult(&catalog, "models")(e.ListCatalogModels(ctx, "openai", "")))
	require.Empty(t, catalog.Models)
	ids := []string{"openai/friendly", "openai/retired", "openai/unknown"}
	unavailable, err := e.UnavailableModels(ctx, ids)
	require.NoError(t, err)
	require.True(t, unavailable["openai/friendly"])
	require.True(t, unavailable["openai/retired"])
	require.False(t, unavailable["openai/unknown"])

	// An enabled key with an available target keeps a shared alias discoverable.
	key, err := e.CreateKey(ctx, "openai", testJSON(t, map[string]any{
		"name": "available", "value": "secret",
		"aliases": map[string]any{"friendly": map[string]any{"model_id": "current"}},
	}))
	require.NoError(t, err)
	unavailable, err = e.UnavailableModels(ctx, ids)
	require.NoError(t, err)
	require.False(t, unavailable["openai/friendly"])
	_, err = e.UpdateKey(ctx, "openai", key.ID, testJSON(t, map[string]any{"enabled": false}))
	require.NoError(t, err)
	unavailable, err = e.UnavailableModels(ctx, ids)
	require.NoError(t, err)
	require.True(t, unavailable["openai/friendly"])
}

func TestModelDiscoveryUsesAliasTargetMetadata(t *testing.T) {
	e, ctx := startTestEngine(t)
	path, _ := modeldata.Paths(e.opts.DataDir)
	require.NoError(t, os.WriteFile(path, []byte(`{
		"old-chat":{"provider":"openai","mode":"chat","is_deprecated":true,"input_cost_per_token":0.01},
		"current":{"provider":"openai","mode":"chat","input_cost_per_token":0.001}
	}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"current"}]}`))
	}))
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{
		"provider":       "openai",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true},
	}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{
		"name": "test", "value": "secret",
		"models":  []string{"old-chat"},
		"aliases": map[string]any{"old-chat": map[string]any{"model_id": "current"}},
	}))))
	for _, path := range []string{"/v1/models", "/v1/models?provider=openai"} {
		var listed struct{ Data []schemas.Model }
		require.NoError(t, testInference(ctx, t, e, http.MethodGet, path, nil, &listed))
		require.Len(t, listed.Data, 1)
		require.Equal(t, "openai/old-chat", listed.Data[0].ID)
		require.False(t, listed.Data[0].IsDeprecated)
		require.NotNil(t, listed.Data[0].Pricing)
		require.Equal(t, "0.0010000000", *listed.Data[0].Pricing.Prompt)
	}
	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	unavailable, err := e.UnavailableModels(ctx, []string{"openai/old-chat"})
	require.NoError(t, err)
	require.Empty(t, unavailable, "retention must preserve aliases with active targets")
	catalog, err := e.ListCatalogModels(ctx, "openai", "old-chat")
	require.NoError(t, err)
	require.Len(t, catalog, 1)
}

func TestAliasAvailabilityRequiresEligibleAlternativeKeys(t *testing.T) {
	e := &Engine{availability: &modelAvailability{unavailable: map[string]bool{"openai/retired": true}}}
	aliasKey := schemas.Key{
		Models:  schemas.WhiteList{"friendly"},
		Aliases: schemas.KeyAliases{"friendly": {ModelID: "retired"}},
	}
	for _, tc := range []struct {
		name        string
		alternative schemas.Key
		unavailable bool
	}{
		{
			name:        "unrelated allowlist",
			alternative: schemas.Key{Models: schemas.WhiteList{"other"}},
			unavailable: true,
		},
		{
			name: "blacklisted alias",
			alternative: schemas.Key{
				Models:            schemas.WhiteList{"*"},
				BlacklistedModels: schemas.BlackList{"friendly"},
			},
			unavailable: true,
		},
		{
			name: "regex blacklist",
			alternative: schemas.Key{
				Models:            schemas.WhiteList{"*"},
				BlacklistedModels: schemas.BlackList{"regex:friend.*"},
			},
			unavailable: true,
		},
		{
			name:        "eligible direct model",
			alternative: schemas.Key{Models: schemas.WhiteList{"friendly"}},
		},
		{
			name: "eligible active alias",
			alternative: schemas.Key{
				Models:  schemas.WhiteList{"friendly"},
				Aliases: schemas.KeyAliases{"FRIENDLY": {ModelID: "current"}},
			},
		},
		{
			name: "disabled alternative",
			alternative: schemas.Key{
				Models:  schemas.WhiteList{"*"},
				Enabled: new(false),
			},
			unavailable: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.unavailable, e.modelWithKeysIsUnavailable("openai/friendly", []schemas.Key{aliasKey, tc.alternative}))
		})
	}
	require.False(t, e.modelWithKeysIsUnavailable("openai/unknown", nil), "missing credentials alone must not disable outage retention")
}
