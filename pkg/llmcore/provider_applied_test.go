package llmcore_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestProviderAppliedMatchesDesiredInputs(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "generic", llmcoreclient.Provider{
		Config: schemas.ProviderConfig{
			CustomProviderConfig: &schemas.CustomProviderConfig{
				BaseProviderType: schemas.OpenAI,
				IsKeyLess:        true,
			},
		},
	}))
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.False(t, (&llmcore.Engine{}).ProviderApplied("generic", loaded.Config), "no runtime exists before application")
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	require.True(t, engine.ProviderApplied("generic", loaded.Config), "keyless generic is a supported runtime")
	require.False(t, engine.ProviderApplied("missing", loaded.Config))
	require.False(t, engine.ProviderApplied("generic", llmcoreclient.Config{}))
	for _, change := range []string{"config", "proxy", "keys"} {
		t.Run(change, func(t *testing.T) {
			if change == "proxy" {
				require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
					settings["proxy"] = json.RawMessage(`{"enabled":true,"enable_for_inference":true,"url":"http://proxy.example"}`)
					return nil
				}))
			} else {
				require.NoError(t, store.MutateProvider(t.Context(), "generic", func(p *llmcoreclient.Provider) error {
					if change == "config" {
						p.Config.NetworkConfig.BaseURL = "https://replacement.example"
					} else {
						p.Keys = []schemas.Key{{
							ID:    "new-key",
							Name:  "new-key",
							Value: *schemas.NewSecretVar("new-secret"),
						}}
					}
					return nil
				}))
			}
			desired, err := store.Load(t.Context())
			require.NoError(t, err)
			require.False(t, engine.ProviderApplied("generic", desired.Config), "unapplied desired changes cannot authorize cleanup")
			_, err = engine.Sync(t.Context())
			require.NoError(t, err)
			require.True(t, engine.ProviderApplied("generic", desired.Config))
		})
	}
	require.NoError(t, engine.Shutdown(context.Background()))
	loaded, err = store.Load(t.Context())
	require.NoError(t, err)
	require.False(t, engine.ProviderApplied("generic", loaded.Config), "shutdown invalidates applied runtimes")
}

func TestProviderAppliedUsesLiveAzureInputsAfterPartialSync(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "azure", llmcoreclient.Provider{
		Keys: []schemas.Key{{
			ID:             "key",
			Name:           "key",
			Value:          *schemas.NewSecretVar("secret"),
			AzureKeyConfig: &schemas.AzureKeyConfig{Endpoint: *schemas.NewSecretVar("https://account.openai.azure.com")},
		}},
	}))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	require.NoError(t, store.CreateProvider(t.Context(), "broken", llmcoreclient.Provider{}))
	for _, change := range []string{"account", "discovery", "keys"} {
		t.Run(change, func(t *testing.T) {
			require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *llmcoreclient.Provider) error {
				switch change {
				case "account":
					p.AzureAccounts = map[string]llmcoreclient.AzureAccount{"key": {
						SubscriptionID: "subscription",
						ResourceGroup:  "group",
						AccountName:    "account",
					}}
				case "discovery":
					p.AzureDiscoveries = map[string]llmcoreclient.AzureDiscoverySnapshot{"key": {
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(p.Keys[0], p.AzureAccounts["key"]),
						Names:       []string{"deployment"},
					}}
				case "keys":
					p.Keys[0].Value = *schemas.NewSecretVar("rotated")
				}
				return nil
			}))
			desired, err := store.Load(t.Context())
			require.NoError(t, err)
			require.False(t, engine.ProviderApplied("azure", desired.Config))
			_, err = engine.Sync(t.Context())
			require.ErrorContains(t, err, "broken")
			require.True(t, engine.ProviderApplied("azure", desired.Config), "live inputs advance despite unrelated errors")
			require.False(t, engine.ProviderApplied("broken", desired.Config))
		})
	}
}
