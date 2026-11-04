package llmgatewayconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type retryResponse struct{ delay time.Duration }
type plaintextCodec struct{}
type testEngine struct {
	ready     bool
	core      *llmcore.Engine
	syncErr   error
	syncCalls int
}

func (r *retryResponse) Attributes() map[string]any     { return nil }
func (r *retryResponse) RetryAfter(delay time.Duration) { r.delay = delay }
func (plaintextCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (plaintextCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (e *testEngine) Ready() bool { return e.ready }
func (e *testEngine) Sync(ctx context.Context) (bool, error) {
	e.syncCalls++
	if e.syncErr != nil {
		return false, e.syncErr
	}
	return e.core.Sync(ctx)
}

func TestReconcileRetriesStartupAndDiscovery(t *testing.T) {
	ctx := t.Context()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextCodec{})
	require.NoError(t, err)
	engine := &testEngine{}
	attempts := 0
	discoveryErr := errors.New("discovery unavailable")
	handler := New(engine, func(context.Context) error {
		attempts++
		if attempts == 1 {
			return discoveryErr
		}
		return nil
	})
	response := &retryResponse{}
	require.NoError(t, handler.Reconcile(router.Request{Ctx: ctx}, response))
	require.Equal(t, time.Second, response.delay, "startup-window events must be retried")
	require.Zero(t, engine.syncCalls)
	require.Zero(t, attempts)
	engine.core, err = llmcore.New(ctx, store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(engine.core.Shutdown)
	engine.ready = true
	require.NoError(t, store.CreateProvider(ctx, "openai", llmcoreclient.Provider{}))
	// The event's payload is deliberately obsolete: reconciliation must load the store.
	request := router.Request{Ctx: ctx, Object: &v1.LLMGatewayProvider{Spec: v1.LLMGatewayResourceSpec{ID: "obsolete"}}}
	require.ErrorIs(t, handler.Reconcile(request, &retryResponse{}), discoveryErr)
	providers, err := engine.core.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Contains(t, providers, schemas.OpenAI, "application succeeded before discovery failed")
	require.NotContains(t, providers, schemas.ModelProvider("obsolete"))
	changed, err := engine.core.Sync(ctx)
	require.NoError(t, err)
	require.False(t, changed)
	require.NoError(t, handler.Reconcile(request, &retryResponse{}))
	require.Equal(t, 2, attempts, "discovery must retry even when application is unchanged")
}

func TestReconcileRetriesApplicationWithoutStarvingDiscovery(t *testing.T) {
	ctx := t.Context()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextCodec{})
	require.NoError(t, err)
	core, err := llmcore.New(ctx, store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(core.Shutdown)
	applicationErr := errors.New("application failed")
	engine := &testEngine{ready: true, core: core, syncErr: applicationErr}
	discoveries := 0
	handler := New(engine, func(context.Context) error { discoveries++; return nil })
	request := router.Request{Ctx: ctx}
	for range 2 {
		require.ErrorIs(t, handler.Reconcile(request, &retryResponse{}), applicationErr)
	}
	require.Equal(t, 2, engine.syncCalls, "failed application remains retryable without another write")
	require.Equal(t, 2, discoveries, "application errors must not starve discovery")
	engine.syncErr = nil
	require.NoError(t, handler.Reconcile(request, &retryResponse{}))
	require.Equal(t, 3, discoveries)
}

func TestDiscoveryAfterRoutingTargetDeleted(t *testing.T) {
	ctx := t.Context()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextCodec{})
	require.NoError(t, err)
	for _, name := range []string{"openai", "anthropic"} {
		require.NoError(t, store.PutProvider(ctx, name, llmcoreclient.Provider{}))
	}
	provider := "anthropic"
	require.NoError(t, store.PutRule(ctx, tables.TableRoutingRule{
		ID: "keep-policy", Name: "keep-policy", CelExpression: "true", Scope: "global",
		Targets: []tables.TableRoutingTarget{{Provider: &provider, Weight: 1}},
	}))
	core, err := llmcore.New(ctx, store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(core.Shutdown)
	require.NoError(t, store.DeleteProvider(ctx, "anthropic"))
	// An unrelated provider must still apply and become discoverable.
	require.NoError(t, store.PutProvider(ctx, "gemini", llmcoreclient.Provider{}))
	discoveries := 0
	handler := New(&testEngine{ready: true, core: core}, func(context.Context) error {
		discoveries++
		providers, err := core.Client().GetConfiguredProviders()
		require.NoError(t, err)
		require.Contains(t, providers, schemas.Gemini)
		require.NotContains(t, providers, schemas.Anthropic)
		return nil
	})
	for range 2 {
		require.ErrorContains(t, handler.Reconcile(router.Request{Ctx: ctx}, &retryResponse{}), "anthropic is not configured")
	}
	require.Equal(t, 2, discoveries)
	cfg, err := store.Load(ctx)
	require.NoError(t, err)
	require.Contains(t, cfg.Rules, "keep-policy")
}
