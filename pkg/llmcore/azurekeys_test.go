package llmcore

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestAzureKeysStayWithinDiscoveredAccounts(t *testing.T) {
	for _, provider := range []schemas.ModelProvider{schemas.Azure, "custom"} {
		t.Run(string(provider), func(t *testing.T) {
			one := schemas.Key{
				ID:      "one",
				Models:  schemas.WhiteList{"*"},
				Aliases: schemas.KeyAliases{"friendly": {ModelID: "only-one"}},
			}
			two := schemas.Key{
				ID:     "two",
				Models: schemas.WhiteList{"*"},
			}
			firstAccount := llmcoreclient.AzureAccount{
				SubscriptionID: "sub",
				ResourceGroup:  "group",
				AccountName:    "first",
			}
			secondAccount := llmcoreclient.AzureAccount{
				SubscriptionID: "sub",
				ResourceGroup:  "group",
				AccountName:    "second",
			}
			cfg := llmcoreclient.Config{
				Providers:     map[schemas.ModelProvider]schemas.ProviderConfig{provider: {CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.Azure}}},
				Keys:          map[schemas.ModelProvider][]schemas.Key{provider: {one, two}},
				AzureAccounts: map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{provider: {one.ID: firstAccount, two.ID: secondAccount}},
				AzureDiscoveries: map[schemas.ModelProvider]map[string]llmcoreclient.AzureDiscoverySnapshot{provider: {
					one.ID: {
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(one, firstAccount),
						Names:       []string{"only-one"},
					},
					two.ID: {
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(two, secondAccount),
						Names:       []string{"only-two"},
					},
				}},
			}

			account := &account{}
			account.config.Store(&cfg)

			keys, err := account.GetKeysForProvider(t.Context(), provider)
			require.NoError(t, err)
			require.True(t, keys[0].Models.IsAllowed("only-one"))
			require.False(t, keys[1].Models.IsAllowed("only-one"), "the other account must not be eligible for this deployment")
			require.True(t, keys[1].Models.IsAllowed("only-two"))
			require.False(t, keys[0].Models.IsAllowed("only-two"))
			require.True(t, keys[0].Models.IsAllowed("friendly"))
			require.Equal(t, "only-one", keys[0].Aliases.Resolve("friendly"))
			require.False(t, keys[1].Models.IsAllowed("friendly"))
			require.Equal(t, schemas.WhiteList{"*"}, cfg.Keys[provider][0].Models, "runtime filtering must not mutate saved configuration")

			// Existing policy still restricts the discovered pool, including alias names.
			cfg.Keys[provider][0].Models = schemas.WhiteList{"friendly"}
			keys, err = account.GetKeysForProvider(t.Context(), provider)
			require.NoError(t, err)
			require.True(t, keys[0].Models.IsAllowed("friendly"))
			require.False(t, keys[0].Models.IsAllowed("only-one"))

			cfg.Keys[provider][0].BlacklistedModels = schemas.BlackList{"friendly"}
			keys, err = account.GetKeysForProvider(t.Context(), provider)
			require.NoError(t, err)
			require.Empty(t, keys[0].Models)

			// A confirmed empty or unknown discovery cannot borrow another account's models.
			delete(cfg.AzureDiscoveries[provider], two.ID)
			keys, err = account.GetKeysForProvider(t.Context(), provider)
			require.NoError(t, err)
			require.Empty(t, keys[1].Models)

			cfg.AzureDiscoveries[provider][two.ID] = llmcoreclient.AzureDiscoverySnapshot{
				Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(two, secondAccount),
				Names:       []string{},
			}
			keys, err = account.GetKeysForProvider(t.Context(), provider)
			require.NoError(t, err)
			require.Empty(t, keys[1].Models)

			cfg.AzureDiscoveries[provider][two.ID] = llmcoreclient.AzureDiscoverySnapshot{
				Fingerprint: [32]byte{},
				Names:       []string{"only-two"},
			}
			keys, err = account.GetKeysForProvider(t.Context(), provider)
			require.NoError(t, err)
			require.Empty(t, keys[1].Models, "a stale snapshot cannot authorize a key")

			// Keys without automatic discovery retain their configured routing behavior.
			delete(cfg.AzureAccounts[provider], two.ID)
			keys, err = account.GetKeysForProvider(t.Context(), provider)
			require.NoError(t, err)
			require.Equal(t, schemas.WhiteList{"*"}, keys[1].Models)
		})
	}
}

func TestAzureAliasCollisionsRespectTheActualTarget(t *testing.T) {
	for _, provider := range []schemas.ModelProvider{schemas.Azure, "custom"} {
		for _, tc := range []struct {
			name        string
			deployments []string
			want        []string
		}{
			{
				name:        "missing target",
				deployments: []string{"blue"},
				want:        []string{},
			},
			{
				name:        "discovered target",
				deployments: []string{"blue", "green"},
				want:        []string{"blue", "green"},
			},
		} {
			t.Run(string(provider)+"/"+tc.name, func(t *testing.T) {
				key := schemas.Key{
					ID:      "key",
					Models:  schemas.WhiteList{"*"},
					Aliases: schemas.KeyAliases{"blue": {ModelID: "green"}},
				}
				resource := llmcoreclient.AzureAccount{
					SubscriptionID: "sub",
					ResourceGroup:  "group",
					AccountName:    "account",
				}
				cfg := llmcoreclient.Config{
					Providers: map[schemas.ModelProvider]schemas.ProviderConfig{provider: {
						CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.Azure},
					}},
					Keys:          map[schemas.ModelProvider][]schemas.Key{provider: {key}},
					AzureAccounts: map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{provider: {key.ID: resource}},
					AzureDiscoveries: map[schemas.ModelProvider]map[string]llmcoreclient.AzureDiscoverySnapshot{provider: {key.ID: {
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, resource),
						Names:       tc.deployments,
					}}},
				}
				runtime := &account{}
				runtime.config.Store(&cfg)

				keys, err := runtime.GetKeysForProvider(t.Context(), provider)
				require.NoError(t, err)
				require.ElementsMatch(t, tc.want, keys[0].Models)
				require.Equal(t, schemas.WhiteList{"*"}, cfg.Keys[provider][0].Models)
			})
		}
	}
}
