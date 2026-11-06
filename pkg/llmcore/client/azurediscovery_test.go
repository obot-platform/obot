package client_test

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAzureSnapshotPublicationAndInvalidation(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store := newStore(t, storage)
	key := schemas.Key{ID: "key", AzureKeyConfig: &schemas.AzureKeyConfig{ClientSecret: schemas.NewSecretVar("secret")}}
	account := client.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
	require.NoError(t, store.CreateProvider(t.Context(), "azure", client.Provider{Keys: []schemas.Key{key}, AzureAccounts: map[string]client.AzureAccount{key.ID: account}}))
	snapshot := client.AzureDiscoverySnapshot{Fingerprint: client.AzureDiscoveryFingerprint(key, account), Names: []string{"real"}}
	require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, snapshot))
	versions := func() string {
		t.Helper()
		var providers v1.LLMGatewayProviderList
		require.NoError(t, storage.List(t.Context(), &providers))
		require.Len(t, providers.Items, 1)
		return providers.Items[0].ResourceVersion
	}
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, snapshot, loaded.Config.AzureDiscoveries[schemas.Azure][key.ID])
	before := versions()
	require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, snapshot))
	require.Equal(t, before, versions(), "identical discovery must not write or trigger another watch")
	for _, change := range []string{"credentials", "account", "clear", "delete"} {
		t.Run(change, func(t *testing.T) {
			require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *client.Provider) error {
				p.Keys = []schemas.Key{key}
				p.AzureAccounts = map[string]client.AzureAccount{key.ID: account}
				return nil
			}))
			require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, snapshot))
			require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *client.Provider) error {
				switch change {
				case "credentials":
					p.Keys[0].AzureKeyConfig.ClientSecret = schemas.NewSecretVar("replacement")
				case "account":
					a := p.AzureAccounts[key.ID]
					a.AccountName = "other"
					p.AzureAccounts[key.ID] = a
				case "clear":
					delete(p.AzureAccounts, key.ID)
				case "delete":
					p.Keys = nil
					delete(p.AzureAccounts, key.ID)
				}
				return nil
			}))
			before := versions()
			// This is the old worker completing after the administrator's edit.
			require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, snapshot))
			require.Equal(t, before, versions())
			loaded, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Empty(t, loaded.Config.AzureDiscoveries[schemas.Azure])
		})
	}
}
