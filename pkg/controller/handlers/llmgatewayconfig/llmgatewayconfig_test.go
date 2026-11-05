package llmgatewayconfig

import (
	"context"
	"errors"
	"sync/atomic"
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
	syncFn    func(context.Context) (bool, error)
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
	if e.syncFn != nil {
		return e.syncFn(ctx)
	}
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
	t.Cleanup(func() { require.NoError(t, engine.core.Shutdown(context.Background())) })
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
	t.Cleanup(func() { require.NoError(t, core.Shutdown(context.Background())) })
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
		require.NoError(t, store.CreateProvider(ctx, name, llmcoreclient.Provider{}))
	}
	provider := "anthropic"
	require.NoError(t, store.PutRule(ctx, tables.TableRoutingRule{
		ID: "keep-policy", Name: "keep-policy", CelExpression: "true", Scope: "global",
		Targets: []tables.TableRoutingTarget{{Provider: &provider, Weight: 1}},
	}))
	core, err := llmcore.New(ctx, store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, core.Shutdown(context.Background())) })
	require.NoError(t, store.DeleteProvider(ctx, "anthropic"))
	// An unrelated provider must still apply and become discoverable.
	require.NoError(t, store.CreateProvider(ctx, "gemini", llmcoreclient.Provider{}))
	discoveries := 0
	discoveryErr := errors.New("discovery unavailable")
	handler := New(&testEngine{ready: true, core: core}, func(context.Context) error {
		discoveries++
		providers, err := core.Client().GetConfiguredProviders()
		require.NoError(t, err)
		require.Contains(t, providers, schemas.Gemini)
		require.NotContains(t, providers, schemas.Anthropic)
		if discoveries == 1 {
			return discoveryErr
		}
		return nil
	})
	require.ErrorIs(t, handler.Reconcile(router.Request{Ctx: ctx}, &retryResponse{}), discoveryErr)
	require.ErrorContains(t, core.Err(), "anthropic is not configured")
	response := &retryResponse{}
	require.NoError(t, handler.Reconcile(router.Request{Ctx: ctx}, response), "the dangling reference must not force another retry")
	require.Zero(t, response.delay)
	require.Equal(t, 2, discoveries, "failed discovery still retries with an unchanged dangling rule")
	changed, err := core.Sync(ctx)
	require.NoError(t, err)
	require.False(t, changed)
	require.ErrorContains(t, core.Err(), "anthropic is not configured")
	loaded, err := store.Load(ctx)
	require.NoError(t, err)
	require.Contains(t, loaded.Config.Rules, "keep-policy")
}

func TestReconcileCoalescesOverlappingEvents(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage string
		fail  bool
	}{
		{
			name:  "during sync",
			stage: "sync",
		},
		{
			name:  "during discovery",
			stage: "discovery",
		},
		{
			name:  "failed sync retries pending changes",
			stage: "sync",
			fail:  true,
		},
		{
			name:  "failed discovery retries pending changes",
			stage: "discovery",
			fail:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			var desired atomic.Int64
			var observed []int64
			failure := errors.New("first pass failed")
			pause := func() error {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				if tc.fail {
					return failure
				}
				return nil
			}
			engine := &testEngine{ready: true}
			engine.syncFn = func(context.Context) (bool, error) {
				observed = append(observed, desired.Load())
				if engine.syncCalls == 1 && tc.stage == "sync" {
					return false, pause()
				}
				return false, ctx.Err()
			}
			discoveries := 0
			handler := New(engine, func(context.Context) error {
				discoveries++
				if discoveries == 1 && tc.stage == "discovery" {
					return pause()
				}
				return ctx.Err()
			})
			done := make(chan error, 1)
			go func() { done <- handler.Reconcile(router.Request{Ctx: ctx}, &retryResponse{}) }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("first reconciliation did not start")
			}

			desired.Store(1)
			for range 10 {
				require.NoError(t, handler.Reconcile(router.Request{Ctx: ctx}, &retryResponse{}))
			}
			require.Equal(t, 1, engine.syncCalls, "overlapping events must not reload independently")
			close(release)
			select {
			case err := <-done:
				if tc.fail {
					require.ErrorIs(t, err, failure, "the active event must return the error for router retry")
					require.Equal(t, 1, discoveries, "application failure must still trigger discovery")
					// Retry the active event without another configuration change.
					require.NoError(t, handler.Reconcile(router.Request{Ctx: ctx}, &retryResponse{}))
				} else {
					require.NoError(t, err)
				}
			case <-ctx.Done():
				t.Fatal("reconciliation did not finish")
			}
			require.Equal(t, []int64{0, 1}, observed, "the next pass must load changes made during sync or discovery")
			require.Equal(t, 2, engine.syncCalls)
			require.Equal(t, 2, discoveries)
			// A later, non-overlapping event must still reconcile normally.
			require.NoError(t, handler.Reconcile(router.Request{Ctx: ctx}, &retryResponse{}))
			require.Equal(t, 3, engine.syncCalls)
			require.Equal(t, 3, discoveries)
		})
	}
}
