package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modeldata"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type availabilityTestCodec struct{}

func (availabilityTestCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func (availabilityTestCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func TestGatewayModelSyncDoesNotRestoreUnavailableModels(t *testing.T) {
	mp := llmengine.ModelProviderResource()
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion})
	for _, kind := range []string{"Model", "ModelInfo", "ModelProvider"} {
		mapper.Add(v1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
	}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithRESTMapper(mapper).
		WithObjects(mp).
		WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
			return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
		}).WithInterceptorFuncs(interceptor.Funcs{List: listControllerObjects}).Build()
	var failed atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failed.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"listing unavailable"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	}))
	t.Cleanup(up.Close)
	store, err := llmcoreclient.New(storage, availabilityTestCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{
		Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{
			BaseURL:             up.URL,
			AllowPrivateNetwork: true,
		}},
		Keys: []schemas.Key{{
			ID:     "test-key",
			Name:   "test-key",
			Value:  *schemas.NewSecretVar("secret"),
			Models: schemas.WhiteList{"*"},
			Weight: 1,
		}},
	}))
	dir := t.TempDir()
	modeldatatest.Write(t, dir)
	pricingPath, _ := modeldata.Paths(dir)
	require.NoError(t, os.WriteFile(pricingPath, []byte(`{"test-model":{"provider":"openai","mode":"chat","input_cost_per_token":0.001}}`), 0600))
	engine := llmengine.New()
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: dir}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	d := dispatcher.New(nil, storage, nil, nil, "", "", "")
	d.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, engine)
	syncModels := func() (int, int) {
		t.Helper()
		require.NoError(t, BackPopulateModels(t.Context(), storage, d, mp))
		var models v1.ModelList
		require.NoError(t, storage.List(t.Context(), &models, kclient.InNamespace(system.DefaultNamespace)))
		var info v1.ModelInfoList
		require.NoError(t, storage.List(t.Context(), &info, kclient.InNamespace(system.DefaultNamespace)))
		return len(models.Items), len(info.Items)
	}
	models, prices := syncModels()
	require.Equal(t, 1, models)
	require.Equal(t, 1, prices)

	// Restart without an in-memory listing: outage retention must use persisted resources.
	require.NoError(t, engine.Shutdown(context.Background()))
	failed.Store(true)
	engine = llmengine.New()
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: dir}))
	d.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, engine)
	models, prices = syncModels()
	require.Equal(t, 1, models)
	require.Equal(t, 1, prices)

	failed.Store(false)
	require.NoError(t, os.WriteFile(pricingPath, []byte(`{"test-model":{"provider":"openai","mode":"chat","is_deprecated":true}}`), 0600))
	require.NoError(t, engine.RefreshPricing(t.Context()))
	models, prices = syncModels()
	require.Zero(t, models, "retention must not restore the provider's last unavailable model")
	require.Zero(t, prices, "retention must not restore its pricing either")
}
