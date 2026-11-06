package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/obot-platform/nah/pkg/backend"
	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type legacyTestCodec struct{}

type legacyTestBackend struct {
	backend.Backend
	client    kclient.WithWatch
	informers map[schema.GroupVersionKind]cache.SharedIndexInformer
}

type legacyTestMux struct {
	mux     *http.ServeMux
	storage storage.Client
	gateway *gateway.Client
	user    kuser.Info
}

func (legacyTestCodec) SealGatewayConfig(_ context.Context, _ string, b []byte) ([]byte, error) {
	return b, nil
}
func (legacyTestCodec) OpenGatewayConfig(_ context.Context, _ string, b []byte) ([]byte, error) {
	return b, nil
}

func (b *legacyTestBackend) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return b.client.GroupVersionKindFor(obj)
}
func (b *legacyTestBackend) GetInformerForKind(_ context.Context, gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	if b.informers[gvk] == nil {
		obj, err := scheme.Scheme.New(gvk)
		if err != nil {
			return nil, err
		}
		b.informers[gvk] = cache.NewSharedIndexInformer(&cache.ListWatch{}, obj, 0, cache.Indexers{})
	}
	return b.informers[gvk], nil
}

func (m *legacyTestMux) HandleFunc(pattern string, handler api.HandlerFunc) {
	m.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		err := handler(api.Context{ResponseWriter: w, Request: r, Storage: m.storage, GatewayClient: m.gateway, User: m.user})
		if err != nil {
			code := http.StatusInternalServerError
			if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
				code = httpErr.Code
			}
			http.Error(w, err.Error(), code)
		}
	})
}

func TestLegacyProxyRoutesPreserveProviderAndProtocol(t *testing.T) {
	ctx := t.Context()
	g := newUsageTestClient(t)
	var lock sync.Mutex
	var requests []struct{ path, query, key, model string }
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-test","object":"model"}],"object":"list"}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		lock.Lock()
		requests = append(requests, struct{ path, query, key, model string }{path: r.URL.Path, query: r.URL.RawQuery, key: r.Header.Get("Authorization"), model: req.Model})
		lock.Unlock()
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
	}))
	t.Cleanup(up.Close)
	for name, secrets := range map[string]map[string]string{
		system.OpenAIModelProvider:           {"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "openai-key"},
		system.GenericResponsesModelProvider: {"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY": "generic-key", "OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "https://93.184.216.34/v1"},
	} {
		require.NoError(t, g.UpsertCredential(ctx, gatewaytypes.Credential{Context: name, Name: name, Secrets: secrets}))
	}
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource()).Build()
	store, err := llmcoreclient.New(storageClient, legacyTestCodec{})
	require.NoError(t, err)
	engine := llmengine.New()
	require.NoError(t, engine.Start(ctx, llmengine.Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	_, err = engine.ImportLegacyModelProviders(ctx, storageClient, g)
	require.NoError(t, err)
	for _, provider := range []string{"openai", "generic-responses"} {
		require.NoError(t, store.MutateProvider(ctx, provider, func(p *llmcoreclient.Provider) error {
			body, _ := json.Marshal(map[string]any{"base_url": up.URL, "allow_private_network": true})
			return json.Unmarshal(body, &p.Config.NetworkConfig)
		}))
	}
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	back := &legacyTestBackend{client: storageClient, informers: map[schema.GroupVersionKind]cache.SharedIndexInformer{}}
	helper, err := modelaccesspolicy.NewHelper(ctx, back)
	require.NoError(t, err)
	for provider, id := range map[string]string{"openai": "m1-openai", "generic-responses": "m1-generic"} {
		model := &v1.Model{Namespace: system.DefaultNamespace, Name: id, Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: provider + "/gpt-test", Active: true, Usage: "llm", Dialect: string(llmtypes.DialectOpenAIChatCompletions)}}}
		require.NoError(t, storageClient.Create(ctx, model))
		gvk, err := storageClient.GroupVersionKindFor(model)
		require.NoError(t, err)
		require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
	}
	policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, Models: []types.ModelResource{{ID: "*"}}}}}
	gvk, err := storageClient.GroupVersionKindFor(policy)
	require.NoError(t, err)
	require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
	mux := &legacyTestMux{mux: http.NewServeMux(), storage: storageClient, gateway: g, user: &kuser.DefaultInfo{UID: "7"}}
	s := &Server{llmGateway: engine, mapHelper: helper}
	s.addLLMProxyRoutes(mux)
	for _, tc := range []struct {
		name, path, model, key string
		stream                 bool
	}{
		{
			name:  "old OpenAI",
			path:  "/api/llm-proxy/openai/v1/chat/completions",
			model: "gpt-test",
			key:   "Bearer openai-key",
		},
		{
			name:  "old Generic Responses",
			path:  "/api/llm-proxy/generic-responses/v1/chat/completions",
			model: "gpt-test",
			key:   "Bearer generic-key",
		},
		{
			name:   "streaming",
			path:   "/api/llm-proxy/generic-responses/v1/chat/completions",
			model:  "gpt-test",
			key:    "Bearer generic-key",
			stream: true,
		},
		{
			name:  "canonical",
			path:  "/api/llm-proxy/v1/chat/completions",
			model: "generic-responses/gpt-test",
			key:   "Bearer generic-key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"model": tc.model, "stream": tc.stream, "messages": []any{map[string]any{"role": "user", "content": "hi"}}})
			req := httptest.NewRequest(http.MethodPost, tc.path+"?client=test", strings.NewReader(string(body)))
			req.Header.Set("Authorization", "Bearer client-secret")
			rec := httptest.NewRecorder()
			mux.mux.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "hello")
			lock.Lock()
			got := requests[len(requests)-1]
			lock.Unlock()
			require.Equal(t, "/v1/chat/completions", got.path)
			require.Equal(t, tc.key, got.key)
			require.Equal(t, "gpt-test", got.model)
		})
	}
	for _, route := range []string{"openai", "generic-responses"} {
		rec := httptest.NewRecorder()
		mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-proxy/"+route+"/v1/models", nil))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var listing struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listing))
		require.Len(t, listing.Data, 1)
		require.Equal(t, route+"/gpt-test", listing.Data[0].ID)
	}

	before := len(requests)
	denied := httptest.NewRecorder()
	mux.mux.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/openai/v1/chat/completions", strings.NewReader(`{"model":"generic-responses/gpt-test","messages":[]}`)))
	require.Equal(t, http.StatusBadRequest, denied.Code)
	require.Len(t, requests, before, "a legacy route must not select another provider")
	// No policy for this caller: compatibility must retain the access check.
	mux.user = &kuser.DefaultInfo{UID: "8"}
	denied = httptest.NewRecorder()
	mux.mux.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/generic-responses/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[]}`)))
	require.Equal(t, http.StatusForbidden, denied.Code)
	require.Len(t, requests, before)
}

func TestLegacyProxyPathNormalization(t *testing.T) {
	for _, tc := range []struct{ provider, path, want string }{
		{
			provider: "aws-bedrock",
			path:     "anthropic/v1/messages",
			want:     "v1/messages",
		},
		{
			provider: "aws-bedrock-api-key",
			path:     "openai/v1/responses",
			want:     "v1/responses",
		},
		{
			provider: "azure",
			path:     "openai/v1/models",
			want:     "v1/models",
		},
		{
			provider: "azure-entra",
			path:     "messages",
			want:     "v1/messages",
		},
		{
			provider: "generic-responses",
			path:     "v1/responses",
			want:     "v1/responses",
		},
	} {
		t.Run(tc.provider+tc.path, func(t *testing.T) {
			got, err := normalizeLegacyProxyPath(tc.provider, tc.path)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	_, err := normalizeLegacyProxyPath("aws-bedrock", "v1/images/generations")
	require.Error(t, err)
}
