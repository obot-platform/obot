package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/nah/pkg/router"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/services"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type gatewayRouteBackend struct {
	backend.Backend
	storage   kclient.WithWatch
	callbacks map[schema.GroupVersionKind]backend.Callback
	retry     time.Duration
}

type gatewayRouteEngine struct {
	ready bool
	calls int
	sync  func(context.Context) (bool, error)
}

type gatewayRouteCodec struct{}

func (b *gatewayRouteBackend) GVKForObject(obj runtime.Object, s *runtime.Scheme) (schema.GroupVersionKind, error) {
	return apiutil.GVKForObject(obj, s)
}
func (b *gatewayRouteBackend) Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	return b.storage.Get(ctx, key, obj, opts...)
}
func (b *gatewayRouteBackend) Watcher(_ context.Context, gvk schema.GroupVersionKind, _ string, cb backend.Callback) error {
	b.callbacks[gvk] = cb
	return nil
}
func (*gatewayRouteBackend) Start(context.Context) error { return nil }
func (b *gatewayRouteBackend) Trigger(_ context.Context, _ schema.GroupVersionKind, _ string, delay time.Duration) error {
	b.retry = delay
	return nil
}
func (b *gatewayRouteBackend) event(ctx context.Context, t *testing.T, obj kclient.Object) error {
	t.Helper()
	gvk, err := b.GVKForObject(obj, scheme.Scheme)
	require.NoError(t, err)
	// Absent objects exercise the actual IncludeRemoved handler wiring.
	require.Contains(t, b.callbacks, gvk, "the router must register this resource")
	_, err = b.callbacks[gvk](ctx, gvk, obj.GetNamespace()+"/"+obj.GetName(), nil)
	return err
}

func (e *gatewayRouteEngine) Ready() bool { return e.ready }
func (e *gatewayRouteEngine) Sync(ctx context.Context) (bool, error) {
	e.calls++
	return e.sync(ctx)
}

func (gatewayRouteCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (gatewayRouteCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func TestGatewayRoutesLeaderOwnsImports(t *testing.T) {
	for _, obj := range []kclient.Object{&v1.LLMGatewayProvider{}, &v1.LLMGatewayRoutingRule{}, &v1.LLMGatewayConfig{}} {
		gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
		require.NoError(t, err)
		t.Run(gvk.Kind, func(t *testing.T) {
			ctx := t.Context()
			writes := 0
			failCreate := true
			importFailure := errors.New("temporary import failure")
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					writes++
					if _, ok := obj.(*v1.LLMGatewayProvider); ok && failCreate {
						return importFailure
					}
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					writes++
					return c.Update(ctx, obj, opts...)
				},
			}).Build()
			store, err := llmcoreclient.New(storage, gatewayRouteCodec{})
			require.NoError(t, err)
			importer := llmengine.New()
			require.NoError(t, importer.Start(ctx, llmengine.Options{Store: store, DataDir: t.TempDir()}))
			t.Cleanup(func() { require.NoError(t, importer.Shutdown(context.Background())) })
			g := newTestGatewayClient(t)
			t.Cleanup(func() { require.NoError(t, g.Close()) })
			require.NoError(t, g.UpsertCredential(ctx, gatewaytypes.Credential{
				Context: system.OpenAIModelProvider,
				Name:    system.OpenAIModelProvider,
				Secrets: map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "test-key"},
			}))
			newRouter := func(name string) (*router.Router, *gatewayRouteBackend) {
				b := &gatewayRouteBackend{storage: storage, callbacks: map[schema.GroupVersionKind]backend.Callback{}}
				return router.New(router.NewHandlerSet(name, scheme.Scheme, b), nil, 0), b
			}
			leader, leaderBackend := newRouter("leader")
			replica, replicaBackend := newRouter("replica")
			controller := &Controller{services: &services.Services{
				Router:             leader,
				EveryReplicaRouter: replica,
				BifrostEngine:      importer,
				GatewayClient:      g,
				StorageClient:      storage,
			}}
			engine := &gatewayRouteEngine{sync: func(context.Context) (bool, error) { return false, nil }}
			controller.setupLLMGatewayConfigRoutes(engine)
			require.NoError(t, leader.Start(ctx))
			require.NoError(t, replica.Start(ctx))
			obj.SetNamespace(system.DefaultNamespace)
			obj.SetName(system.LLMGatewayConfigName)
			before := writes
			require.NoError(t, replicaBackend.event(ctx, t, obj))
			require.Equal(t, time.Second, replicaBackend.retry)
			require.Equal(t, before, writes)
			require.Zero(t, engine.calls)
			engine.ready = true
			require.NoError(t, replicaBackend.event(ctx, t, obj))
			require.Equal(t, 1, engine.calls)
			require.Equal(t, before, writes, "standby resource events must never import")

			engine.ready = false
			require.NoError(t, leaderBackend.event(ctx, t, obj))
			require.Equal(t, time.Second, leaderBackend.retry)
			require.Equal(t, before, writes)
			engine.ready = true
			runtimeFailure := errors.New("temporary runtime failure")
			engine.sync = func(context.Context) (bool, error) { return false, runtimeFailure }
			err = leaderBackend.event(ctx, t, obj)
			require.ErrorIs(t, err, importFailure)
			require.ErrorIs(t, err, runtimeFailure, "failed import must still apply independent runtime state")
			require.Equal(t, 2, engine.calls)
			failCreate = false
			engine.sync = func(ctx context.Context) (bool, error) {
				_, err := store.GetProvider(ctx, "openai")
				require.NoError(t, err, "leader import must precede runtime application")
				return false, nil
			}
			require.NoError(t, leaderBackend.event(ctx, t, obj))
			require.Equal(t, 3, engine.calls)

			before = writes
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			require.ErrorIs(t, leaderBackend.event(canceled, t, obj), context.Canceled)
			require.Equal(t, before, writes)

			// Both routers retain namespace filters and the global settings singleton filter.
			for _, b := range []*gatewayRouteBackend{replicaBackend, leaderBackend} {
				obj.SetNamespace("other")
				require.NoError(t, b.event(ctx, t, obj))
				obj.SetNamespace(system.DefaultNamespace)
				if _, ok := obj.(*v1.LLMGatewayConfig); ok {
					obj.SetName("other")
					require.NoError(t, b.event(ctx, t, obj))
				}
			}
			require.Equal(t, 4, engine.calls)
			require.Equal(t, before, writes)
		})
	}
}
