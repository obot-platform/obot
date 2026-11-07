package llmengine

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/modeldata"
	"github.com/stretchr/testify/require"
)

func installAzureMetadataProvider(ctx context.Context, t *testing.T, e *Engine, provider llmcoreclient.Provider) {
	t.Helper()

	provider.AzureDiscoveries = map[string]llmcoreclient.AzureDiscoverySnapshot{}
	for _, key := range provider.Keys {
		provider.AzureDiscoveries[key.ID] = llmcoreclient.AzureDiscoverySnapshot{
			Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, provider.AzureAccounts[key.ID]),
			Names:       []string{"deployment"},
			BaseModels:  map[string]string{"deployment": "current-base"},
		}
	}
	require.NoError(t, e.store.CreateProvider(ctx, "azure", provider))
	_, err := e.Sync(ctx)
	require.NoError(t, err)
}

func TestAzureMetadataUsesTheKeyThatAdvertisedTheID(t *testing.T) {
	for _, exclusion := range []string{"allowlist", "blacklist", "disabled", "stale", "manual first"} {
		t.Run(exclusion, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			path, _ := modeldata.Paths(e.opts.DataDir)
			require.NoError(t, os.WriteFile(path, []byte(`{
 "old-base":{"provider":"azure","mode":"chat","is_deprecated":true,"max_input_tokens":10},
 "current-base":{"provider":"azure","mode":"chat","max_input_tokens":2000,"input_cost_per_token":0.000002}
 }`), 0600))
			require.NoError(t, e.prices.ForceReloadPricing(ctx))
			first, second := azureTestKey("first"), azureTestKey("second")
			first.Models = schemas.WhiteList{"friendly"}
			second.Models = schemas.WhiteList{"friendly"}
			first.Aliases = schemas.KeyAliases{"friendly": {ModelID: "deployment"}}
			second.Aliases = schemas.KeyAliases{"friendly": {ModelID: "deployment"}}
			switch exclusion {
			case "allowlist":
				first.Models = schemas.WhiteList{"other"}
			case "blacklist":
				first.BlacklistedModels = schemas.BlackList{"friendly"}
			case "disabled":
				first.Enabled = new(false)
			}
			provider := azureTestProvider(first, second)
			installAzureMetadataProvider(ctx, t, e, provider)
			require.NoError(t, e.store.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
				snapshot := p.AzureDiscoveries[first.ID]
				snapshot.BaseModels["deployment"] = "old-base"
				if exclusion == "stale" {
					snapshot.Fingerprint = [32]byte{}
				}
				p.AzureDiscoveries[first.ID] = snapshot
				if exclusion == "manual first" {
					delete(p.AzureAccounts, first.ID)
				}
				return nil
			}))

			models, err := e.gatewayModels(ctx, schemas.Azure)
			require.NoError(t, err)
			require.Len(t, models, 1, "metadata from an excluded key must not hide the eligible deployment")
			require.Equal(t, "azure/friendly", models[0].ID)
			require.Equal(t, "deployment", *models[0].Alias)
			if exclusion == "manual first" {
				require.Nil(t, models[0].ContextLength, "a later account cannot supply metadata for the first manual advertiser")
			} else {
				require.NotNil(t, models[0].ContextLength)
				require.Equal(t, 2000, *models[0].ContextLength)
				require.NotNil(t, models[0].Pricing)
				cost, err := strconv.ParseFloat(*models[0].Pricing.Prompt, 64)
				require.NoError(t, err)
				require.Equal(t, 0.000002, cost)
			}
		})
	}
}

func TestAzureUsageUsesBaseModelAndPreservesWireIDs(t *testing.T) {
	for _, tc := range []struct {
		base  string
		usage types.ModelUsage
		alias bool
	}{
		{
			base:  "tts-1",
			usage: types.ModelUsageOther,
		},
		{
			base:  "whisper-1",
			alias: true,
			usage: types.ModelUsageOther,
		},
		{
			base:  "text-embedding-3-small",
			usage: types.ModelUsageEmbedding,
		},
		{
			base:  "dall-e-3",
			alias: true,
			usage: types.ModelUsageImage,
		},
	} {
		t.Run(tc.base, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			key := azureTestKey("key")
			expectedID := "azure/deployment"
			if tc.alias {
				key.Aliases = schemas.KeyAliases{"friendly": {ModelID: "deployment"}}
				key.Models = schemas.WhiteList{"friendly"}
				expectedID = "azure/friendly"
			}
			installAzureMetadataProvider(ctx, t, e, azureTestProvider(key))
			require.NoError(t, e.store.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
				snapshot := p.AzureDiscoveries[key.ID]
				snapshot.BaseModels["deployment"] = tc.base
				p.AzureDiscoveries[key.ID] = snapshot
				return nil
			}))

			models, err := e.ListModels(ctx)
			require.NoError(t, err)
			require.Len(t, models.Models, 1)
			require.Equal(t, expectedID, models.Models[0].ID)
			require.Equal(t, string(tc.usage), models.Models[0].Metadata["usage"])
			listed, err := e.gatewayModels(ctx, schemas.Azure)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			if tc.alias {
				require.Equal(t, "deployment", *listed[0].Alias)
			} else {
				require.Nil(t, listed[0].Alias)
			}
		})
	}
}

func TestAzureBaseModelRetirementCannotRestoreCachedDeployments(t *testing.T) {
	e, ctx := startTestEngine(t)
	key := azureTestKey("key")
	key.Aliases = schemas.KeyAliases{"friendly": {ModelID: "deployment"}}
	key.Models = schemas.WhiteList{"friendly"}
	installAzureMetadataProvider(ctx, t, e, azureTestProvider(key))
	path, _ := modeldata.Paths(e.opts.DataDir)
	require.NoError(t, os.WriteFile(path, []byte(`{"current-base":{"provider":"azure","mode":"chat"}}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 1, "prime the last-known listing before retirement")

	require.NoError(t, os.WriteFile(path, []byte(`{"current-base":{"provider":"azure","mode":"chat","is_deprecated":true}}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	unavailable, err := e.UnavailableModels(ctx, []string{"azure/friendly"})
	require.NoError(t, err)
	require.True(t, unavailable["azure/friendly"], "retention must check the underlying model before any new public listing")
	models, err = e.ListModels(ctx)
	require.NoError(t, err)
	require.Empty(t, models.Models, "an empty live response must not restore a cached retired deployment")
	catalog, err := e.ListCatalogModels(ctx, "azure", "")
	require.NoError(t, err)
	require.Empty(t, catalog)
}

func TestAzureAliasCollisionUsesTheActualDeploymentMetadata(t *testing.T) {
	for _, outcome := range []string{"missing", "active", "retired"} {
		t.Run(outcome, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			key := azureTestKey("key")
			key.Models = schemas.WhiteList{"blue"}
			key.Aliases = schemas.KeyAliases{"blue": {ModelID: "green"}}
			provider := azureTestProvider(key)
			blueBase, greenBase := "retired-base", "tts-1"
			names := []string{"blue", "green"}
			if outcome == "missing" {
				names = []string{"blue"}
				blueBase = "tts-1"
			}
			if outcome == "retired" {
				blueBase, greenBase = "tts-1", "retired-base"
			}
			provider.AzureDiscoveries = map[string]llmcoreclient.AzureDiscoverySnapshot{key.ID: {
				Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, provider.AzureAccounts[key.ID]),
				Names:       names,
				BaseModels:  map[string]string{"blue": blueBase, "green": greenBase},
			}}
			require.NoError(t, e.store.CreateProvider(ctx, "azure", provider))
			_, err := e.Sync(ctx)
			require.NoError(t, err)
			path, _ := modeldata.Paths(e.opts.DataDir)
			require.NoError(t, os.WriteFile(path, []byte(`{
 "retired-base":{"provider":"azure","mode":"chat","is_deprecated":true},
 "tts-1":{"provider":"azure","mode":"audio_speech","input_cost_per_token":0.000003}
 }`), 0600))
			require.NoError(t, e.prices.ForceReloadPricing(ctx))

			models, err := e.gatewayModels(ctx, schemas.Azure)
			require.NoError(t, err)
			unavailable, err := e.UnavailableModels(ctx, []string{"azure/blue"})
			require.NoError(t, err)
			if outcome != "active" {
				require.Empty(t, models)
				require.True(t, unavailable["azure/blue"])
				return
			}
			require.Len(t, models, 1)
			require.Equal(t, "azure/blue", models[0].ID)
			require.NotNil(t, models[0].Alias)
			require.Equal(t, "green", *models[0].Alias)
			require.Equal(t, "tts-1", azureBaseModel(models[0]))
			require.NotNil(t, models[0].Pricing)
			cost, err := strconv.ParseFloat(*models[0].Pricing.Prompt, 64)
			require.NoError(t, err)
			require.Equal(t, 0.000003, cost)
			require.False(t, unavailable["azure/blue"])
			listed, err := e.ListModels(ctx)
			require.NoError(t, err)
			require.Len(t, listed.Models, 1)
			require.Equal(t, string(types.ModelUsageOther), listed.Models[0].Metadata["usage"])
		})
	}
}

func TestAzureDuplicateDeploymentsKeepAnActiveContributor(t *testing.T) {
	for _, alias := range []bool{false, true} {
		for _, order := range []string{"retired first", "active first", "all retired"} {
			t.Run(fmt.Sprintf("%s/alias=%t", order, alias), func(t *testing.T) {
				e, ctx := startTestEngine(t)
				path, _ := modeldata.Paths(e.opts.DataDir)
				require.NoError(t, os.WriteFile(path, []byte(`{
 "retired-base":{"provider":"azure","mode":"chat","is_deprecated":true},
 "tts-1":{"provider":"azure","mode":"audio_speech","input_cost_per_token":0.000003}
 }`), 0600))
				require.NoError(t, e.prices.ForceReloadPricing(ctx))
				retired, active := azureTestKey("retired"), azureTestKey("active")
				retiredTarget, activeTarget := "deployment", "deployment"
				advertised := "azure/deployment"
				if alias {
					retiredTarget, activeTarget = "old-deployment", "current-deployment"
					advertised = "azure/friendly"
					retired.Models, active.Models = schemas.WhiteList{"friendly"}, schemas.WhiteList{"friendly"}
					retired.Aliases = schemas.KeyAliases{"friendly": {ModelID: retiredTarget}}
					active.Aliases = schemas.KeyAliases{"friendly": {ModelID: activeTarget}}
				}
				provider := azureTestProvider(retired, active)
				if order == "active first" {
					provider.Keys = []schemas.Key{active, retired}
				}
				activeBase := "tts-1"
				if order == "all retired" {
					activeBase = "retired-base"
				}
				provider.AzureDiscoveries = map[string]llmcoreclient.AzureDiscoverySnapshot{
					retired.ID: {
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(retired, provider.AzureAccounts[retired.ID]),
						Names:       []string{retiredTarget},
						BaseModels:  map[string]string{retiredTarget: "retired-base"},
					},
					active.ID: {
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(active, provider.AzureAccounts[active.ID]),
						Names:       []string{activeTarget},
						BaseModels:  map[string]string{activeTarget: activeBase},
					},
				}
				require.NoError(t, e.store.CreateProvider(ctx, "azure", provider))
				_, err := e.Sync(ctx)
				require.NoError(t, err)

				models, err := e.gatewayModels(ctx, schemas.Azure)
				require.NoError(t, err)
				unavailable, err := e.UnavailableModels(ctx, []string{advertised})
				require.NoError(t, err)
				listed, err := e.ListModels(ctx)
				require.NoError(t, err)
				catalog, err := e.ListCatalogModels(ctx, "azure", "")
				require.NoError(t, err)
				if order == "all retired" {
					require.Empty(t, models)
					require.True(t, unavailable[advertised])
					require.Empty(t, listed.Models)
					require.Empty(t, catalog)
					return
				}
				require.Len(t, models, 1)
				require.Equal(t, advertised, models[0].ID)
				require.Equal(t, "tts-1", azureBaseModel(models[0]))
				if alias {
					require.NotNil(t, models[0].Alias)
					require.Equal(t, activeTarget, *models[0].Alias)
				}
				require.NotNil(t, models[0].Pricing)
				cost, err := strconv.ParseFloat(*models[0].Pricing.Prompt, 64)
				require.NoError(t, err)
				require.Equal(t, 0.000003, cost)
				require.False(t, unavailable[advertised])
				require.Len(t, listed.Models, 1)
				require.Equal(t, string(types.ModelUsageOther), listed.Models[0].Metadata["usage"])
				require.Len(t, catalog, 1)
			})
		}
	}
}
