package llmengine

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestAzureDeploymentModels(t *testing.T) {
	engine := New()
	cfg := llmcoreclient.Config{
		Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Azure: {}, "custom": {CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.Azure}}},
		Keys:      map[schemas.ModelProvider][]schemas.Key{},
	}

	for _, tc := range []struct {
		name string
		keys []schemas.Key
		want []string
	}{
		{
			name: "wildcard",
			keys: []schemas.Key{{Models: schemas.WhiteList{"*"}}},
			want: []string{},
		},
		{
			name: "regex",
			keys: []schemas.Key{{Models: schemas.WhiteList{"regex:.*"}}},
			want: []string{},
		},
		{
			name: "explicit",
			keys: []schemas.Key{{Models: schemas.WhiteList{"my-deployment"}}, {Models: schemas.WhiteList{"my-deployment", "other"}}},
			want: []string{"azure/my-deployment", "azure/other"},
		},
		{
			name: "disabled",
			keys: []schemas.Key{{Models: schemas.WhiteList{"hidden"}, Enabled: new(false)}},
			want: []string{},
		},
		{
			name: "blocked",
			keys: []schemas.Key{{Models: schemas.WhiteList{"hidden"}, BlacklistedModels: schemas.BlackList{"*"}}},
			want: []string{},
		},
		{
			name: "alias",
			keys: []schemas.Key{{Models: schemas.WhiteList{"*"}, Aliases: schemas.KeyAliases{"friendly": {ModelID: "deployment"}}}},
			want: []string{"azure/friendly"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg.Keys[schemas.Azure] = tc.keys
			models, azure := engine.azureDeploymentModels(cfg, schemas.Azure)
			require.True(t, azure)

			ids := []string{}

			for _, model := range models {
				ids = append(ids, model.ID)
			}

			require.Equal(t, tc.want, ids)
		})
	}

	cfg.Keys["custom"] = []schemas.Key{{Models: schemas.WhiteList{"deployment"}}}
	models, azure := engine.azureDeploymentModels(cfg, "custom")
	require.True(t, azure)
	require.Equal(t, "custom/deployment", models[0].ID)

	_, azure = engine.azureDeploymentModels(cfg, schemas.OpenAI)
	require.False(t, azure)
}

func TestAzureListsConfiguredDeploymentsWithoutUpstreamDiscovery(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, []byte(`{"provider":"azure"}`))))

	var key schemas.Key
	require.NoError(t, testMutation(ctx, e, &key)(e.CreateKey(ctx, "azure", []byte(`{"name":"test","value":"secret","models":["my-deployment"],"azure_key_config":{"endpoint":"https://example.openai.azure.com"}}`))))

	for _, path := range []string{"/v1/models", "/v1/models?provider=azure"} {
		var listed struct{ Data []schemas.Model }
		require.NoError(t, testInference(ctx, t, e, http.MethodGet, path, nil, &listed))
		require.Len(t, listed.Data, 1)
		require.Equal(t, "azure/my-deployment", listed.Data[0].ID)
	}

	// Listing needs no Azure token, including when inference uses Entra credentials.
	require.NoError(t, testMutation(ctx, e, nil)(e.UpdateKey(ctx, "azure", key.ID, []byte(`{"value":"","azure_key_config":{"client_id":"client","client_secret":"secret","tenant_id":"tenant"}}`))))
	_, err := e.listGatewayModels(ctx, schemas.Azure, true)
	require.NoError(t, err)

	catalog, err := e.ListCatalogModels(ctx, "azure", "")
	require.NoError(t, err)
	require.Len(t, catalog, 1)

	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	require.NoError(t, testMutation(ctx, e, nil)(e.UpdateKey(ctx, "azure", key.ID, []byte(`{"models":[]}`))))

	models, err = e.ListModels(ctx)
	require.NoError(t, err)
	require.Empty(t, models.Models)

	unavailable, err := e.UnavailableModels(ctx, []string{"azure/my-deployment"})
	require.NoError(t, err)
	require.True(t, unavailable["azure/my-deployment"])
}

func TestAzureAutomaticDeploymentPolicies(t *testing.T) {
	engine := New()
	for _, tc := range []struct {
		name       string
		provider   schemas.ModelProvider
		keys       []schemas.Key
		discovered map[string][]string
		want       []schemas.Model
	}{
		{
			name:       "no nonexistent allowlist backfill",
			provider:   schemas.Azure,
			keys:       []schemas.Key{{ID: "key", Models: schemas.WhiteList{"missing"}}},
			discovered: map[string][]string{"key": {"real"}},
			want:       []schemas.Model{},
		},
		{
			name:       "allowlist and blacklist",
			provider:   schemas.Azure,
			keys:       []schemas.Key{{ID: "key", Models: schemas.WhiteList{"real", "blocked", "missing"}, BlacklistedModels: schemas.BlackList{"blocked"}}},
			discovered: map[string][]string{"key": {"real", "blocked", "other"}},
			want:       []schemas.Model{{ID: "azure/real"}},
		},
		{
			name:     "combine enabled keys and deduplicate",
			provider: schemas.Azure,
			keys: []schemas.Key{
				{ID: "one", Models: schemas.WhiteList{"*"}},
				{ID: "two", Models: schemas.WhiteList{"*"}},
				{ID: "disabled", Models: schemas.WhiteList{"*"}, Enabled: new(false)},
			},
			discovered: map[string][]string{"one": {"real"}, "two": {"real", "second"}, "disabled": {"hidden"}},
			want:       []schemas.Model{{ID: "azure/real"}, {ID: "azure/second"}},
		},
		{
			name:       "custom namespace and alias routing target",
			provider:   "custom",
			keys:       []schemas.Key{{ID: "key", Models: schemas.WhiteList{"friendly"}, Aliases: schemas.KeyAliases{"friendly": {ModelID: "real"}}}},
			discovered: map[string][]string{"key": {"real"}},
			want:       []schemas.Model{{ID: "custom/friendly", Alias: new("real")}},
		},
		{
			name:       "confirmed empty cannot backfill aliases",
			provider:   schemas.Azure,
			keys:       []schemas.Key{{ID: "key", Models: schemas.WhiteList{"*"}, Aliases: schemas.KeyAliases{"friendly": {ModelID: "missing"}}}},
			discovered: map[string][]string{"key": {}},
			want:       []schemas.Model{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := llmcoreclient.Config{
				Providers: map[schemas.ModelProvider]schemas.ProviderConfig{tc.provider: {CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.Azure}}},
				Keys:      map[schemas.ModelProvider][]schemas.Key{tc.provider: tc.keys},
			}

			cfg.AzureAccounts = map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{tc.provider: {}}
			cfg.AzureDiscoveries = map[schemas.ModelProvider]map[string]llmcoreclient.AzureDiscoverySnapshot{tc.provider: {}}
			for _, key := range tc.keys {
				if names, automatic := tc.discovered[key.ID]; automatic {
					account := llmcoreclient.AzureAccount{
						SubscriptionID: "sub",
						ResourceGroup:  "group",
						AccountName:    key.ID,
					}
					cfg.AzureAccounts[tc.provider][key.ID] = account
					cfg.AzureDiscoveries[tc.provider][key.ID] = llmcoreclient.AzureDiscoverySnapshot{
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, account),
						Names:       names,
					}
				}
			}

			models, azure := engine.azureDeploymentModels(cfg, tc.provider)
			require.True(t, azure)
			require.Equal(t, tc.want, models)
		})
	}
}

func BenchmarkAzureUnavailableModels(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			key := azureTestKey("key")
			account := llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
			names, ids := make([]string, count), make([]string, count)

			for i := range names {
				names[i] = fmt.Sprintf("deployment-%04d", i)
				ids[i] = "azure/" + names[i]
			}

			cfg := llmcoreclient.Config{
				Providers:        map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Azure: {}},
				Keys:             map[schemas.ModelProvider][]schemas.Key{schemas.Azure: {key}},
				AzureAccounts:    map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{schemas.Azure: {key.ID: account}},
				AzureDiscoveries: map[schemas.ModelProvider]map[string]llmcoreclient.AzureDiscoverySnapshot{schemas.Azure: {key.ID: {Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, account), Names: names}}},
			}

			ctx := context.WithValue(context.Background(), configContextKey{}, &cfg)
			e := New()
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				unavailable, err := e.UnavailableModels(ctx, ids)
				if err != nil || len(unavailable) != 0 {
					b.Fatalf("unexpected membership: %v, %v", unavailable, err)
				}
			}
		})
	}
}
