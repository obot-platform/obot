package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// Count storage and informer work separately: an indexed resolver can still rebuild
// all grants for every candidate even when it avoids storage reads.
type listingOperations struct {
	pointReads, modelLists, aliasLists atomic.Int64
	enumerations, indexReads           atomic.Int64
	failList, failIndexRead            atomic.Int64
}

type listingCountingBackend struct {
	*legacyTestBackend
	operations *listingOperations
}

type listingCountingInformer struct {
	cache.SharedIndexInformer
	operations *listingOperations
}

type listingCountingIndexer struct {
	cache.Indexer
	operations *listingOperations
}

func TestModelListingEligibility(t *testing.T) {
	for _, endpoint := range []struct {
		backend string
		scope   string
	}{
		{
			backend: system.LLMGatewayModelProvider,
			scope:   "azure",
		},
		{
			backend: system.LLMGatewayModelProvider,
			scope:   "generic-responses",
		},
		{
			backend: system.LLMGatewayModelProvider,
			scope:   "bedrock",
		},
		{
			backend: system.AzureModelProvider,
			scope:   "azure",
		},
	} {
		provider := endpoint.backend
		t.Run(provider+"/"+endpoint.scope, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			back := &legacyTestBackend{client: c, informers: map[schema.GroupVersionKind]cache.SharedIndexInformer{}}
			helper, err := modelaccesspolicy.NewHelper(t.Context(), back)
			require.NoError(t, err)
			for _, tc := range []struct {
				id     string
				active bool
				usage  types.ModelUsage
			}{
				{
					id:     "deployment",
					active: true,
					usage:  types.ModelUsageLLM,
				},
				{
					id:    "inactive",
					usage: types.ModelUsageLLM,
				},
				{
					id:     "embedding",
					active: true,
					usage:  types.ModelUsageEmbedding,
				},
				{
					id:     "denied",
					active: true,
					usage:  types.ModelUsageLLM,
				},
			} {
				target := tc.id
				if provider == system.LLMGatewayModelProvider {
					target = endpoint.scope + "/" + target
				}
				m := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-" + tc.id, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: provider,
					TargetModel:   target,
					Active:        tc.active,
					Usage:         tc.usage,
					Dialect:       string(llmtypes.DialectOpenAIResponses),
				}}}
				gvk, err := c.GroupVersionKindFor(m)
				require.NoError(t, err)
				require.NoError(t, back.informers[gvk].GetIndexer().Add(m))
			}
			policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
				Models:   []types.ModelResource{{ID: "m1-deployment"}, {ID: "m1-inactive"}, {ID: "m1-embedding"}},
			}}}
			gvk, err := c.GroupVersionKindFor(policy)
			require.NoError(t, err)
			require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
			req := httptest.NewRequest(http.MethodGet, "/anthropic/v1/models", nil).WithContext(llmengine.WithLegacyProviderScope(t.Context(), endpoint.scope))
			req.SetPathValue("path", "v1/models")
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"deployment"},{"id":"inactive"},{"id":"embedding"},{"id":"denied"}],"has_more":false,"first_id":"inactive","last_id":"denied"}`))}
			r := &llmResponseWriter{mapHelper: helper, modelProvider: provider, user: &kuser.DefaultInfo{UID: "7"}, routeDialect: llmtypes.DialectAnthropicMessages}
			require.NoError(t, r.prepareUnary(resp))
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			if provider == system.LLMGatewayModelProvider {
				require.Equal(t, []string{"deployment"}, modelDataIDs(string(body)))
			} else {
				require.Empty(t, modelDataIDs(string(body)), "legacy backends retain preferred-dialect filtering")
			}
		})
	}
}

func TestModelListingNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name      string
		scope     string
		anthropic bool
		targets   []string
		body      string
		want      []string
	}{
		{
			name:      "other provider target equals selected native name",
			scope:     "generic-responses",
			anthropic: true,
			targets:   []string{"generic-responses/anthropic/claude-test", "anthropic/claude-test"},
			body:      `{"data":[{"id":"anthropic/claude-test"},{"id":"claude-test"}],"has_more":false,"first_id":"claude-test","last_id":"claude-test"}`,
			want:      []string{"anthropic/claude-test"},
		},
		{
			name:    "qualified permission cannot expose unauthorized native collision",
			scope:   "generic-responses",
			targets: []string{"generic-responses/Qwen/Qwen3-32B"},
			body:    `{"data":[{"id":"Qwen/Qwen3-32B"},{"id":"generic-responses/Qwen/Qwen3-32B"}]}`,
			want:    []string{"Qwen/Qwen3-32B"},
		},
		{
			name:      "global Anthropic permissions belong to Anthropic only",
			anthropic: true,
			targets:   []string{"openai/claude-test"},
			body:      `{"data":[{"id":"claude-test"}],"has_more":false}`,
		},
		{
			name:    "global OpenAI permissions remain qualified only",
			targets: []string{"generic-responses/anthropic/claude-test"},
			body:    `{"data":[{"id":"anthropic/claude-test"}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			back := &legacyTestBackend{client: c, informers: map[schema.GroupVersionKind]cache.SharedIndexInformer{}}
			helper, err := modelaccesspolicy.NewHelper(t.Context(), back)
			require.NoError(t, err)
			for i, target := range tc.targets {
				m := &v1.Model{Namespace: system.DefaultNamespace, Name: fmt.Sprintf("m1-%d", i), Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.LLMGatewayModelProvider,
					TargetModel:   target,
					Active:        true,
					Usage:         types.ModelUsageLLM,
					Dialect:       string(llmtypes.DialectAnthropicMessages),
				}}}
				gvk, err := c.GroupVersionKindFor(m)
				require.NoError(t, err)
				require.NoError(t, back.informers[gvk].GetIndexer().Add(m))
			}
			policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
				Models:   []types.ModelResource{{ID: "*"}},
			}}}
			gvk, err := c.GroupVersionKindFor(policy)
			require.NoError(t, err)
			require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(llmengine.WithLegacyProviderScope(t.Context(), tc.scope))
			req.SetPathValue("path", "v1/models")
			r := &llmResponseWriter{mapHelper: helper, modelProvider: system.LLMGatewayModelProvider, user: &kuser.DefaultInfo{UID: "7"}}
			if tc.anthropic {
				r.routeDialect = llmtypes.DialectAnthropicMessages
			}
			for range 32 {
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req, Body: io.NopCloser(strings.NewReader(tc.body))}
				require.NoError(t, r.prepareUnary(resp))
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, tc.want, modelDataIDs(string(body)))
				if tc.anthropic && len(tc.want) > 0 {
					assertCursor(t, string(body), "first_id", &tc.want[0])
					assertCursor(t, string(body), "last_id", &tc.want[len(tc.want)-1])
				}
			}
		})
	}
}

func TestLegacyAzureModelListingAcrossDialects(t *testing.T) {
	ctx := t.Context()
	paths := make(chan string, 4)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		paths <- r.URL.Path + " " + extractModelFromBody(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"r1","object":"response","status":"completed","model":"deployment-blue","output":[{"id":"msg1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
	}))
	t.Cleanup(up.Close)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource()).Build()
	store, err := llmcoreclient.New(c, legacyTestCodec{})
	require.NoError(t, err)
	engine := llmengine.New()
	require.NoError(t, engine.Start(ctx, llmengine.Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	_, err = engine.CreateProvider(ctx, []byte(`{"provider":"azure","network_config":{"allow_private_network":true}}`))
	require.NoError(t, err)
	key, err := json.Marshal(map[string]any{
		"name":             "test",
		"value":            "secret",
		"models":           []string{"deployment-blue"},
		"azure_key_config": map[string]any{"endpoint": up.URL},
	})
	require.NoError(t, err)
	_, err = engine.CreateKey(ctx, "azure", key)
	require.NoError(t, err)
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	back := &legacyTestBackend{client: c, informers: map[schema.GroupVersionKind]cache.SharedIndexInformer{}}
	helper, err := modelaccesspolicy.NewHelper(ctx, back)
	require.NoError(t, err)
	model := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-azure", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
		ModelProvider: system.LLMGatewayModelProvider,
		TargetModel:   "azure/deployment-blue",
		Active:        true,
		Usage:         types.ModelUsageLLM,
		Dialect:       string(llmtypes.DialectOpenAIResponses),
	}}}
	require.NoError(t, c.Create(ctx, model))
	gvk, err := c.GroupVersionKindFor(model)
	require.NoError(t, err)
	require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
	policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
		Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
		Models:   []types.ModelResource{{ID: model.Name}},
	}}}
	gvk, err = c.GroupVersionKindFor(policy)
	require.NoError(t, err)
	require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
	mux := &legacyTestMux{mux: http.NewServeMux(), storage: c, gateway: newUsageTestClient(t), user: &kuser.DefaultInfo{UID: "7"}}
	s := &Server{llmGateway: engine, mapHelper: helper}
	s.addLLMProxyRoutes(mux)
	for _, route := range []string{"azure", "azure-entra"} {
		for _, anthropic := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/anthropic=%v", route, anthropic), func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/"+route+"/v1/models", nil)
				if anthropic {
					req.Header.Set("anthropic-version", "2023-06-01")
				}
				rec := httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, req)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, []string{"deployment-blue"}, modelDataIDs(rec.Body.String()))
				rec = httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+route+"/v1/messages", strings.NewReader(`{"model":"deployment-blue","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				wire := <-paths
				require.Contains(t, rec.Body.String(), "hello")
				require.Equal(t, "/openai/v1/responses deployment-blue", wire)
			})
		}
	}
}

func (b *listingCountingBackend) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	i, err := b.legacyTestBackend.GetInformerForKind(ctx, gvk)
	return listingCountingInformer{SharedIndexInformer: i, operations: b.operations}, err
}

func (i listingCountingInformer) GetIndexer() cache.Indexer {
	return listingCountingIndexer{Indexer: i.SharedIndexInformer.GetIndexer(), operations: i.operations}
}

func (i listingCountingIndexer) List() []any {
	i.operations.enumerations.Add(1)
	return i.Indexer.List()
}

func (i listingCountingIndexer) ListIndexFuncValues(name string) []string {
	i.operations.enumerations.Add(1)
	return i.Indexer.ListIndexFuncValues(name)
}

func (i listingCountingIndexer) ByIndex(name, value string) ([]any, error) {
	if n := i.operations.indexReads.Add(1); n == i.operations.failIndexRead.Load() {
		return nil, errors.New("listing grant snapshot failed")
	}
	return i.Indexer.ByIndex(name, value)
}

func TestScopedModelListingBoundedOperations(t *testing.T) {
	for _, size := range []int{1, 500} {
		for _, slash := range []bool{false, true} {
			t.Run(fmt.Sprintf("size=%d/slash=%v", size, slash), func(t *testing.T) {
				ctx := t.Context()
				operations := &listingOperations{}
				c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource()).WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
						switch obj.(type) {
						case *v1.Model, *v1.Alias:
							operations.pointReads.Add(1)
						}
						return c.Get(ctx, key, obj, opts...)
					},
					List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
						switch list.(type) {
						case *v1.ModelList:
							operations.modelLists.Add(1)
							if operations.failList.Load() == 1 {
								return errors.New("model snapshot failed")
							}
						case *v1.AliasList:
							operations.aliasLists.Add(1)
							if operations.failList.Load() == 2 {
								return errors.New("alias snapshot failed")
							}
						}
						return c.List(ctx, list, opts...)
					},
				}).Build()
				back := &listingCountingBackend{
					legacyTestBackend: &legacyTestBackend{client: c, informers: map[schema.GroupVersionKind]cache.SharedIndexInformer{}},
					operations:        operations,
				}
				helper, err := modelaccesspolicy.NewHelper(ctx, back)
				require.NoError(t, err)
				var natives []string
				var grants []types.ModelResource
				for i := range size + 2 {
					native := fmt.Sprintf("deployment-%03d", i)
					if slash {
						native = "team/" + native
					}
					natives = append(natives, native)
					target := "azure/" + native
					if i == size+1 {
						target = "openai/" + native
					}
					model := &v1.Model{Namespace: system.DefaultNamespace, Name: fmt.Sprintf("m1-%03d", i), Spec: v1.ModelSpec{Manifest: types.ModelManifest{
						ModelProvider: system.LLMGatewayModelProvider,
						TargetModel:   target,
						Active:        true,
						Usage:         types.ModelUsageLLM,
					}}}
					require.NoError(t, c.Create(ctx, model))
					gvk, err := c.GroupVersionKindFor(model)
					require.NoError(t, err)
					require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
					if i != size {
						grants = append(grants, types.ModelResource{ID: model.Name})
					}
				}
				policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
					Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
					Models:   grants,
				}}}
				gvk, err := c.GroupVersionKindFor(policy)
				require.NoError(t, err)
				require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
				store, err := llmcoreclient.New(c, legacyTestCodec{})
				require.NoError(t, err)
				engine := llmengine.New()
				require.NoError(t, engine.Start(ctx, llmengine.Options{Store: store, DataDir: t.TempDir()}))
				t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					t.Error("listing configured Azure deployments must not contact upstream")
					w.WriteHeader(http.StatusInternalServerError)
				}))
				t.Cleanup(up.Close)
				_, err = engine.CreateProvider(ctx, []byte(`{"provider":"azure","network_config":{"allow_private_network":true}}`))
				require.NoError(t, err)
				key, err := json.Marshal(map[string]any{
					"name":             "test",
					"value":            "secret",
					"models":           natives,
					"azure_key_config": map[string]any{"endpoint": up.URL},
				})
				require.NoError(t, err)
				_, err = engine.CreateKey(ctx, "azure", key)
				require.NoError(t, err)
				_, err = engine.Sync(ctx)
				require.NoError(t, err)
				mux := &legacyTestMux{mux: http.NewServeMux(), storage: c, gateway: newUsageTestClient(t), user: &kuser.DefaultInfo{UID: "7"}}
				s := &Server{llmGateway: engine, mapHelper: helper}
				s.addLLMProxyRoutes(mux)
				for _, anthropic := range []bool{false, true} {
					t.Run(fmt.Sprintf("anthropic=%v", anthropic), func(t *testing.T) {
						operations.pointReads.Store(0)
						operations.modelLists.Store(0)
						operations.aliasLists.Store(0)
						operations.enumerations.Store(0)
						operations.indexReads.Store(0)
						req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/azure/v1/models", nil)
						if anthropic {
							req.Header.Set("anthropic-version", "2023-06-01")
						}
						rec := httptest.NewRecorder()
						mux.mux.ServeHTTP(rec, req)
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						require.ElementsMatch(t, natives[:size], modelDataIDs(rec.Body.String()))
						t.Logf("rows=%d point_reads=%d model_lists=%d alias_lists=%d enumerations=%d index_reads=%d", size, operations.pointReads.Load(), operations.modelLists.Load(), operations.aliasLists.Load(), operations.enumerations.Load(), operations.indexReads.Load())
						assert.Zero(t, operations.pointReads.Load(), "ordinary names must not resolve through storage")
						assert.LessOrEqual(t, operations.modelLists.Load(), int64(1))
						assert.LessOrEqual(t, operations.aliasLists.Load(), int64(1))
						assert.LessOrEqual(t, operations.enumerations.Load(), int64(4), "at most two grant snapshots")
						assert.LessOrEqual(t, operations.indexReads.Load(), int64(7), "guardrail profiles, target grants and record grants must stay bounded")
					})
				}
				if size == 1 && !slash {
					for _, fault := range []struct {
						name      string
						list      int64
						indexRead int64
					}{
						{
							name: "model snapshot failure",
							list: 1,
						},
						{
							name: "alias snapshot failure",
							list: 2,
						},
						{
							name:      "grant snapshot failure",
							indexRead: 6, // After guardrail profiles and allowed-target evaluation.
						},
					} {
						for _, anthropic := range []bool{false, true} {
							t.Run(fmt.Sprintf("%s/anthropic=%v", fault.name, anthropic), func(t *testing.T) {
								operations.indexReads.Store(0)
								operations.failList.Store(fault.list)
								operations.failIndexRead.Store(fault.indexRead)
								defer operations.failList.Store(0)
								defer operations.failIndexRead.Store(0)
								req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/azure/v1/models", nil)
								if anthropic {
									req.Header.Set("anthropic-version", "2023-06-01")
								}
								rec := httptest.NewRecorder()
								mux.mux.ServeHTTP(rec, req)
								require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
								require.Contains(t, rec.Body.String(), "failed to prepare gateway response")
								require.Empty(t, modelDataIDs(rec.Body.String()))
								// An unauthorized response never initializes the collision/grant snapshot.
								mux.user = &kuser.DefaultInfo{UID: "8"}
								defer func() { mux.user = &kuser.DefaultInfo{UID: "7"} }()
								operations.indexReads.Store(0)
								operations.modelLists.Store(0)
								operations.aliasLists.Store(0)
								rec = httptest.NewRecorder()
								mux.mux.ServeHTTP(rec, req)
								require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
								require.Empty(t, modelDataIDs(rec.Body.String()))
								require.Zero(t, operations.modelLists.Load())
								require.Zero(t, operations.aliasLists.Load())
							})
						}
					}
				}
			})
		}
	}
}
