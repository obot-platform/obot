//go:build e2e

package llmcore_test

import (
	"fmt"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestLocalRuleFallbackServesAfterGlobalUnregister(t *testing.T) {
	env := newTestEnv(t)
	const provider = "replica-rule-fallback"
	const fallbackKey = "fallback-specific-key"
	pc := providerConfig(env.mock.URL)
	pc.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}
	require.NoError(t, env.store.CreateProvider(t.Context(), provider, llmcoreclient.Provider{
		Config: pc,
		Keys:   []schemas.Key{apiKey("fallback-pin", fallbackKey)},
	}))
	for _, variant := range []string{"legacy", "legacy pinned", "structured pinned", "omitted model"} {
		t.Run(variant, func(t *testing.T) {
			fallback := tables.RoutingFallback{}
			switch variant {
			case "legacy", "legacy pinned":
				schemas.UnregisterKnownProvider(schemas.ModelProvider(provider))
				require.NoError(t, fallback.UnmarshalJSON([]byte(`"`+provider+`/namespace/backup"`)))
				if variant == "legacy pinned" {
					fallback.KeyID = "fallback-pin"
				}
			case "structured pinned":
				fallback.Fallback = schemas.Fallback{Provider: provider, Model: "namespace/backup", KeyID: "fallback-pin"}
			case "omitted model":
				fallback.Fallback = schemas.Fallback{Provider: provider, KeyID: "fallback-pin"}
			}
			// A second key makes lost fallback pins observable. In unpinned mode the
			// single configured key must still belong to the fallback provider.
			require.NoError(t, env.store.MutateProvider(t.Context(), provider, func(p *llmcoreclient.Provider) error {
				p.Keys = []schemas.Key{apiKey("fallback-pin", fallbackKey)}
				if variant != "legacy" {
					other := apiKey("wrong-pin", "wrong-fallback-key")
					// A zero-weight pinned key is never selected by the normal pool
					// while the other key has positive weight, making lost pins deterministic.
					p.Keys[0].Weight = 0
					p.Keys = append(p.Keys, other)
				}
				return nil
			}))
			rule := tables.TableRoutingRule{
				ID:            "local-rule-fallback",
				Name:          "local-rule-fallback",
				Scope:         "global",
				CelExpression: "true",
				// The caller model is healthy; routing changes only the primary to force
				// an actual upstream error. Omitted fallback models inherit the caller.
				Targets:         []tables.TableRoutingTarget{{Model: schemas.Ptr("gpt-e2e-broken"), KeyID: schemas.Ptr("openai-1"), Weight: 1}},
				ParsedFallbacks: []tables.RoutingFallback{fallback},
			}
			require.NoError(t, env.store.PutRule(t.Context(), rule))
			env.A.sync(t)
			env.B.sync(t)
			for _, replica := range []*replica{env.A, env.B} {
				for _, api := range allAPIs {
					for _, stream := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
							schemas.UnregisterKnownProvider(schemas.ModelProvider(provider))
							n := nonce("rule-fallback")
							result := replica.call(t, api, "openai/gpt-e2e-a", "hi "+n, stream, nil).requireOK(t)
							require.Equal(t, provider, result.Header.Get(headerServedProvider))
							requests := env.mock.InferenceRequests(n)
							require.GreaterOrEqual(t, len(requests), 2)
							require.Equal(t, "gpt-e2e-broken", requests[0].Model)
							last := requests[len(requests)-1]
							require.Equal(t, fallbackKey, last.Key, "fallback must use its own configured/pinned key, not the primary pin")
							model := "namespace/backup"
							if variant == "omitted model" {
								model = "gpt-e2e-a"
							}
							require.Equal(t, model, last.Model)
						})
					}
				}
				// Direct Client calls have no HTTP parsing snapshot and must use the same
				// replica-local source for rule fallbacks after global unregister.
				schemas.UnregisterKnownProvider(schemas.ModelProvider(provider))
				n := nonce("direct-rule-fallback")
				ctx, cancel := schemas.NewBifrostContextWithCancel(t.Context())
				result, err := replica.engine.Client().ChatCompletionRequest(ctx, &schemas.BifrostChatRequest{
					Provider: schemas.OpenAI,
					Model:    "gpt-e2e-a",
					Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi " + n)}}},
				})
				cancel()
				require.Nil(t, err)
				require.Equal(t, schemas.ModelProvider(provider), result.ExtraFields.Provider)
				requests := env.mock.InferenceRequests(n)
				require.GreaterOrEqual(t, len(requests), 2)
				require.Equal(t, fallbackKey, requests[len(requests)-1].Key)
			}
		})
	}
}
