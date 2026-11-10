package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-test","object":"model","created":17,"owned_by":"fixture"},{"id":"Qwen/Qwen3-32B","object":"model"},{"id":"generic-responses/Qwen/Qwen3-32B","object":"model"},{"id":"team/family/model","object":"model"},{"id":"openai/gpt-test","object":"model"},{"id":"anthropic/claude-test","object":"model"},{"id":"claude-test","object":"model"},{"id":"llm","object":"model"},{"id":"m1-generic-qwen","object":"model"},{"id":"m1-native-resource","object":"model"},{"id":"m1-old-generic","object":"model"},{"id":"m1-old-native","object":"model"},{"id":"m1-old-missing","object":"model"},{"id":"m1-old-inactive","object":"model"},{"id":"bare-target","object":"model"},{"id":"fresh-alias","object":"model"}],"object":"list"}`)
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
	var sourceReads atomic.Int64
	var failSourceRead atomic.Bool
	var failAliasRead atomic.Bool
	var failTargetAliasRead atomic.Bool
	var failListingSourceRead atomic.Bool
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource()).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if _, ok := obj.(*v1.Alias); ok && key.Name == alias.KeyFromScopeID("Model", "m1-missing-alias-target") && failTargetAliasRead.Load() {
				return errors.New("second-hop alias storage failed")
			}
			if _, ok := obj.(*v1.Model); ok && key.Name == "m1-old-inactive" && failListingSourceRead.Load() {
				return errors.New("listing migration source storage failed")
			}
			if _, ok := obj.(*v1.Alias); ok && key.Name == alias.KeyFromScopeID("Model", "llm") && failAliasRead.Load() {
				return errors.New("saved reference storage failed")
			}
			if _, ok := obj.(*v1.Model); ok && key.Name == "m1-old-openai" {
				sourceReads.Add(1)
				if failSourceRead.Load() {
					return errors.New("retained source read failed")
				}
			}
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
	t.Run("retained mapped source", func(t *testing.T) {
		source := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-old-openai", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			ModelProvider: system.OpenAIModelProvider,
			TargetModel:   "gpt-test",
			Active:        true,
		}}}
		require.NoError(t, storageClient.Create(ctx, source))
		var target v1.Model
		require.NoError(t, storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "m1-openai"}, &target))
		beforeSettings := settingsReads.Load()
		failSettingsRead.Store(true)
		defer failSettingsRead.Store(false)
		defer failSourceRead.Store(false)

		// Change only the retained source; the mapping stays cached throughout these requests.
		for _, state := range []struct {
			name       string
			active     bool
			deleted    bool
			readError  bool
			wantStatus int
		}{
			{
				name:       "active",
				active:     true,
				wantStatus: http.StatusOK,
			},
			{
				name:       "deactivated",
				wantStatus: http.StatusNotFound,
			},
			{
				name:       "reactivated",
				active:     true,
				wantStatus: http.StatusOK,
			},
			{
				name:       "deleted",
				deleted:    true,
				wantStatus: http.StatusOK,
			},
			{
				name:       "read failure",
				readError:  true,
				wantStatus: http.StatusInternalServerError,
			},
		} {
			t.Run(state.name, func(t *testing.T) {
				if state.deleted {
					require.NoError(t, storageClient.Delete(ctx, source))
				} else if !state.readError {
					source.Spec.Manifest.Active = state.active
					require.NoError(t, storageClient.Update(ctx, source))
				}
				failSourceRead.Store(state.readError)

				for _, route := range []string{"openai/v1", "v1"} {
					for _, tc := range []struct {
						name      string
						reference string
						fallback  string
						mapped    bool
					}{
						{
							name:      "old ID",
							reference: source.Name,
							mapped:    true,
						},
						{
							name:      "padded old ID",
							reference: " \t" + source.Name + "\n ",
							mapped:    true,
						},
						{
							name:      "gateway target ID",
							reference: target.Name,
						},
						{
							name:      "native target name",
							reference: "openai/gpt-test",
						},
						{
							name:      "mapped fallback",
							reference: target.Name,
							fallback:  source.Name,
							mapped:    true,
						},
					} {
						t.Run(route+"/"+tc.name, func(t *testing.T) {
							wantStatus := http.StatusOK
							var wantReads int64
							if tc.mapped {
								wantStatus = state.wantStatus
								wantReads = 1
							}
							if tc.fallback != "" && wantStatus != http.StatusOK {
								wantStatus = http.StatusBadRequest
							}
							beforeReads := sourceReads.Load()
							lock.Lock()
							before := len(requests)
							lock.Unlock()
							payload := map[string]any{"model": tc.reference, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}
							if tc.fallback != "" {
								payload["fallbacks"] = []string{tc.fallback}
							}
							body, err := json.Marshal(payload)
							require.NoError(t, err)
							rec := httptest.NewRecorder()
							mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+route+"/chat/completions", strings.NewReader(string(body))))
							wantDispatches := 0
							if wantStatus == http.StatusOK {
								wantDispatches = 1
								require.Contains(t, rec.Body.String(), "hello")
							}
							lock.Lock()
							gotDispatches := len(requests) - before
							lock.Unlock()
							require.Equal(t, wantDispatches, gotDispatches, "upstream inference calls; HTTP %d: %s", rec.Code, rec.Body.String())
							require.Equal(t, wantStatus, rec.Code, rec.Body.String())
							require.Equal(t, wantReads, sourceReads.Load()-beforeReads, "only mapped IDs read the retained source")
						})
					}
				}
				var currentTarget v1.Model
				require.NoError(t, storageClient.Get(ctx, kclient.ObjectKeyFromObject(&target), &currentTarget))
				require.Equal(t, target, currentTarget, "source changes must not mutate the gateway target")
			})
		}
		require.Equal(t, beforeSettings, settingsReads.Load(), "requests must use the cached mapping")
	})
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
		assert.Equal(t, "gpt-test", listing.Data[0].ID)
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
				assert.Equal(t, "gpt-test", listing.Data[0].ID)
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
				id:     "m1-generic-anthropic",
				target: "generic-responses/anthropic/claude-test",
			},
			{
				id:     "m1-anthropic",
				target: "anthropic/claude-test",
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
			{
				id:     "m1-generic-alias-denied",
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
			if tc.id == "m1-generic-alias-collision" {
				// Qualified target lookup keeps selecting the permitted record, independently of the alias.
				model.CreationTimestamp = metav1.NewTime(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
			}
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
		t.Run("saved alias listing collisions", func(t *testing.T) {
			var inactive v1.Model
			require.NoError(t, storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "m1-generic-qwen"}, &inactive))
			inactive.Name = "m1-inactive-alias-target"
			inactive.ResourceVersion = ""
			inactive.Spec.Manifest.Active = false
			require.NoError(t, storageClient.Create(ctx, &inactive))
			policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "native-only", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "11"}},
				Models:   []types.ModelResource{{ID: "m1-generic-alias-collision"}},
			}}}
			gvk, err := storageClient.GroupVersionKindFor(policy)
			require.NoError(t, err)
			require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
			defer func() {
				mux.user = &kuser.DefaultInfo{UID: "7"}
				defaultAlias.Spec.Manifest.Model = "m1-generic-qwen"
				require.NoError(t, storageClient.Update(ctx, defaultAlias))
			}()
			for _, tc := range []struct {
				name           string
				aliasTo        string
				user           string
				listed         bool
				wantCode       int
				wantModel      string
				failRead       bool
				failTargetRead bool
				grantDuplicate bool
			}{
				{
					name:      "authorized different target",
					aliasTo:   "m1-generic-qwen",
					user:      "7",
					wantCode:  http.StatusOK,
					wantModel: "Qwen/Qwen3-32B",
				},
				{
					name:     "unauthorized shadow",
					aliasTo:  "m1-generic-qwen",
					user:     "11",
					wantCode: http.StatusForbidden,
				},
				{
					name:      "same target alias",
					aliasTo:   "m1-generic-alias-collision",
					user:      "7",
					listed:    true,
					wantCode:  http.StatusOK,
					wantModel: "llm",
				},
				{
					name:     "denied duplicate selected by alias",
					aliasTo:  "m1-generic-alias-denied",
					user:     "11",
					wantCode: http.StatusForbidden,
				},
				{
					name:           "duplicate granted on next request",
					aliasTo:        "m1-generic-alias-denied",
					user:           "11",
					listed:         true,
					wantCode:       http.StatusOK,
					wantModel:      "llm",
					grantDuplicate: true,
				},
				{
					name:     "duplicate grant revoked on next request",
					aliasTo:  "m1-generic-alias-denied",
					user:     "11",
					wantCode: http.StatusForbidden,
				},
				{
					name:      "permitted duplicate selected by alias",
					aliasTo:   "m1-generic-alias-collision",
					user:      "11",
					listed:    true,
					wantCode:  http.StatusOK,
					wantModel: "llm",
				},
				{
					name:     "inactive shadow",
					aliasTo:  inactive.Name,
					user:     "7",
					wantCode: http.StatusInternalServerError,
				},
				{
					name:     "unconfigured alias",
					user:     "7",
					wantCode: http.StatusInternalServerError,
				},
				{
					name:     "other provider shadow",
					aliasTo:  "m1-openai-qwen",
					user:     "7",
					wantCode: http.StatusBadRequest,
				},
				{
					name:     "storage failure remains an error",
					aliasTo:  "m1-generic-qwen",
					user:     "7",
					wantCode: http.StatusInternalServerError,
					failRead: true,
				},
				{
					name:      "missing alias target retains native fallback",
					aliasTo:   "m1-missing-alias-target",
					user:      "7",
					listed:    true,
					wantCode:  http.StatusOK,
					wantModel: "llm",
				},
				{
					name:           "missing alias target second-hop storage failure",
					aliasTo:        "m1-missing-alias-target",
					user:           "7",
					wantCode:       http.StatusInternalServerError,
					failTargetRead: true,
				},
			} {
				for _, anthropic := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/anthropic=%v", tc.name, anthropic), func(t *testing.T) {
						policy.Spec.Manifest.Models = []types.ModelResource{{ID: "m1-generic-alias-collision"}}
						if tc.grantDuplicate {
							policy.Spec.Manifest.Models = append(policy.Spec.Manifest.Models, types.ModelResource{ID: "m1-generic-alias-denied"})
						}
						require.NoError(t, back.informers[gvk].GetIndexer().Update(policy))
						defaultAlias.Spec.Manifest.Model = tc.aliasTo
						require.NoError(t, storageClient.Update(ctx, defaultAlias))
						mux.user = &kuser.DefaultInfo{UID: tc.user}
						failAliasRead.Store(tc.failRead)
						defer failAliasRead.Store(false)
						failTargetAliasRead.Store(tc.failTargetRead)
						defer failTargetAliasRead.Store(false)
						req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/generic-responses/v1/models", nil)
						if anthropic {
							req.Header.Set("anthropic-version", "2023-06-01")
						}
						rec := httptest.NewRecorder()
						mux.mux.ServeHTTP(rec, req)
						if tc.failRead || tc.failTargetRead {
							require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
							require.Contains(t, rec.Body.String(), "failed to prepare gateway response")
						} else {
							require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
							ids := modelDataIDs(rec.Body.String())
							assert.Equal(t, tc.listed, slices.Contains(ids, "llm"), rec.Body.String())
							if tc.user == "7" {
								require.Contains(t, ids, "Qwen/Qwen3-32B", "omit the shadowed entry without replacing the real target")
							} else if tc.listed {
								assert.Equal(t, []string{"llm"}, ids)
							} else {
								assert.Empty(t, ids, "neither the shadowed entry nor unauthorized alias target may appear")
							}
						}
						if tc.aliasTo == "m1-generic-alias-denied" {
							selected, err := helper.ResolveModelReference(ctx, storageClient, system.DefaultNamespace, system.LLMGatewayModelProvider, "llm")
							require.NoError(t, err)
							require.Equal(t, tc.aliasTo, selected.Name, "alias must select the named record, not its permitted duplicate")
						}
						payload := map[string]any{"model": "llm", "input": "hi"}
						operation := "responses"
						if anthropic {
							operation = "messages"
							payload = map[string]any{"model": "llm", "max_tokens": 16, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}
						}
						body, err := json.Marshal(payload)
						require.NoError(t, err)
						lock.Lock()
						before := len(requests)
						lock.Unlock()
						rec = httptest.NewRecorder()
						mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/generic-responses/v1/"+operation, strings.NewReader(string(body))))
						require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
						if tc.failTargetRead {
							require.Contains(t, rec.Body.String(), "second-hop alias storage failed")
						}
						lock.Lock()
						got := slices.Clone(requests[before:])
						lock.Unlock()
						if tc.wantCode == http.StatusOK {
							require.Len(t, got, 1)
							require.Equal(t, tc.wantModel, got[0].model)
						} else {
							require.Empty(t, got)
						}
						// Unified qualified IDs bypass the scoped listing check and keep their meaning.
						rec = httptest.NewRecorder()
						mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/models", nil))
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						require.Contains(t, modelDataIDs(rec.Body.String()), "generic-responses/llm")
						rec = httptest.NewRecorder()
						mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/responses", strings.NewReader(`{"model":"generic-responses/llm","input":"hi"}`)))
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						lock.Lock()
						wireModel := requests[len(requests)-1].model
						lock.Unlock()
						require.Equal(t, "llm", wireModel)
					})
				}
			}
		})
		t.Run("saved resource and migration listing collisions", func(t *testing.T) {
			var original json.RawMessage
			require.NoError(t, store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
				original = slices.Clone(settings["legacy-provider-migration"])
				var status llmengine.MigrationStatus
				if err := json.Unmarshal(original, &status); err != nil {
					return err
				}
				status.Mappings["m1-old-native"] = "m1-listed-migration-same"
				status.Mappings["m1-old-missing"] = "m1-missing-listing"
				status.Mappings["m1-old-inactive"] = "m1-generic-qwen"
				status.Mappings["team/family/model"] = "m1-generic-qwen"
				body, err := json.Marshal(status)
				settings["legacy-provider-migration"] = body
				return err
			}))
			_, err := engine.Sync(ctx)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
					settings["legacy-provider-migration"] = original
					return nil
				}))
				_, err := engine.Sync(ctx)
				require.NoError(t, err)
			})
			for _, tc := range []struct {
				id     string
				native string
			}{
				{
					id:     "m1-listed-resource-shadow",
					native: "m1-generic-qwen",
				},
				{
					id:     "m1-native-resource",
					native: "m1-native-resource",
				},
				{
					id:     "m1-listed-migration-shadow",
					native: "m1-old-generic",
				},
				{
					id:     "m1-listed-migration-same",
					native: "m1-old-native",
				},
				{
					id:     "m1-listed-migration-missing",
					native: "m1-old-missing",
				},
				{
					id:     "m1-listed-migration-inactive",
					native: "m1-old-inactive",
				},
			} {
				model := &v1.Model{Namespace: system.DefaultNamespace, Name: tc.id, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.LLMGatewayModelProvider,
					TargetModel:   "generic-responses/" + tc.native,
					Active:        true,
					Usage:         types.ModelUsageLLM,
				}}}
				require.NoError(t, storageClient.Create(ctx, model))
				gvk, err := storageClient.GroupVersionKindFor(model)
				require.NoError(t, err)
				require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
				t.Cleanup(func() {
					require.NoError(t, back.informers[gvk].GetIndexer().Delete(model))
					require.NoError(t, storageClient.Delete(ctx, model))
				})
			}
			source := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-old-inactive", Spec: v1.ModelSpec{Manifest: types.ModelManifest{Active: false}}}
			require.NoError(t, storageClient.Create(ctx, source))
			t.Cleanup(func() { require.NoError(t, storageClient.Delete(ctx, source)) })
			for _, anthropic := range []bool{false, true} {
				t.Run(fmt.Sprintf("anthropic=%v", anthropic), func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/generic-responses/v1/models", nil)
					if anthropic {
						req.Header.Set("anthropic-version", "2023-06-01")
					}
					rec := httptest.NewRecorder()
					mux.mux.ServeHTTP(rec, req)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					ids := modelDataIDs(rec.Body.String())
					for _, hidden := range []string{"m1-generic-qwen", "m1-old-generic", "m1-old-missing", "m1-old-inactive", "team/family/model"} {
						require.NotContains(t, ids, hidden)
					}
					for _, native := range []string{"m1-native-resource", "m1-old-native"} {
						require.Contains(t, ids, native)
						rec := httptest.NewRecorder()
						body := `{"model":"` + native + `","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
						mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/generic-responses/v1/messages", strings.NewReader(body)))
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						lock.Lock()
						wire := requests[len(requests)-1].model
						lock.Unlock()
						require.Equal(t, native, wire)
					}
					failListingSourceRead.Store(true)
					defer failListingSourceRead.Store(false)
					rec = httptest.NewRecorder()
					mux.mux.ServeHTTP(rec, req)
					require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), "failed to prepare gateway response")
				})
			}
		})
		t.Run("bare reference collision freshness", func(t *testing.T) {
			var exact *v1.Model
			for _, target := range []string{"generic-responses/bare-target", "bare-target", "generic-responses/fresh-alias"} {
				model := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-bare-" + strings.ReplaceAll(target, "/", "-"), Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.LLMGatewayModelProvider,
					TargetModel:   target,
					Active:        true,
					Usage:         types.ModelUsageLLM,
				}}}
				require.NoError(t, storageClient.Create(ctx, model))
				gvk, err := storageClient.GroupVersionKindFor(model)
				require.NoError(t, err)
				require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
				t.Cleanup(func() {
					require.NoError(t, back.informers[gvk].GetIndexer().Delete(model))
					require.NoError(t, storageClient.Delete(ctx, model))
				})
				if target == "bare-target" {
					exact = model
				}
			}
			gvk, err := storageClient.GroupVersionKindFor(exact)
			require.NoError(t, err)
			for _, active := range []bool{true, false, true} {
				exact = exact.DeepCopy()
				exact.Spec.Manifest.Active = active
				require.NoError(t, storageClient.Update(ctx, exact))
				require.NoError(t, back.informers[gvk].GetIndexer().Update(exact))
				for _, anthropic := range []bool{false, true} {
					req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/generic-responses/v1/models", nil)
					if anthropic {
						req.Header.Set("anthropic-version", "2023-06-01")
					}
					rec := httptest.NewRecorder()
					mux.mux.ServeHTTP(rec, req)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, !active, slices.Contains(modelDataIDs(rec.Body.String()), "bare-target"), "only an active unqualified gateway target intercepts the scoped native name")
				}
				lock.Lock()
				before := len(requests)
				lock.Unlock()
				rec := httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/generic-responses/v1/responses", strings.NewReader(`{"model":"bare-target","input":"hi"}`)))
				if active {
					require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
					lock.Lock()
					dispatches := len(requests) - before
					lock.Unlock()
					require.Zero(t, dispatches)
				} else {
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				}
			}
			saved := &v1.Alias{Name: alias.KeyFromScopeID("Model", "fresh-alias"), Spec: v1.AliasSpec{
				TargetKind:      "Model",
				TargetNamespace: system.DefaultNamespace,
				TargetName:      "m1-generic-qwen",
			}}
			t.Cleanup(func() { require.NoError(t, kclient.IgnoreNotFound(storageClient.Delete(ctx, saved))) })
			for i, present := range []bool{false, true, false} {
				if present {
					require.NoError(t, storageClient.Create(ctx, saved))
				} else if i > 0 {
					require.NoError(t, storageClient.Delete(ctx, saved))
				}
				for _, anthropic := range []bool{false, true} {
					req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/generic-responses/v1/models", nil)
					if anthropic {
						req.Header.Set("anthropic-version", "2023-06-01")
					}
					rec := httptest.NewRecorder()
					mux.mux.ServeHTTP(rec, req)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, !present, slices.Contains(modelDataIDs(rec.Body.String()), "fresh-alias"), "new and deleted alias keys must be visible on the next listing")
				}
			}
		})
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
			t.Run("listed native IDs round trip", func(t *testing.T) {
				shadowOnly := restricted.DeepCopy()
				shadowOnly.Name = "shadow-only"
				shadowOnly.Spec.Manifest.Subjects = []types.Subject{{Type: types.SubjectTypeUser, ID: "10"}}
				shadowOnly.Spec.Manifest.Models = []types.ModelResource{{ID: shadow.Name}}
				require.NoError(t, back.informers[gvk].GetIndexer().Add(shadowOnly))
				defer func() { mux.user = &kuser.DefaultInfo{UID: "7"} }()
				for _, tc := range []struct {
					name string
					user string
					want []string
				}{
					{
						name: "first only",
						user: "9",
						want: []string{"Qwen/Qwen3-32B"},
					},
					{
						name: "second only",
						user: "10",
						want: []string{"generic-responses/Qwen/Qwen3-32B"},
					},
					{
						name: "both and other native slash names",
						user: "7",
						want: []string{"gpt-test", "Qwen/Qwen3-32B", "generic-responses/Qwen/Qwen3-32B", "team/family/model", "openai/gpt-test", "anthropic/claude-test"},
					},
					{
						name: "no permission",
						user: "8",
					},
				} {
					for _, anthropic := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/anthropic=%v", tc.name, anthropic), func(t *testing.T) {
							mux.user = &kuser.DefaultInfo{UID: tc.user}
							req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/generic-responses/v1/models", nil)
							if anthropic {
								req.Header.Set("anthropic-version", "2023-06-01")
							}
							rec := httptest.NewRecorder()
							mux.mux.ServeHTTP(rec, req)
							require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
							ids := modelDataIDs(rec.Body.String())
							assert.ElementsMatch(t, tc.want, ids, rec.Body.String())
							if !anthropic && tc.user == "7" {
								require.Equal(t, int64(17), gjson.Get(rec.Body.String(), `data.#(id=="gpt-test").created`).Int())
								require.Equal(t, "fixture", gjson.Get(rec.Body.String(), `data.#(id=="gpt-test").owned_by`).String())
							}
							for _, id := range ids {
								payload := map[string]any{"model": id, "input": "hi"}
								operation := "responses"
								if anthropic {
									operation = "messages"
									payload = map[string]any{"model": id, "max_tokens": 16, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}
								}
								body, err := json.Marshal(payload)
								require.NoError(t, err)
								rec := httptest.NewRecorder()
								mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/generic-responses/v1/"+operation, strings.NewReader(string(body))))
								require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
								lock.Lock()
								got := requests[len(requests)-1]
								lock.Unlock()
								require.Equal(t, id, got.model, "listed native ID must select its own wire model")
							}
							if tc.user == "9" {
								// Explicit slash inputs still prefer the native shadow, even when denied.
								lock.Lock()
								before := len(requests)
								lock.Unlock()
								rec := httptest.NewRecorder()
								mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/generic-responses/v1/responses", strings.NewReader(`{"model":"generic-responses/Qwen/Qwen3-32B","input":"hi"}`)))
								require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
								lock.Lock()
								dispatches := len(requests) - before
								lock.Unlock()
								require.Zero(t, dispatches)
							}
						})
					}
				}
				mux.user = &kuser.DefaultInfo{UID: "7"}
				rec := httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/models", nil))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				ids := modelDataIDs(rec.Body.String())
				for _, id := range []string{"generic-responses/Qwen/Qwen3-32B", shadow.Spec.Manifest.TargetModel} {
					require.Contains(t, ids, id)
					body, err := json.Marshal(map[string]any{"model": id, "input": "hi"})
					require.NoError(t, err)
					rec := httptest.NewRecorder()
					mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/responses", strings.NewReader(string(body))))
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					lock.Lock()
					got := requests[len(requests)-1]
					lock.Unlock()
					require.Equal(t, strings.TrimPrefix(id, "generic-responses/"), got.model)
				}
			})
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
	back := &legacyTestBackend{client: storageClient, informers: map[schema.GroupVersionKind]cache.SharedIndexInformer{}}
	helper, err := modelaccesspolicy.NewHelper(ctx, back)
	require.NoError(t, err)
	model := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-subscription", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
		ModelProvider: system.LLMGatewayModelProvider,
		TargetModel:   "anthropic/claude-test",
		Active:        true,
		Usage:         types.ModelUsageLLM,
		Dialect:       string(llmtypes.DialectAnthropicMessages),
	}}}
	gvk, err := storageClient.GroupVersionKindFor(model)
	require.NoError(t, err)
	require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
	policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
		Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
		Models:   []types.ModelResource{{ID: model.Name}},
	}}}
	gvk, err = storageClient.GroupVersionKindFor(policy)
	require.NoError(t, err)
	require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
	for _, route := range []string{"anthropic/v1", "v1"} {
		for _, anthropic := range []bool{false, true} {
			for _, user := range []string{"7", "8"} {
				t.Run(fmt.Sprintf("%s/anthropic=%v/user=%s", route, anthropic, user), func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/"+route+"/models", nil)
					req.SetPathValue("path", "v1/models")
					if route == "anthropic/v1" {
						req = req.WithContext(llmengine.WithLegacyProviderScope(ctx, "anthropic"))
					}
					r := &llmResponseWriter{mapHelper: helper, modelProvider: system.LLMGatewayModelProvider, user: &kuser.DefaultInfo{UID: user}, unlisted: b.unlistedModels}
					if anthropic {
						r.routeDialect = llmtypes.DialectAnthropicMessages
					}
					// A successful empty page must still filter subscription additions by access.
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req, Body: io.NopCloser(strings.NewReader(`{"data":[],"has_more":false}`))}
					require.NoError(t, r.prepareUnary(resp))
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					if user == "8" {
						require.Empty(t, modelDataIDs(string(body)), "synthetic entries must be filtered by access")
					} else {
						id := "claude-test"
						if route == "v1" && !anthropic {
							id = "anthropic/" + id
						}
						require.Equal(t, []string{id}, modelDataIDs(string(body)))
					}
				})
			}
		}
	}
}
