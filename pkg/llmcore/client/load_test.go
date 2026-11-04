package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestLoadQuarantinesWholeProvidersAndRecovers(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store := newStore(t, storage)
	for _, name := range []string{"healthy", "corrupt", "partial-key"} {
		require.NoError(t, store.CreateProvider(t.Context(), name, client.Provider{Keys: []schemas.Key{{ID: "key"}}}))
	}
	var rows v1.LLMGatewayProviderList
	require.NoError(t, storage.List(t.Context(), &rows))
	for i := range rows.Items {
		row := &rows.Items[i]
		switch row.Spec.ID {
		case "corrupt":
			row.Spec.Payload = "invalid JSON"
		case "partial-key":
			row.Spec.Payload = `{"Config":{},"Keys":[{"id":"valid"},{"id":123}]}`
		default:
			continue
		}
		require.NoError(t, storage.Update(t.Context(), row))
	}
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Len(t, loaded.Config.Providers, 1)
	require.Contains(t, loaded.Config.Providers, schemas.ModelProvider("healthy"))
	require.Len(t, loaded.Config.Keys, 1)
	require.Len(t, loaded.ProviderErrors, 2)
	for _, problem := range loaded.ProviderErrors {
		require.Equal(t, system.DefaultNamespace, problem.Namespace)
		require.NotEmpty(t, problem.ResourceName)
		require.NotEqual(t, schemas.ModelProvider("healthy"), problem.Provider)
		require.ErrorIs(t, problem, problem.Err)
		var syntax *json.SyntaxError
		var typeError *json.UnmarshalTypeError
		require.True(t, errors.As(problem, &syntax) || errors.As(problem, &typeError))
	}
	for i := range rows.Items {
		row := &rows.Items[i]
		if row.Spec.ID == "healthy" {
			continue
		}
		payload, err := json.Marshal(client.Provider{Keys: []schemas.Key{{ID: "repaired"}}})
		require.NoError(t, err)
		row.Spec.Payload = string(payload)
		require.NoError(t, storage.Update(t.Context(), row))
	}
	loaded, err = store.Load(t.Context())
	require.NoError(t, err)
	require.Empty(t, loaded.ProviderErrors)
	require.Len(t, loaded.Config.Providers, 3)
	require.NoError(t, store.DeleteProvider(t.Context(), "corrupt"))
	loaded, err = store.Load(t.Context())
	require.NoError(t, err)
	require.Empty(t, loaded.ProviderErrors)
	require.NotContains(t, loaded.Config.Providers, schemas.ModelProvider("corrupt"))
}

func TestLoadBlockingErrorsDiscardResult(t *testing.T) {
	for _, kind := range []string{"providers", "rules", "settings", "bad-rule", "bad-settings"} {
		t.Run(kind, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			require.NoError(t, newStore(t, storage).CreateProvider(t.Context(), "healthy", client.Provider{}))
			failure := fmt.Errorf("resource API unavailable")
			wrapped := interceptor.NewClient(storage, interceptor.Funcs{
				List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
					_, providers := list.(*v1.LLMGatewayProviderList)
					_, rules := list.(*v1.LLMGatewayRoutingRuleList)
					if kind == "providers" && providers || kind == "rules" && rules {
						return failure
					}
					return c.List(ctx, list, opts...)
				},
				Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
					if kind == "settings" {
						return failure
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})
			if kind == "bad-rule" {
				require.NoError(t, storage.Create(t.Context(), &v1.LLMGatewayRoutingRule{Name: "bad", Namespace: system.DefaultNamespace, Spec: v1.LLMGatewayResourceSpec{ID: "bad", Payload: "invalid JSON"}}))
			}
			if kind == "bad-settings" {
				require.NoError(t, storage.Create(t.Context(), &v1.LLMGatewayConfig{Name: system.LLMGatewayConfigName, Namespace: system.DefaultNamespace, Spec: v1.LLMGatewayResourceSpec{Payload: "invalid JSON"}}))
			}
			loaded, err := newStore(t, wrapped).Load(t.Context())
			require.Error(t, err)
			require.Equal(t, client.LoadResult{}, loaded)
			if kind != "bad-rule" && kind != "bad-settings" {
				require.ErrorIs(t, err, failure)
			}
		})
	}
}

func TestMutateProviderDoesNotCreateMissingResource(t *testing.T) {
	store := newStore(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	err := store.MutateProvider(t.Context(), "missing", func(*client.Provider) error { t.Fatal("callback called without existing provider"); return nil })
	require.Error(t, err)
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Empty(t, loaded.Config.Providers)
}
