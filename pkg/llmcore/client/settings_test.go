package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestLoadOnlyDefaultSettings(t *testing.T) {
	for _, hasDefault := range []bool{false, true} {
		t.Run(fmt.Sprintf("default=%t", hasDefault), func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			store := newStore(t, storage)
			if hasDefault {
				require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
					settings["client"] = json.RawMessage(`{"max_request_body_size_mb":1}`)
					return nil
				}))
			}
			before, err := store.Load(t.Context())
			require.NoError(t, err)
			extra := v1.LLMGatewayConfig{Name: "extra", Namespace: system.DefaultNamespace, Spec: v1.LLMGatewayResourceSpec{ID: "extra", Payload: `{"proxy":{"enabled":true,"url":"https://unexpected.example"}}`}}
			require.NoError(t, storage.Create(t.Context(), &extra))
			for _, payload := range []string{extra.Spec.Payload, "invalid JSON"} {
				extra.Spec.Payload = payload
				require.NoError(t, storage.Update(t.Context(), &extra))
				cfg, err := store.Load(t.Context())
				require.NoError(t, err)
				settings, err := store.GetSettings(t.Context())
				require.NoError(t, err)
				require.Equal(t, before.Config.Settings, cfg.Config.Settings)
				require.Equal(t, settings, cfg.Config.Settings)
			}
			require.NoError(t, storage.Delete(t.Context(), &extra))
			cfg, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, before.Config.Settings, cfg.Config.Settings)
			require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
				settings["proxy"] = json.RawMessage(`{"enabled":false}`)
				return nil
			}))
			cfg, err = store.Load(t.Context())
			require.NoError(t, err)
			require.NotEqual(t, before.Config.Settings, cfg.Config.Settings)
		})
	}
}

func TestSettingsReadFailures(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	failure := fmt.Errorf("singleton read failed")
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if _, ok := obj.(*v1.LLMGatewayConfig); ok {
				return failure
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	store := newStore(t, wrapped)
	_, err := store.Load(t.Context())
	require.ErrorIs(t, err, failure)
	_, err = store.GetSettings(t.Context())
	require.ErrorIs(t, err, failure)
}

func TestDefaultSettingsDecodeFailure(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.LLMGatewayConfig{Name: "default", Namespace: system.DefaultNamespace, Spec: v1.LLMGatewayResourceSpec{ID: "default", Payload: "invalid JSON"}}).Build()
	store := newStore(t, storage)
	_, err := store.Load(t.Context())
	require.Error(t, err)
	_, err = store.GetSettings(t.Context())
	require.Error(t, err)
}
