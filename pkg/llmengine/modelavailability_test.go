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
	require.NoError(t, testMutation(e, ctx, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}))))
	require.NoError(t, testMutation(e, ctx, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "test", "value": "secret"}))))
	var listed struct{ Data []schemas.Model }
	require.NoError(t, testInference(t, e, ctx, http.MethodGet, "/v1/models", nil, &listed))
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
			require.NoError(t, testMutation(e, ctx, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "bedrock", "network_config": map[string]any{"insecure_skip_verify": true, "allow_private_network": true}}))))
			require.NoError(t, testMutation(e, ctx, nil)(e.CreateKey(ctx, "bedrock", testJSON(t, map[string]any{
				"name": "test", "value": "test-bearer", "bedrock_key_config": map[string]any{
					"region": "us-east-1", "endpoints": map[string]any{"control_plane": up.URL, "mantle": up.URL},
				},
			}))))
			models, err := e.ListModels(ctx)
			require.NoError(t, err)
			require.Len(t, models.Models, 1, "models supporting both inference types remain available")
			unavailable.Store(true)
			var listed struct{ Data []schemas.Model }
			require.NoError(t, testInference(t, e, ctx, http.MethodGet, "/v1/models", nil, &listed))
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
	require.NoError(t, testMutation(e, ctx, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{
		"provider":       "openai",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true},
	}))))
	require.NoError(t, testMutation(e, ctx, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{
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
