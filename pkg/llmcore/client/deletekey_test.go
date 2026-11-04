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

func TestDeleteKeyRequiresProvider(t *testing.T) {
	store := newStore(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	require.True(t, apierrors.IsNotFound(store.DeleteKey(t.Context(), "missing", "key")))
}

func TestDeleteKeyProviderDisappearsDuringUpdate(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	base := newStore(t, storage)
	require.NoError(t, base.CreateProvider(t.Context(), "openai", client.Provider{Keys: []schemas.Key{{ID: "key"}}}))
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			require.NoError(t, c.Delete(ctx, obj))
			return c.Update(ctx, obj, opts...)
		},
	})
	require.True(t, apierrors.IsNotFound(newStore(t, wrapped).DeleteKey(t.Context(), "openai", "key")))
	_, err := base.GetProvider(t.Context(), "openai")
	require.True(t, apierrors.IsNotFound(err))
}

func TestDeleteKeyIsIdempotentForExistingProvider(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	base := newStore(t, storage)
	require.NoError(t, base.CreateProvider(t.Context(), "openai", client.Provider{Keys: []schemas.Key{{ID: "remove"}, {ID: "keep"}}}))
	writes := 0
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			writes++
			return c.Update(ctx, obj, opts...)
		},
	})
	store := newStore(t, wrapped)
	require.NoError(t, store.DeleteKey(t.Context(), "openai", "remove"))
	require.NoError(t, store.DeleteKey(t.Context(), "openai", "remove"))
	require.NoError(t, store.DeleteKey(t.Context(), "openai", "never-existed"))
	require.Equal(t, 1, writes)
	provider, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Len(t, provider.Keys, 1)
	require.Equal(t, "keep", provider.Keys[0].ID)
}
