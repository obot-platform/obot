package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/nah/pkg/router"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/services"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type retryResponse struct{ delay time.Duration }

type plaintextGatewayCodec struct{}

func (plaintextGatewayCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (plaintextGatewayCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func TestGatewayReconciliationRetriesStartupAndDiscovery(t *testing.T) {
	ctx := t.Context()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource()).Build()
	store, err := llmcoreclient.New(storage, plaintextGatewayCodec{})
	require.NoError(t, err)
	engine := llmengine.New()
	failDiscovery := true
	attempts := 0
	discovery := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if _, ok := obj.(*v1.ModelProvider); ok {
				attempts++
				if failDiscovery {
					failDiscovery = false
					return fmt.Errorf("discovery unavailable")
				}
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	c := &Controller{services: &services.Services{BifrostEngine: engine, StorageClient: discovery}}
	response := &retryResponse{}
	require.NoError(t, c.reconcileLLMGatewayConfig(router.Request{Ctx: ctx}, response))
	require.Equal(t, time.Second, response.delay, "startup-window events must be retried")
	require.NoError(t, engine.Start(ctx, llmengine.Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(engine.Shutdown)
	require.NoError(t, store.CreateProvider(ctx, "openai", llmcoreclient.Provider{}))
	require.ErrorContains(t, c.reconcileLLMGatewayConfig(router.Request{Ctx: ctx}, &retryResponse{}), "discovery unavailable")
	providers, err := engine.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Contains(t, providers, schemas.OpenAI, "application succeeded before discovery failed")
	changed, err := engine.Sync(ctx)
	require.NoError(t, err)
	require.False(t, changed)
	require.NoError(t, c.reconcileLLMGatewayConfig(router.Request{Ctx: ctx}, &retryResponse{}))
	require.Equal(t, 2, attempts, "discovery must retry even when application is unchanged")
}

func (r *retryResponse) Attributes() map[string]any     { return nil }
func (r *retryResponse) RetryAfter(delay time.Duration) { r.delay = delay }
