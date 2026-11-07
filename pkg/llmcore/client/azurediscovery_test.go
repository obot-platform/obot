package client_test

import (
	"context"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type cancelingDiscoveryCodec struct {
	cancel  context.CancelFunc
	enabled bool
}

func (c *cancelingDiscoveryCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	if c.enabled {
		c.cancel()
	}
	return data, nil
}

func (*cancelingDiscoveryCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func TestAzureSnapshotPublicationAndInvalidation(t *testing.T) {
	for _, change := range []string{"credentials", "credentials restored", "account", "clear", "delete"} {
		t.Run(change, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			store := newStore(t, storage)
			key := schemas.Key{ID: "key", AzureKeyConfig: &schemas.AzureKeyConfig{ClientSecret: schemas.NewSecretVar("secret")}}
			account := client.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
			require.NoError(t, store.CreateProvider(t.Context(), "azure", client.Provider{
				Keys:          []schemas.Key{key},
				AzureAccounts: map[string]client.AzureAccount{key.ID: account},
			}))
			snapshots := map[string]client.AzureDiscoverySnapshot{key.ID: {
				Fingerprint: client.AzureDiscoveryFingerprint(key, account),
				Names:       []string{"real"},
			}}
			_, version, err := store.GetProviderWithVersion(t.Context(), "azure")
			require.NoError(t, err)
			require.NoError(t, store.SaveAzureDiscoveries(t.Context(), "azure", version, snapshots))
			loaded, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, snapshots[key.ID].Names, loaded.Config.AzureDiscoveries[schemas.Azure][key.ID].Names)
			require.NotEmpty(t, loaded.Config.AzureDiscoveries[schemas.Azure][key.ID].RefreshID)

			_, oldVersion, err := store.GetProviderWithVersion(t.Context(), "azure")
			require.NoError(t, err)
			require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *client.Provider) error {
				switch change {
				case "credentials", "credentials restored":
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
			if change == "credentials restored" {
				require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *client.Provider) error {
					p.Keys[0].AzureKeyConfig.ClientSecret = schemas.NewSecretVar("secret")
					return nil
				}))
			}
			require.True(t, apierrors.IsConflict(store.SaveAzureDiscoveries(t.Context(), "azure", oldVersion, snapshots)))
			loaded, err = store.Load(t.Context())
			require.NoError(t, err)
			require.Empty(t, loaded.Config.AzureDiscoveries[schemas.Azure])
		})
	}
}

func TestAzureDiscoveryRejectsOlderOwnerIncludingUnchangedSuccess(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	older := newStore(t, storage)
	newer := newStore(t, storage)
	key := schemas.Key{ID: "key", AzureKeyConfig: &schemas.AzureKeyConfig{ClientSecret: schemas.NewSecretVar("secret")}}
	account := client.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
	empty := map[string]client.AzureDiscoverySnapshot{key.ID: {
		Fingerprint: client.AzureDiscoveryFingerprint(key, account),
		Names:       []string{},
	}}
	require.NoError(t, older.CreateProvider(t.Context(), "azure", client.Provider{
		Keys:             []schemas.Key{key},
		AzureAccounts:    map[string]client.AzureAccount{key.ID: account},
		AzureDiscoveries: empty,
	}))
	_, oldVersion, err := older.GetProviderWithVersion(t.Context(), "azure")
	require.NoError(t, err)
	_, newVersion, err := newer.GetProviderWithVersion(t.Context(), "azure")
	require.NoError(t, err)

	// An unchanged empty success still fences the previous owner's delayed response.
	require.NoError(t, newer.SaveAzureDiscoveries(t.Context(), "azure", newVersion, empty))
	stale := map[string]client.AzureDiscoverySnapshot{key.ID: {
		Fingerprint: empty[key.ID].Fingerprint,
		Names:       []string{"deleted-deployment"},
	}}
	require.True(t, apierrors.IsConflict(older.SaveAzureDiscoveries(t.Context(), "azure", oldVersion, stale)))
	stored, _, err := older.GetProviderWithVersion(t.Context(), "azure")
	require.NoError(t, err)
	require.Empty(t, stored.AzureDiscoveries[key.ID].Names)
	require.NotEmpty(t, stored.AzureDiscoveries[key.ID].RefreshID)
}

func TestAzurePublicationChecksCancellationAfterEncoding(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	codec := &cancelingDiscoveryCodec{cancel: cancel}
	var updates int
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			updates++
			return c.Update(context.WithoutCancel(ctx), obj, opts...)
		},
	}).Build()
	store, err := client.New(storage, codec)
	require.NoError(t, err)
	key := schemas.Key{ID: "key"}
	account := client.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
	require.NoError(t, store.CreateProvider(ctx, "azure", client.Provider{
		Keys:          []schemas.Key{key},
		AzureAccounts: map[string]client.AzureAccount{key.ID: account},
	}))
	_, version, err := store.GetProviderWithVersion(ctx, "azure")
	require.NoError(t, err)
	codec.enabled = true

	err = store.SaveAzureDiscoveries(ctx, "azure", version, map[string]client.AzureDiscoverySnapshot{key.ID: {
		Fingerprint: client.AzureDiscoveryFingerprint(key, account),
		Names:       []string{"late"},
	}})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, updates, "a canceled owner cannot reach storage publication even if storage ignores cancellation")
}
