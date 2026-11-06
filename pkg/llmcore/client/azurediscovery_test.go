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
	generation, err := store.BeginAzureDiscovery(t.Context(), "azure", key.ID, snapshot.Fingerprint)
	require.NoError(t, err)
	require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, generation, snapshot))

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
	require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, generation, snapshot))
	require.Equal(t, before, versions(), "identical discovery must not write or trigger another watch")

	for _, change := range []string{"credentials", "account", "clear", "delete"} {
		t.Run(change, func(t *testing.T) {
			require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *client.Provider) error {
				p.Keys = []schemas.Key{key}
				p.AzureAccounts = map[string]client.AzureAccount{key.ID: account}

				return nil
			}))
			generation, err = store.BeginAzureDiscovery(t.Context(), "azure", key.ID, snapshot.Fingerprint)
			require.NoError(t, err)
			require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, generation, snapshot))
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
			require.NoError(t, store.SaveAzureDiscovery(t.Context(), "azure", key.ID, generation, snapshot))
			require.Equal(t, before, versions())

			loaded, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Empty(t, loaded.Config.AzureDiscoveries[schemas.Azure])
		})
	}
}

func TestAzureDiscoveryRejectsOlderRefresh(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	olderClient := newStore(t, storage)
	newerClient := newStore(t, storage)
	key := schemas.Key{ID: "key", AzureKeyConfig: &schemas.AzureKeyConfig{ClientSecret: schemas.NewSecretVar("secret")}}
	account := client.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
	require.NoError(t, olderClient.CreateProvider(t.Context(), "azure", client.Provider{
		Keys:          []schemas.Key{key},
		AzureAccounts: map[string]client.AzureAccount{key.ID: account},
	}))

	older := client.AzureDiscoverySnapshot{
		Fingerprint: client.AzureDiscoveryFingerprint(key, account),
		Names:       []string{"deleted-deployment"},
		BaseModels:  map[string]string{"deleted-deployment": "gpt-4o"},
	}
	newer := client.AzureDiscoverySnapshot{Fingerprint: older.Fingerprint, Names: []string{}}

	olderGeneration, err := olderClient.BeginAzureDiscovery(t.Context(), "azure", key.ID, older.Fingerprint)
	require.NoError(t, err)
	newerGeneration, err := newerClient.BeginAzureDiscovery(t.Context(), "azure", key.ID, newer.Fingerprint)
	require.NoError(t, err)
	require.Greater(t, newerGeneration, olderGeneration)

	reservation, err := newerClient.GetProvider(t.Context(), "azure")
	require.NoError(t, err)
	require.Equal(t, newerGeneration, reservation.AzureRefreshGenerations[key.ID])

	// A second replica confirms deletion before the older replica returns its result.
	require.NoError(t, newerClient.SaveAzureDiscovery(t.Context(), "azure", key.ID, newerGeneration, newer))
	require.NoError(t, olderClient.SaveAzureDiscovery(t.Context(), "azure", key.ID, olderGeneration, older))

	stored, err := olderClient.GetProvider(t.Context(), "azure")
	require.NoError(t, err)
	require.Empty(t, stored.AzureDiscoveries[key.ID].Names)
	require.Empty(t, stored.AzureDiscoveries[key.ID].BaseModels)

	before, err := olderClient.Load(t.Context())
	require.NoError(t, err)

	// A failed newer attempt also keeps the last published result, not an older worker's late result.
	lateGeneration, err := olderClient.BeginAzureDiscovery(t.Context(), "azure", key.ID, older.Fingerprint)
	require.NoError(t, err)
	_, err = newerClient.BeginAzureDiscovery(t.Context(), "azure", key.ID, newer.Fingerprint)
	require.NoError(t, err)
	require.NoError(t, olderClient.SaveAzureDiscovery(t.Context(), "azure", key.ID, lateGeneration, older))

	stored, err = olderClient.GetProvider(t.Context(), "azure")
	require.NoError(t, err)
	require.Empty(t, stored.AzureDiscoveries[key.ID].Names)

	after, err := olderClient.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, newer, after.Config.AzureDiscoveries[schemas.Azure][key.ID])
	require.Equal(t, before.Config, after.Config, "reservations must not change the runtime configuration view")
}
