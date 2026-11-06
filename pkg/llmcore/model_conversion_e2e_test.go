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

func TestUnavailableCustomProviderCanRouteAndFallback(t *testing.T) {
	env := newTestEnv(t)
	healthy := llmcoreclient.Provider{
		Config: providerConfig(env.mock.URL),
		Keys:   []schemas.Key{apiKey("fallback-key", openAIKey)},
	}
	healthy.Config.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}
	require.NoError(t, env.store.CreateProvider(t.Context(), "local-fallback", healthy))
	failed := transitionProvider(env.mock.URL, "must-not-reach-upstream")
	failed.Config.CustomProviderConfig.BaseProviderType = "unsupported-base"
	require.NoError(t, env.store.CreateProvider(t.Context(), "local-pending", failed))

	for _, replica := range []*replica{env.A, env.B} {
		_, err := replica.engine.Sync(t.Context())
		require.ErrorContains(t, err, "unsupported-base")
	}

	for _, mode := range []string{"caller fallback", "routing rule"} {
		if mode == "routing rule" {
			rule := tables.TableRoutingRule{
				ID:            "pending-route",
				Name:          "pending-route",
				Scope:         "global",
				CelExpression: "true",
				Targets:       []tables.TableRoutingTarget{{Provider: schemas.Ptr("local-fallback"), Model: schemas.Ptr("gpt-e2e-a"), Weight: 1}},
			}
			require.NoError(t, env.store.PutRule(t.Context(), rule))
			for _, replica := range []*replica{env.A, env.B} {
				_, err := replica.engine.Sync(t.Context())
				require.ErrorContains(t, err, "unsupported-base")
			}
		}

		for _, api := range allAPIs {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", mode, api, stream), func(t *testing.T) {
					for _, replica := range []*replica{env.A, env.B} {
						// Explicit removal reproduces another replica retiring the same name.
						schemas.UnregisterKnownProvider("local-fallback")
						schemas.UnregisterKnownProvider("local-pending")
						extra := map[string]any{}
						if mode == "caller fallback" {
							extra["fallbacks"] = []string{"local-fallback/gpt-e2e-a"}
						}
						requestNonce := nonce("custom-fallback")

						result := replica.call(t, api, "local-pending/test", "hi "+requestNonce, stream, extra).requireOK(t)
						require.Equal(t, "local-fallback", result.Header.Get(headerServedProvider))
						requests := env.mock.InferenceRequests(requestNonce)
						require.Len(t, requests, 1)
						require.Equal(t, openAIKey, requests[0].Key)
					}
				})
			}
		}
	}
}
