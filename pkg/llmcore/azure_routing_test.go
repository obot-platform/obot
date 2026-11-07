package llmcore_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestAzureInferenceUsesTheOwningAccount(t *testing.T) {
	served := make(chan string, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Model string }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		served <- r.Header.Get("api-key") + "/" + body.Model

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, configTestChatResponse)
	}))
	t.Cleanup(upstream.Close)

	store := newTestStore(t)
	provider := llmcoreclient.Provider{
		Config:        schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{AllowPrivateNetwork: true}},
		AzureAccounts: map[string]llmcoreclient.AzureAccount{},
	}
	for _, id := range []string{"one", "two"} {
		key := schemas.Key{
			ID:             id,
			Name:           id,
			Value:          *schemas.NewSecretVar(id),
			Models:         schemas.WhiteList{"*"},
			Weight:         1,
			AzureKeyConfig: &schemas.AzureKeyConfig{Endpoint: *schemas.NewSecretVar(upstream.URL)},
		}
		if id == "one" {
			key.Aliases = schemas.KeyAliases{"friendly": {ModelID: "only-one"}}
		}

		provider.Keys = append(provider.Keys, key)
		provider.AzureAccounts[id] = llmcoreclient.AzureAccount{
			SubscriptionID: "sub",
			ResourceGroup:  "group",
			AccountName:    id,
		}
	}

	require.NoError(t, store.CreateProvider(t.Context(), "azure", provider))

	for _, key := range provider.Keys {
		snapshot := llmcoreclient.AzureDiscoverySnapshot{
			Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, provider.AzureAccounts[key.ID]),
			Names:       []string{"only-" + key.ID},
		}
		generation, err := store.BeginAzureDiscovery(t.Context(), "azure", key.ID, snapshot.Fingerprint)
		require.NoError(t, err)
		require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, generation, snapshot))
	}

	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	call := func(keyID, model string) *schemas.BifrostError {
		ctx := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
		defer ctx.Cancel()
		ctx.SetValue(schemas.BifrostContextKeyAPIKeyID, keyID)

		_, err := engine.Client().ChatCompletionRequest(ctx, &schemas.BifrostChatRequest{
			Provider: schemas.Azure,
			Model:    model,
			Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("hello")}}},
		})

		return err
	}

	require.NotNil(t, call("two", "only-one"), "the wrong account must be rejected before dispatch")
	require.Empty(t, served)
	require.Nil(t, call("one", "only-one"))
	require.Equal(t, "one/only-one", <-served)
	require.Nil(t, call("one", "friendly"))
	require.Equal(t, "one/only-one", <-served, "the alias target must remain the deployment ID")
	require.Nil(t, call("two", "only-two"))
	require.Equal(t, "two/only-two", <-served)

	stored, err := store.GetProvider(t.Context(), "azure")
	require.NoError(t, err)
	require.Equal(t, schemas.WhiteList{"*"}, stored.Keys[0].Models)
	require.Equal(t, schemas.WhiteList{"*"}, stored.Keys[1].Models)
}
