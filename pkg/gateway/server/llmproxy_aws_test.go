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
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestLegacyAWSCurrentProvider(t *testing.T) {
	ctx := t.Context()
	var calls atomic.Int64
	var failUpstream atomic.Int64
	var failStorage atomic.Bool
	up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if strings.Contains(r.URL.Path, "foundation-models") {
				_, _ = io.WriteString(w, `{"modelSummaries":[{"modelId":"anthropic.claude-test","modelName":"Claude","inputModalities":["TEXT"],"outputModalities":["TEXT"],"inferenceTypesSupported":["ON_DEMAND"]}]}`)
			} else {
				_, _ = io.WriteString(w, `{"data":[{"id":"claude-sonnet-4-5","object":"model"}]}`)
			}
			return
		}
		calls.Add(1)
		if status := failUpstream.Load(); status != 0 {
			http.Error(w, `{"message":"upstream unavailable"}`, int(status))
			return
		}
		if strings.Contains(r.URL.Path, "messages") {
			_, _ = io.WriteString(w, `{"id":"msg1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
			return
		}
		_, _ = io.WriteString(w, `{"output":{"message":{"role":"assistant","content":[{"text":"hello"}]}},"stopReason":"end_turn","usage":{"inputTokens":2,"outputTokens":1,"totalTokens":3}}`)
	}))
	t.Cleanup(up.Close)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource()).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if _, ok := obj.(*v1.Model); ok && failStorage.Load() {
				return errors.New("model storage failed")
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	store, err := llmcoreclient.New(c, legacyTestCodec{})
	require.NoError(t, err)
	g := newUsageTestClient(t)
	input, output := guardrails.New(g, nil).Plugins()
	engine := llmengine.New()
	require.NoError(t, engine.Start(ctx, llmengine.Options{Store: store, DataDir: t.TempDir(), FirstPlugin: input, LastPlugin: output}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	// Exercise the real Bedrock adapter against a local runtime, with no live AWS credentials.
	host := strings.TrimPrefix(up.URL, "https://")
	var p llmcoreclient.Provider
	require.NoError(t, json.Unmarshal([]byte(`{"config":{"network_config":{"allow_private_network":true,"insecure_skip_verify":true}},"keys":[{"id":"current-key","name":"current","value":"test-key","weight":1,"models":["*"],"bedrock_key_config":{"region":"us-east-1","endpoints":{"runtime":"`+host+`","control_plane":"`+host+`","mantle":"`+host+`"}}}]}`), &p))
	require.NoError(t, store.CreateProvider(ctx, "bedrock", p))
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	back := &legacyTestBackend{client: c, informers: map[schema.GroupVersionKind]cache.SharedIndexInformer{}}
	helper, err := modelaccesspolicy.NewHelper(ctx, back)
	require.NoError(t, err)
	model := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-bedrock", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "bedrock/anthropic.claude-test", Dialect: string(llmtypes.DialectAnthropicMessages), Active: true, Usage: types.ModelUsageLLM}}}
	require.NoError(t, c.Create(ctx, model))
	gvk, err := c.GroupVersionKindFor(model)
	require.NoError(t, err)
	require.NoError(t, back.informers[gvk].GetIndexer().Add(model))
	policy := &v1.ModelAccessPolicy{Namespace: system.DefaultNamespace, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, Models: []types.ModelResource{{ID: "*"}}}}}
	gvk, err = c.GroupVersionKindFor(policy)
	require.NoError(t, err)
	require.NoError(t, back.informers[gvk].GetIndexer().Add(policy))
	mux := &legacyTestMux{mux: http.NewServeMux(), storage: c, gateway: g, user: &kuser.DefaultInfo{UID: "7"}}
	s := &Server{llmGateway: engine, mapHelper: helper}
	s.addLLMProxyRoutes(mux)
	// Retained v0.27 credentials must not duplicate the intentional v0.28 AWS configuration.
	require.NoError(t, g.UpsertCredential(ctx, gatewaytypes.Credential{Context: system.AmazonBedrockAPIKeyModelProvider, Name: system.AmazonBedrockAPIKeyModelProvider, Secrets: map[string]string{"OBOT_AWS_MODEL_PROVIDER_API_KEY": "old-key"}}))
	results, err := engine.ImportLegacyModelProviders(ctx, c, g)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "blocked", results[0].Status)
	providers, err := engine.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Equal(t, []schemas.ModelProvider{schemas.Bedrock}, providers)
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	snapshot := func() []byte {
		providers := &v1.LLMGatewayProviderList{}
		models := &v1.ModelList{}
		configs := &v1.LLMGatewayConfigList{}
		require.NoError(t, c.List(ctx, providers))
		require.NoError(t, c.List(ctx, models))
		require.NoError(t, c.List(ctx, configs))
		raw, err := json.Marshal([]any{providers.Items, models.Items, configs.Items})
		require.NoError(t, err)
		return raw
	}
	beforeState := snapshot()
	for _, prefix := range []string{"aws-bedrock", "aws-bedrock-api-key"} {
		for _, path := range []string{"anthropic/v1/messages", "openai/v1/chat/completions", "openai/v1/responses"} {
			t.Run(prefix+"/"+path, func(t *testing.T) {
				before := calls.Load()
				rec := httptest.NewRecorder()
				mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+prefix+"/"+path, strings.NewReader(`{"model":"anthropic.claude-test","max_tokens":16,"input":"hi","messages":[{"role":"user","content":"hi"}]}`)))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, before+1, calls.Load())
				require.Equal(t, "bedrock", rec.Header().Get(headerServedProvider))
				require.Contains(t, rec.Body.String(), "hello")
			})
		}
	}

	require.Equal(t, string(beforeState), string(snapshot()), "requests must preserve provider, key, model and settings resources byte-for-byte")
	send := func(path, reference, extra string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		body := `{"model":"` + reference + `","max_tokens":16,"messages":[{"role":"user","content":"hi"}]` + extra + `}`
		mux.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+path, strings.NewReader(body)))
		return rec
	}
	addModel := func(id, target string) {
		m := model.DeepCopy()
		m.Name = id
		m.ResourceVersion = ""
		m.Spec.Manifest.TargetModel = target
		require.NoError(t, c.Create(ctx, m))
		gvk, err := c.GroupVersionKindFor(m)
		require.NoError(t, err)
		require.NoError(t, back.informers[gvk].GetIndexer().Add(m))
	}
	addModel("m1-mantle", "bedrock_mantle/claude-sonnet-4-5")
	addModel("m1-slash", "bedrock/team/family/model")
	require.NoError(t, store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
		settings["legacy-provider-migration"] = json.RawMessage(`{"sources":{},"mappings":{"m1-old-bedrock":"m1-bedrock","m1-old-mantle":"m1-mantle"}}`)
		return nil
	}))
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	for _, reference := range []string{"team/family/model", "m1-old-bedrock", "bedrock/anthropic.claude-test"} {
		rec := send("aws-bedrock/anthropic/v1/messages", reference, "")
		require.Equal(t, 200, rec.Code, rec.Body.String())
		require.Equal(t, "bedrock", rec.Header().Get(headerServedProvider))
	}
	var mantle llmcoreclient.Provider
	require.NoError(t, json.Unmarshal([]byte(`{"config":{"network_config":{"allow_private_network":true,"insecure_skip_verify":true,"max_retries":0}},"keys":[{"id":"mantle-key","name":"mantle","value":"mantle-secret","weight":1,"models":["*"],"bedrock_mantle_key_config":{"region":"us-east-1","endpoints":{"mantle":"`+host+`"}}}]}`), &mantle))
	require.NoError(t, store.CreateProvider(ctx, "bedrock_mantle", mantle))
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	t.Run("both prefer bedrock and retain one scope", func(t *testing.T) {
		before := calls.Load()
		rec := send("aws-bedrock/anthropic/v1/messages", "anthropic.claude-test", "")
		require.Equal(t, 200, rec.Code, rec.Body.String())
		require.Equal(t, "bedrock", rec.Header().Get(headerServedProvider))
		require.Equal(t, before+1, calls.Load())
		for _, reference := range []string{"bedrock_mantle/claude-sonnet-4-5", "m1-mantle", "m1-old-mantle", "unknown/family/model"} {
			before := calls.Load()
			rec := send("aws-bedrock-api-key/anthropic/v1/messages", reference, "")
			require.Contains(t, []int{400, 410}, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "/api/llm-proxy/v1")
			require.Equal(t, before, calls.Load())
		}
		before = calls.Load()
		rec = send("aws-bedrock/anthropic/v1/messages", "anthropic.claude-test", `,"fallbacks":["bedrock_mantle/claude-sonnet-4-5"]`)
		require.Equal(t, 400, rec.Code, rec.Body.String())
		require.Equal(t, before, calls.Load())
		b := &bifrostProviderBackend{engine: engine}
		req := api.Context{Request: httptest.NewRequest(http.MethodPost, "/", nil).WithContext(llmengine.WithLegacyProviderScope(ctx, "bedrock")), Storage: c, GatewayClient: mux.gateway, User: mux.user}
		attempts := &gatewayAttempts{budgets: map[string][]string{}}
		require.Error(t, attempts.checkAttempt(b, s.newBifrostLLMProviderProxy(), req, nil)(ctx, guardrails.Attempt{Model: "bedrock_mantle/claude-sonnet-4-5"}))
		require.Empty(t, attempts.admitted)
	})
	t.Run("authorization and storage failures stay intact", func(t *testing.T) {
		before := calls.Load()
		mux.user = &kuser.DefaultInfo{UID: "8"}
		rec := send("aws-bedrock/openai/v1/chat/completions", "anthropic.claude-test", "")
		require.Equal(t, 403, rec.Code, rec.Body.String())
		require.Equal(t, before, calls.Load())
		mux.user = &kuser.DefaultInfo{UID: "7"}
		failStorage.Store(true)
		rec = send("aws-bedrock/openai/v1/chat/completions", "m1-bedrock", "")
		failStorage.Store(false)
		require.Equal(t, 500, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "model storage failed")
		require.Equal(t, before, calls.Load())
	})
	t.Run("inactive mapped source is denied first", func(t *testing.T) {
		source := model.DeepCopy()
		source.Name = "m1-old-bedrock"
		source.ResourceVersion = ""
		source.Spec.Manifest.Active = false
		require.NoError(t, c.Create(ctx, source))
		before := calls.Load()
		rec := send("aws-bedrock/anthropic/v1/messages", source.Name, "")
		require.Equal(t, 404, rec.Code, rec.Body.String())
		require.Equal(t, before, calls.Load())
	})
	t.Run("listings use selected provider and access", func(t *testing.T) {
		for _, prefix := range []string{"aws-bedrock", "aws-bedrock-api-key"} {
			for _, protocol := range []string{"openai", "anthropic"} {
				for _, user := range []string{"7", "8"} {
					mux.user = &kuser.DefaultInfo{UID: user}
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/"+prefix+"/"+protocol+"/v1/models", nil)
					if protocol == "anthropic" {
						req.Header.Set("anthropic-version", "2023-06-01")
					}
					mux.mux.ServeHTTP(rec, req)
					require.Equal(t, 200, rec.Code, rec.Body.String())
					var out struct {
						Data []struct {
							ID string `json:"id"`
						}
					}
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
					if user == "8" {
						require.Empty(t, out.Data)
					} else {
						require.NotEmpty(t, out.Data, "%s/%s user %s body %s", prefix, protocol, user, rec.Body.String())
						for _, m := range out.Data {
							require.Contains(t, m.ID, "anthropic.claude-test")
							require.NotContains(t, m.ID, "bedrock_mantle/")
						}
					}
				}
			}
		}
		mux.user = &kuser.DefaultInfo{UID: "7"}
	})
	t.Run("budget denial remains 429 without inference", func(t *testing.T) {
		budget := &gatewaytypes.LLMGatewayBudget{Name: "blocked", ProfileID: "policy", Enabled: true, Limits: []gatewaytypes.LLMGatewayBudgetLimit{{Metric: gatewaytypes.LLMGatewayBudgetMetricRequests, Amount: 0, Window: "1h"}}}
		require.NoError(t, mux.gateway.CreateLLMGatewayBudget(ctx, budget))
		defer func() { require.NoError(t, mux.gateway.DeleteLLMGatewayBudget(ctx, budget.ID)) }()
		before := calls.Load()
		rec := send("aws-bedrock/anthropic/v1/messages", "anthropic.claude-test", "")
		require.Equal(t, 429, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), "/api/llm-proxy/v1")
		require.Equal(t, before, calls.Load())
	})
	t.Run("upstream rate limit remains 429", func(t *testing.T) {
		failUpstream.Store(429)
		defer failUpstream.Store(0)
		rec := send("aws-bedrock/anthropic/v1/messages", "anthropic.claude-test", "")
		require.Equal(t, 429, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), "/api/llm-proxy/v1")
	})
	t.Run("no inference fallback to another provider", func(t *testing.T) {
		failUpstream.Store(400)
		defer failUpstream.Store(0)
		before := calls.Load()
		rec := send("aws-bedrock/anthropic/v1/messages", "anthropic.claude-test", "")
		require.Equal(t, 400, rec.Code, rec.Body.String())
		require.Equal(t, before+1, calls.Load())
	})
	// Registration is shared by every subsequent request, even as providers reconcile.
	for _, state := range []struct {
		name   string
		remove string
		create string
		model  string
		want   string
	}{
		{
			name:   "remove standard falls back to Mantle",
			remove: "bedrock",
			model:  "claude-sonnet-4-5",
			want:   "bedrock_mantle",
		},
		{
			name:   "delete Mantle recreate via AWS card",
			remove: "bedrock_mantle",
			create: "bedrock",
			model:  "anthropic.claude-test",
			want:   "bedrock",
		},
	} {
		t.Run(state.name, func(t *testing.T) {
			require.NoError(t, store.DeleteProvider(ctx, state.remove))
			if state.create != "" {
				require.NoError(t, store.CreateProvider(ctx, state.create, p))
			}
			_, err = engine.Sync(ctx)
			require.NoError(t, err)
			before := calls.Load()
			for _, prefix := range []string{"aws-bedrock", "aws-bedrock-api-key"} {
				rec := send(prefix+"/anthropic/v1/messages", state.model, "")
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.Equal(t, state.want, rec.Header().Get(headerServedProvider))
			}
			require.Equal(t, before+2, calls.Load())
		})
	}
	t.Run("concurrent requests during reconciliation and registration", func(t *testing.T) {
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 8 {
					rec := send("aws-bedrock/anthropic/v1/messages", "anthropic.claude-test", "")
					if rec.Code != 200 {
						t.Errorf("request: %d %s", rec.Code, rec.Body.String())
					}
				}
			})
		}
		for range 8 {
			require.NoError(t, store.CreateProvider(ctx, "bedrock_mantle", mantle))
			_, err = engine.Sync(ctx)
			require.NoError(t, err)
			otherMux := &legacyTestMux{mux: http.NewServeMux(), storage: c, gateway: mux.gateway, user: mux.user}
			s.addLLMProxyRoutes(otherMux)
			require.NoError(t, store.DeleteProvider(ctx, "bedrock_mantle"))
			_, err = engine.Sync(ctx)
			require.NoError(t, err)
		}
		wg.Wait()
	})
	require.NoError(t, store.DeleteProvider(ctx, "bedrock"))
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	for _, prefix := range []string{"aws-bedrock", "aws-bedrock-api-key"} {
		rec := send(prefix+"/anthropic/v1/messages", "anthropic.claude-test", "")
		require.Equal(t, 410, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "/api/llm-proxy/v1")
		rec = send(prefix+"/v1/images/generations", "anthropic.claude-test", "")
		require.Equal(t, 410, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "/api/llm-proxy/v1")
	}
	t.Run("unavailable engine stays unavailable", func(t *testing.T) {
		failed := llmengine.New()
		failed.Fail(errors.New("startup failed"))
		for _, engine := range []*llmengine.Engine{nil, llmengine.New(), failed} {
			unavailable := &Server{llmGateway: engine}
			m := &legacyTestMux{mux: http.NewServeMux()}
			unavailable.addLLMProxyRoutes(m)
			rec := httptest.NewRecorder()
			m.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-proxy/aws-bedrock/openai/v1/models", nil))
			require.Equal(t, 503, rec.Code, rec.Body.String())
		}
	})
	t.Run("runtime error does not imply removed service", func(t *testing.T) {
		require.NoError(t, store.CreateProvider(ctx, "broken", llmcoreclient.Provider{}))
		_, err := engine.Sync(ctx)
		require.Error(t, err)
		rec := send("aws-bedrock/anthropic/v1/messages", "anthropic.claude-test", "")
		require.Equal(t, 500, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), "no AWS provider is configured")
	})
}
