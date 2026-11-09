package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/nah/pkg/backend"
	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/modeldata"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
		if strings.HasSuffix(r.URL.Path, "/responses") {
			_, _ = io.WriteString(w, `{"id":"r1","object":"response","status":"completed","model":"`+req.Model+`","output":[{"id":"msg1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
			return
		}
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
	var settingsReads atomic.Int64
	var failSettingsRead atomic.Bool
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource()).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if _, ok := obj.(*v1.LLMGatewayConfig); ok {
				settingsReads.Add(1)
				if failSettingsRead.Load() {
					return errors.New("request must use applied migration mappings")
				}
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	store, err := llmcoreclient.New(storageClient, legacyTestCodec{})
	require.NoError(t, err)
	engine := llmengine.New()
	require.NoError(t, engine.Start(ctx, llmengine.Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	_, err = engine.ImportLegacyModelProviders(ctx, storageClient, g)
	require.NoError(t, err)
	status, err := engine.MigrationStatus(ctx)
	require.NoError(t, err)
	status.Mappings["m1-old-openai"] = "m1-openai"
	status.Mappings["m1-old-generic"] = "m1-generic-qwen"
	migration, err := json.Marshal(status)
	require.NoError(t, err)
	require.NoError(t, store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
		settings["legacy-provider-migration"] = migration
		return nil
	}))
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
			name:  "mapped old model ID",
			path:  "/api/llm-proxy/openai/v1/chat/completions",
			model: "m1-old-openai",
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
			before := settingsReads.Load()
			failSettingsRead.Store(true)
			defer failSettingsRead.Store(false)
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
			require.Equal(t, before, settingsReads.Load(), "canonical and legacy requests must not read settings")
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

	for _, state := range []string{"manual", "recreated"} {
		t.Run(state+" provider without provenance", func(t *testing.T) {
			require.NoError(t, store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
				if state == "manual" {
					delete(settings, "legacy-provider-migration")
				} else {
					// Recreating a provider must work even with a stale imported outcome.
					settings["legacy-provider-migration"] = migration
				}
				return nil
			}))
			for _, provider := range []string{"openai", "generic-responses"} {
				p, err := store.GetProvider(ctx, provider)
				require.NoError(t, err)
				p.Migration = nil
				key := state + "-" + provider + "-key"
				p.Keys = []schemas.Key{{ID: "manual", Name: "manual", Value: *schemas.NewSecretVar(key), Models: schemas.WhiteList{"*"}, Weight: 1}}
				require.NoError(t, store.DeleteProvider(ctx, provider))
				require.NoError(t, store.CreateProvider(ctx, provider, p))
				_, err = engine.Sync(ctx)
				require.NoError(t, err)

				rec := httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+provider+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				lock.Lock()
				got := requests[len(requests)-1]
				lock.Unlock()
				require.Equal(t, "Bearer "+key, got.key, "legacy requests must use the current provider's keys")

				rec = httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-proxy/"+provider+"/v1/models", nil))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				var listing struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listing))
				require.Len(t, listing.Data, 1)
				require.Equal(t, provider+"/gpt-test", listing.Data[0].ID)
			}
		})
	}

	t.Run("slash-containing native IDs", func(t *testing.T) {
		for _, tc := range []struct {
			id     string
			target string
		}{
			{
				id:     "m1-generic-qwen",
				target: "generic-responses/Qwen/Qwen3-32B",
			},
			{
				id:     "m1-openai-qwen",
				target: "openai/Qwen/Qwen3-32B",
			},
			{
				id:     "m1-generic-nested",
				target: "generic-responses/team/family/model",
			},
			{
				id:     "m1-generic-collision",
				target: "generic-responses/openai/gpt-test",
			},
			{
				id:     "m1-generic-dated",
				target: "generic-responses/team/model-20261101",
			},
			{
				id:     "m1-generic-alias-collision",
				target: "generic-responses/llm",
			},
		} {
			model := &v1.Model{Namespace: system.DefaultNamespace, Name: tc.id, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
				ModelProvider: system.LLMGatewayModelProvider,
				TargetModel:   tc.target,
				Active:        true,
				Usage:         types.ModelUsageLLM,
				Dialect:       string(llmtypes.DialectOpenAIResponses),
			}}}
			require.NoError(t, storageClient.Create(ctx, model))
			gvk, err := storageClient.GroupVersionKindFor(model)
			require.NoError(t, err)
			require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
		}
		defaultAlias := &v1.DefaultModelAlias{Name: "default-llm", Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{
			Alias: "llm",
			Model: "m1-generic-qwen",
		}}}
		require.NoError(t, storageClient.Create(ctx, defaultAlias))
		require.NoError(t, storageClient.Create(ctx, &v1.Alias{Name: alias.KeyFromScopeID("Model", "llm"), Spec: v1.AliasSpec{
			TargetKind: "DefaultModelAlias",
			TargetName: defaultAlias.Name,
		}}))
		for _, tc := range []struct {
			name      string
			route     string
			reference string
			wantModel string
			wantKey   string
		}{
			{
				name:      "native slash name",
				route:     "generic-responses/v1",
				reference: "Qwen/Qwen3-32B",
				wantModel: "Qwen/Qwen3-32B",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "same native name in another provider",
				route:     "openai/v1",
				reference: "Qwen/Qwen3-32B",
				wantModel: "Qwen/Qwen3-32B",
				wantKey:   "recreated-openai-key",
			},
			{
				name:      "multiple native slashes",
				route:     "generic-responses/v1",
				reference: "team/family/model",
				wantModel: "team/family/model",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "native name precedes other provider target",
				route:     "generic-responses/v1",
				reference: "openai/gpt-test",
				wantModel: "openai/gpt-test",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "explicit same-provider name",
				route:     "generic-responses/v1",
				reference: "generic-responses/Qwen/Qwen3-32B",
				wantModel: "Qwen/Qwen3-32B",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "canonical qualified name",
				route:     "v1",
				reference: "openai/gpt-test",
				wantModel: "gpt-test",
				wantKey:   "recreated-openai-key",
			},
			{
				name:      "resource ID",
				route:     "generic-responses/v1",
				reference: "m1-generic-qwen",
				wantModel: "Qwen/Qwen3-32B",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "saved alias",
				route:     "generic-responses/v1",
				reference: "llm",
				wantModel: "Qwen/Qwen3-32B",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "mapped old resource ID",
				route:     "generic-responses/v1",
				reference: "m1-old-generic",
				wantModel: "Qwen/Qwen3-32B",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "canonical qualified slash name",
				route:     "v1",
				reference: "generic-responses/Qwen/Qwen3-32B",
				wantModel: "Qwen/Qwen3-32B",
				wantKey:   "recreated-generic-responses-key",
			},
			{
				name:      "undated native alias",
				route:     "generic-responses/v1",
				reference: "team/model",
				wantModel: "team/model-20261101",
				wantKey:   "recreated-generic-responses-key",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body, err := json.Marshal(map[string]any{"model": tc.reference, "input": "hi"})
				require.NoError(t, err)
				rec := httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+tc.route+"/responses", strings.NewReader(string(body))))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				lock.Lock()
				got := requests[len(requests)-1]
				lock.Unlock()
				require.Equal(t, "/v1/responses", got.path)
				require.Equal(t, "Bearer "+tc.wantKey, got.key)
				require.Equal(t, tc.wantModel, got.model)
			})
		}

		for _, tc := range []struct {
			name      string
			reference string
			userID    string
			wantCode  int
		}{
			{
				name:      "cross-provider primary",
				reference: "openai/Qwen/Qwen3-32B",
				userID:    "7",
				wantCode:  http.StatusBadRequest,
			},
			{
				name:      "cross-provider mapped resource",
				reference: "m1-old-openai",
				userID:    "7",
				wantCode:  http.StatusBadRequest,
			},
			{
				name:      "cross-provider alias",
				reference: "llm",
				userID:    "7",
				wantCode:  http.StatusBadRequest,
			},
			{
				name:      "unknown native name",
				reference: "unknown/family/model",
				userID:    "7",
				wantCode:  http.StatusNotFound,
			},
			{
				name:      "native name without permission",
				reference: "Qwen/Qwen3-32B",
				userID:    "8",
				wantCode:  http.StatusForbidden,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				mux.user = &kuser.DefaultInfo{UID: tc.userID}
				defer func() { mux.user = &kuser.DefaultInfo{UID: "7"} }()
				body, err := json.Marshal(map[string]any{"model": tc.reference, "input": "hi"})
				require.NoError(t, err)
				route := "generic-responses"
				if tc.name == "cross-provider alias" {
					route = "openai"
				}
				lock.Lock()
				before := len(requests)
				lock.Unlock()
				rec := httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+route+"/v1/responses", strings.NewReader(string(body))))
				require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
				lock.Lock()
				defer lock.Unlock()
				require.Len(t, requests, before, "a refused model must not reach inference")
			})
		}

		b := &bifrostProviderBackend{engine: engine}
		l := s.newBifrostLLMProviderProxy()
		req := api.Context{Request: httptest.NewRequest(http.MethodPost, "/", nil).WithContext(llmengine.WithLegacyProviderScope(ctx, "generic-responses")), Storage: storageClient, GatewayClient: g, User: mux.user}
		t.Run("native fallback", func(t *testing.T) {
			body, err := b.authorizeExtraModels(l, req, []byte(`{"fallbacks":["Qwen/Qwen3-32B","team/family/model","openai/gpt-test"]}`))
			require.NoError(t, err)
			var got struct {
				Fallbacks []string `json:"fallbacks"`
			}
			require.NoError(t, json.Unmarshal(body, &got))
			require.Equal(t, []string{"generic-responses/Qwen/Qwen3-32B", "generic-responses/team/family/model", "generic-responses/openai/gpt-test"}, got.Fallbacks)
		})
		t.Run("cross-provider slash fallback", func(t *testing.T) {
			_, err := b.authorizeExtraModels(l, req, []byte(`{"fallbacks":["openai/Qwen/Qwen3-32B"]}`))
			require.Error(t, err)
		})
		t.Run("denied native fallback", func(t *testing.T) {
			deniedReq := req
			deniedReq.User = &kuser.DefaultInfo{UID: "8"}
			body, err := b.authorizeExtraModels(l, deniedReq, []byte(`{"fallbacks":["Qwen/Qwen3-32B"]}`))
			require.NoError(t, err)
			require.JSONEq(t, `{"fallbacks":[]}`, string(body))
		})
		t.Run("cross-provider concrete routing collision", func(t *testing.T) {
			attempts := &gatewayAttempts{budgets: map[string][]string{}}
			err := attempts.checkAttempt(b, l, req, nil)(ctx, guardrails.Attempt{Model: "openai/gpt-test"})
			require.Error(t, err, "engine targets must not be reinterpreted as native names")
			require.Empty(t, attempts.admitted)
		})
		t.Run("concrete routing permissions", func(t *testing.T) {
			// This native ID collides with a concrete target inside the same provider.
			shadow := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-generic-shadow", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
				ModelProvider: system.LLMGatewayModelProvider,
				TargetModel:   "generic-responses/generic-responses/Qwen/Qwen3-32B",
				Active:        true,
				Usage:         types.ModelUsageLLM,
			}}}
			require.NoError(t, storageClient.Create(ctx, shadow))
			gvk, err := storageClient.GroupVersionKindFor(shadow)
			require.NoError(t, err)
			require.NoError(t, back.informers[gvk].GetIndexer().Add(shadow))
			restricted := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "restricted", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "9"}},
				Models:   []types.ModelResource{{ID: "m1-generic-qwen"}, {ID: "m1-openai"}},
			}}}
			gvk, err = storageClient.GroupVersionKindFor(restricted)
			require.NoError(t, err)
			require.NoError(t, back.informers[gvk].GetIndexer().Add(restricted))
			restrictedReq := req
			restrictedReq.User = &kuser.DefaultInfo{UID: "9"}
			attempts := &gatewayAttempts{budgets: map[string][]string{}}
			check := attempts.checkAttempt(b, l, restrictedReq, nil)
			require.NoError(t, check(ctx, guardrails.Attempt{Model: "generic-responses/Qwen/Qwen3-32B"}))
			require.Equal(t, "generic-responses/Qwen/Qwen3-32B", attempts.admitted)
			require.Error(t, check(ctx, guardrails.Attempt{Model: shadow.Spec.Manifest.TargetModel}))

			// Access to the other provider's target must not grant this native collision.
			_, err = b.authorizeModel(l, restrictedReq, "openai/gpt-test", "model")
			var httpErr *types.ErrHTTP
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, http.StatusForbidden, httpErr.Code)
		})
		t.Run("stale mapping cannot retry an unscoped name", func(t *testing.T) {
			require.NoError(t, store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
				var status llmengine.MigrationStatus
				if err := json.Unmarshal(settings["legacy-provider-migration"], &status); err != nil {
					return err
				}
				status.Mappings["openai/gpt-test"] = "m1-missing"
				status.Mappings["generic-responses/team/family/model"] = "m1-openai"
				body, err := json.Marshal(status)
				settings["legacy-provider-migration"] = body
				return err
			}))
			_, err = engine.Sync(ctx)
			require.NoError(t, err)
			before := len(requests)
			rec := httptest.NewRecorder()
			mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/generic-responses/v1/responses", strings.NewReader(`{"model":"openai/gpt-test","input":"hi"}`)))
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.Len(t, requests, before)
			attempts := &gatewayAttempts{budgets: map[string][]string{}}
			require.Error(t, attempts.checkAttempt(b, l, req, nil)(ctx, guardrails.Attempt{Model: "generic-responses/team/family/model"}))
			require.Empty(t, attempts.admitted, "a mapping must not authorize an out-of-scope concrete model")
		})
	})

	before := len(requests)
	// Fallbacks and routing rules must obey the same scope as the initial model.
	t.Run("cross-provider fallback", func(t *testing.T) {
		beforeReads := settingsReads.Load()
		failSettingsRead.Store(true)
		defer failSettingsRead.Store(false)
		rec := httptest.NewRecorder()
		mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/openai/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[],"fallbacks":["generic-responses/gpt-test"]}`)))
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		require.Len(t, requests, before, "an out-of-scope fallback must be rejected before inference")
		require.Equal(t, beforeReads, settingsReads.Load())
	})
	t.Run("cross-provider routing attempt", func(t *testing.T) {
		beforeReads := settingsReads.Load()
		failSettingsRead.Store(true)
		defer failSettingsRead.Store(false)
		b := &bifrostProviderBackend{engine: engine}
		l := s.newBifrostLLMProviderProxy()
		req := api.Context{Request: httptest.NewRequest(http.MethodPost, "/", nil).WithContext(llmengine.WithLegacyProviderScope(ctx, "openai")), Storage: storageClient, GatewayClient: g, User: mux.user}
		attempts := &gatewayAttempts{budgets: map[string][]string{}}
		err := attempts.checkAttempt(b, l, req, nil)(ctx, guardrails.Attempt{Model: "generic-responses/gpt-test"})
		require.Error(t, err, "a routing rule must not send a legacy request to another provider")
		require.Empty(t, attempts.admitted)
		require.Equal(t, beforeReads, settingsReads.Load())
	})
	unconfigured := httptest.NewRecorder()
	mux.mux.ServeHTTP(unconfigured, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/azure/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","messages":[]}`)))
	require.Equal(t, http.StatusNotFound, unconfigured.Code, "an unconfigured provider must use the normal gateway model error")
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
	listing := httptest.NewRecorder()
	mux.mux.ServeHTTP(listing, httptest.NewRequest(http.MethodGet, "/api/llm-proxy/openai/v1/models", nil))
	require.Equal(t, http.StatusOK, listing.Code, listing.Body.String())
	var body struct{ Data []any }
	require.NoError(t, json.Unmarshal(listing.Body.Bytes(), &body))
	require.Empty(t, body.Data, "model listing must retain access-policy filtering")
}

func TestLegacyProxyProviderMapping(t *testing.T) {
	for _, tc := range []struct {
		prefix   string
		provider string
	}{
		{
			prefix:   "openai",
			provider: "openai",
		},
		{
			prefix:   "anthropic",
			provider: "anthropic",
		},
		{
			prefix:   "generic-responses",
			provider: "generic-responses",
		},
		{
			prefix:   "azure",
			provider: "azure",
		},
		{
			prefix:   "azure-entra",
			provider: "azure",
		},
		{
			prefix:   "aws-bedrock",
			provider: "bedrock_mantle",
		},
		{
			prefix:   "aws-bedrock-api-key",
			provider: "bedrock_mantle",
		},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			require.Equal(t, tc.provider, llmengine.LegacyGatewayProvider(legacyProxySources[tc.prefix]))

			// Unsupported operations stay gone even without an engine or migration state.
			mux := &legacyTestMux{mux: http.NewServeMux()}
			s := &Server{}
			s.addLLMProxyRoutes(mux)
			rec := httptest.NewRecorder()
			mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+tc.prefix+"/v1/images/generations", nil))
			require.Equal(t, http.StatusGone, rec.Code, rec.Body.String())
		})
	}
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

func TestLegacyProxySubscriptionListingRetainsProviderScope(t *testing.T) {
	ctx := t.Context()
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storageClient, legacyTestCodec{})
	require.NoError(t, err)
	dataDir := t.TempDir()
	path, _ := modeldata.Paths(dataDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0750))
	require.NoError(t, os.WriteFile(path, []byte(`{"claude-test":{"provider":"anthropic","mode":"chat"}}`), 0600))
	engine := llmengine.New()
	require.NoError(t, engine.Start(ctx, llmengine.Options{
		Store:               store,
		DataDir:             dataDir,
		ClaudeSubscriptions: func(context.Context) (bool, error) { return true, nil },
	}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	_, err = engine.CreateProvider(ctx, json.RawMessage(`{"provider":"anthropic"}`))
	require.NoError(t, err)
	b := &bifrostProviderBackend{engine: engine}
	require.Len(t, b.unlistedModels(ctx), 1, "the canonical route must retain subscription discovery")
	require.Len(t, b.unlistedModels(llmengine.WithLegacyProviderScope(ctx, "anthropic")), 1)
	require.Empty(t, b.unlistedModels(llmengine.WithLegacyProviderScope(ctx, "openai")), "a scoped listing must not append another provider's subscription models")
}
