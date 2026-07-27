package modelinfosource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// sampleAPIJSON covers known providers, an unknown provider, and tiered cost.
const sampleAPIJSON = `{
  "anthropic": {
    "models": {
      "claude-opus-4-5": {"cost": {"input": 5, "output": 25, "cache_read": 0.5, "cache_write": 6.25}}
    }
  },
  "openai": {
    "models": {
      "gpt-4o": {"cost": {"input": 2.5, "output": 10, "cache_read": 1.25}},
      "gpt-5": {"cost": {"input": 2.5, "output": 15, "cache_read": 0.25, "tiers": [
        {"input": 5, "output": 22.5, "cache_read": 0.5, "tier": {"type": "context", "size": 272000}},
        {"input": 9, "output": 9, "tier": {"type": "context", "size": 0}},
        {"input": 10, "output": 10, "tier": {"type": "context"}},
        {"input": 11, "output": 11, "tier": {"type": "other", "size": 1000}}
      ]}}
    }
  },
  "cohere": {
    "models": {
      "command-r": {"cost": {"input": 0.5, "output": 1.5}}
    }
  }
}`

func mustDecodeDoc(t *testing.T, raw string) modelsDevDocument {
	t.Helper()
	var doc modelsDevDocument
	require.NoError(t, json.Unmarshal([]byte(raw), &doc))
	return doc
}

func TestParseModelInfos(t *testing.T) {
	infos, err := parseModelInfos(system.DefaultNamespace, "default", mustDecodeDoc(t, sampleAPIJSON))
	require.NoError(t, err)
	require.Len(t, infos, 3, "anthropic + 2 openai, cohere dropped")

	byModel := map[string]v1.ModelInfoSpec{}
	for _, info := range infos {
		modelInfo := info.(*v1.ModelInfo)
		assert.Equal(t, system.DefaultNamespace, modelInfo.Namespace)
		assert.Equal(t, "default", modelInfo.Spec.ModelInfoSourceName)
		assert.NotEmpty(t, modelInfo.Name)
		byModel[modelInfo.Spec.Model] = modelInfo.Spec
	}

	a := byModel["claude-opus-4-5"]
	assert.Equal(t, system.AnthropicModelProvider, a.Provider)
	assert.Equal(t, 5.0, a.Cost.Input)
	assert.Equal(t, 25.0, a.Cost.Output)
	assert.Equal(t, 6.25, a.Cost.CacheWrite)
	assert.Equal(t, 10.0, a.Cost.CacheWrite1h, "anthropic 1h cache is 2x input")
	assert.Empty(t, a.Cost.Tiers)

	o := byModel["gpt-4o"]
	assert.Equal(t, system.OpenAIModelProvider, o.Provider)
	assert.Zero(t, o.Cost.CacheWrite, "absent in source")
	assert.Zero(t, o.Cost.CacheWrite1h, "non-anthropic gets no 1h cache")

	g := byModel["gpt-5"]
	require.Len(t, g.Cost.Tiers, 1, "only the context tier with a positive size is kept")
	tier := g.Cost.Tiers[0]
	assert.Equal(t, types.ModelCostTierTypeContext, tier.Type)
	require.NotNil(t, tier.Size)
	assert.Equal(t, 272000, *tier.Size)
	assert.Equal(t, 5.0, tier.Input)
}

func TestParseModelInfos_NoKnownProviders(t *testing.T) {
	_, err := parseModelInfos(system.DefaultNamespace, "default",
		mustDecodeDoc(t, `{"cohere":{"models":{"command-r":{"cost":{"input":0.5}}}}}`))
	require.Error(t, err)
}

func TestSync(t *testing.T) {
	ctx := t.Context()

	t.Run("skips recent sync", func(t *testing.T) {
		called := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		source := newModelInfoSource(server.URL)
		source.Status.LastSyncTime = metav1.NewTime(time.Now().Add(-time.Minute))
		c := newFakeClient(t, source)

		resp := &router.ResponseWrapper{}
		require.NoError(t, newTestHandler(server).Sync(newRequest(ctx, c, source), resp))
		assert.False(t, called)
		assert.Greater(t, resp.Delay, syncInterval-2*time.Minute)
		assert.LessOrEqual(t, resp.Delay, syncInterval)
	})

	t.Run("records error and clears refresh annotation", func(t *testing.T) {
		server := newModelInfoServer(t, http.StatusInternalServerError, "boom")
		defer server.Close()

		source := newModelInfoSource(server.URL)
		source.Annotations = map[string]string{v1.ModelInfoSourceSyncAnnotation: "true"}
		c := newFakeClient(t, source)

		resp := &router.ResponseWrapper{}
		require.NoError(t, newTestHandler(server).Sync(newRequest(ctx, c, source), resp))

		var updated v1.ModelInfoSource
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: source.Namespace, Name: source.Name}, &updated))
		assert.Contains(t, updated.Status.SyncError, "unexpected status 500")
		assert.False(t, updated.Status.LastSyncTime.IsZero())
		_, hasSync := updated.Annotations[v1.ModelInfoSourceSyncAnnotation]
		assert.False(t, hasSync)
		assert.Equal(t, syncInterval, resp.Delay)
	})
}

func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.ModelInfoSource{}).
		WithObjects(objects...).
		Build()
}

func newModelInfoSource(sourceURL string) *v1.ModelInfoSource {
	return &v1.ModelInfoSource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "ModelInfoSource",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.ModelInfoSourceSpec{
			Manifest: types.ModelInfoSourceManifest{URL: sourceURL},
		},
	}
}

func newRequest(ctx context.Context, c kclient.WithWatch, source *v1.ModelInfoSource) router.Request {
	return router.Request{
		Client:    c,
		Object:    source,
		Ctx:       ctx,
		Namespace: source.Namespace,
		Name:      source.Name,
		Key:       source.Namespace + "/" + source.Name,
	}
}

func newTestHandler(server *httptest.Server) *Handler {
	return &Handler{httpClient: server.Client()}
}

func newModelInfoServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}))
}
