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

func TestReconcileRetriesStartupAndApplication(t *testing.T) {
	engine := &testEngine{}
	handler := New(engine)
	response := &retryResponse{}
	require.NoError(t, handler.Reconcile(router.Request{Ctx: t.Context()}, response))
	require.Equal(t, time.Second, response.delay)
	require.Zero(t, engine.syncCalls)

	engine.ready = true
	failure := errors.New("application failed")
	engine.syncErr = failure
	require.ErrorIs(t, handler.Reconcile(router.Request{Ctx: t.Context()}, &retryResponse{}), failure)
	require.Equal(t, 1, engine.syncCalls)
	engine.syncErr = nil
	engine.syncFn = func(context.Context) (bool, error) { return false, nil }
	require.NoError(t, handler.Reconcile(router.Request{Ctx: t.Context()}, &retryResponse{}))
	require.Equal(t, 2, engine.syncCalls)
}

func TestReconcileCoalescesOverlappingEvents(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "retry"}[fail], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			var desired atomic.Int64
			var observed []int64
			failure := errors.New("first pass failed")
			engine := &testEngine{ready: true}
			engine.syncFn = func(context.Context) (bool, error) {
				observed = append(observed, desired.Load())
				if engine.syncCalls == 1 {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
						return false, ctx.Err()
					}
					if fail {
						return false, failure
					}
				}
				return false, ctx.Err()
			}
			handler := New(engine)
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
			require.Equal(t, 1, engine.syncCalls)
			close(release)
			select {
			case err := <-done:
				if fail {
					require.ErrorIs(t, err, failure)
					require.NoError(t, handler.Reconcile(router.Request{Ctx: ctx}, &retryResponse{}))
				} else {
					require.NoError(t, err)
				}
			case <-ctx.Done():
				t.Fatal("reconciliation did not finish")
			}
			require.Equal(t, []int64{0, 1}, observed)
		})
	}
}

func TestSnapshotEventsApplyOnEveryReplicaWithoutModelSync(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextCodec{})
	require.NoError(t, err)
	key := schemas.Key{ID: "key", Models: schemas.WhiteList{"*"}}
	account := llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
	require.NoError(t, store.CreateProvider(t.Context(), "azure", llmcoreclient.Provider{
		Keys:          []schemas.Key{key},
		AzureAccounts: map[string]llmcoreclient.AzureAccount{key.ID: account},
	}))

	var engines []*llmcore.Engine
	for range 2 {
		core, err := llmcore.New(t.Context(), store, llmcore.Options{})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, core.Shutdown(context.Background())) })
		engines = append(engines, core)
	}
	_, version, err := store.GetProviderWithVersion(t.Context(), "azure")
	require.NoError(t, err)
	require.NoError(t, store.SaveAzureDiscoveries(t.Context(), "azure", version, map[string]llmcoreclient.AzureDiscoverySnapshot{key.ID: {
		Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, account),
		Names:       []string{"deployment"},
	}}))

	for _, core := range engines {
		engine := &testEngine{ready: true, core: core}
		handler := New(engine)
		require.NoError(t, handler.Reconcile(router.Request{Ctx: t.Context(), Object: &v1.LLMGatewayProvider{}}, &retryResponse{}))
		changed, err := core.Sync(t.Context())
		require.NoError(t, err)
		require.False(t, changed, "the snapshot was already applied by this replica's watch")
	}
	// The watcher has no discovery/trigger callback: snapshot publication can only apply runtime state.
}

func TestReconcileAfterRoutingTargetDeleted(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextCodec{})
	require.NoError(t, err)
	for _, name := range []string{"openai", "anthropic"} {
		require.NoError(t, store.CreateProvider(t.Context(), name, llmcoreclient.Provider{}))
	}
	provider := "anthropic"
	require.NoError(t, store.PutRule(t.Context(), tables.TableRoutingRule{
		ID:            "keep-policy",
		Name:          "keep-policy",
		CelExpression: "true",
		Scope:         "global",
		Targets:       []tables.TableRoutingTarget{{Provider: &provider, Weight: 1}},
	}))
	core, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, core.Shutdown(context.Background())) })
	require.NoError(t, store.DeleteProvider(t.Context(), "anthropic"))
	require.NoError(t, store.CreateProvider(t.Context(), "gemini", llmcoreclient.Provider{}))

	handler := New(&testEngine{ready: true, core: core})
	response := &retryResponse{}
	require.NoError(t, handler.Reconcile(router.Request{Ctx: t.Context()}, response))
	require.Zero(t, response.delay, "an unchanged dangling rule must not force a retry")
	configured, err := core.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Contains(t, configured, schemas.Gemini)
	require.NotContains(t, configured, schemas.Anthropic)
	require.ErrorContains(t, core.Err(), "anthropic is not configured")
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Contains(t, loaded.Config.Rules, "keep-policy")
}
