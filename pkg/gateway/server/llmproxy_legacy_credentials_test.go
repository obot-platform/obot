package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLegacyRoutesSendSelectedCredentialsUpstream(t *testing.T) {
	for _, mode := range []string{"initial", "provider rule", "provider and key rule", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			type attempt struct{ path, auth string }
			attempts := make(chan attempt, 20)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodPost {
					_, _ = io.WriteString(w, `{"data":[]}`)
					return
				}
				attempts <- attempt{path: r.URL.Path, auth: r.Header.Get("Authorization")}
				if strings.HasPrefix(r.URL.Path, "/model/") {
					if mode == "fallback" {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = io.WriteString(w, `{"message":"try fallback"}`)
						return
					}
					_, _ = io.WriteString(w, `{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2}}`)
					return
				}
				_, _ = io.WriteString(w, `{"id":"resp_1","object":"response","status":"completed","model":"destination","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
			}))
			t.Cleanup(upstream.Close)
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			store, err := llmcoreclient.New(storage, directCodec{})
			require.NoError(t, err)
			config := schemas.ProviderConfig{
				NetworkConfig:            schemas.NetworkConfig{BaseURL: upstream.URL, InsecureSkipVerify: true, AllowPrivateNetwork: true},
				ConcurrencyAndBufferSize: schemas.ConcurrencyAndBufferSize{Concurrency: 2, BufferSize: 10},
			}
			bedrockConfig := &schemas.BedrockKeyConfig{
				Region:    schemas.NewSecretVar("us-east-1"),
				Endpoints: &schemas.BedrockEndpoints{Runtime: schemas.NewSecretVar(strings.TrimPrefix(upstream.URL, "https://"))},
			}
			iamConfig := *bedrockConfig
			iamConfig.AccessKey = *schemas.NewSecretVar("legacy-access-id")
			iamConfig.SecretKey = *schemas.NewSecretVar("legacy-signing-secret")
			require.NoError(t, store.CreateProvider(t.Context(), "bedrock", llmcoreclient.Provider{Config: config, Keys: []schemas.Key{
				{ID: "iam", Name: "imported-" + system.AmazonBedrockModelProvider, Models: schemas.WhiteList{"*"}, Weight: 1, BedrockKeyConfig: &iamConfig},
				{ID: "api", Name: "imported-" + system.AmazonBedrockAPIKeyModelProvider, Models: schemas.WhiteList{"*"}, Weight: 1, Value: *schemas.NewSecretVar("legacy-api-token"), BedrockKeyConfig: bedrockConfig},
			}}))
			require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{Config: config, Keys: []schemas.Key{
				{ID: "destination", Name: "destination", Models: schemas.WhiteList{"*"}, Weight: 1, Value: *schemas.NewSecretVar("destination-token")},
				{ID: "explicit", Name: "explicit", Models: schemas.WhiteList{"*"}, Weight: 0, Value: *schemas.NewSecretVar("explicit-token")},
			}}))
			if strings.Contains(mode, "rule") {
				target := tables.TableRoutingTarget{Provider: new("openai"), Model: new("destination"), Weight: 1}
				if mode == "provider and key rule" {
					target.KeyID = new("explicit")
				}
				require.NoError(t, store.PutRule(t.Context(), tables.TableRoutingRule{ID: "reroute", Name: "reroute", Scope: "global", CelExpression: "true", Targets: []tables.TableRoutingTarget{target}}))
			}
			engine := llmengine.New()
			require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: t.TempDir()}))
			t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
			access := &directAccessBackend{storage: storage, informers: map[string]cache.SharedIndexInformer{}}
			helper, err := modelaccesspolicy.NewHelper(t.Context(), access)
			require.NoError(t, err)
			for id, target := range map[string]string{"m1-bedrock": "bedrock/amazon.nova-test", "m1-openai": "openai/destination"} {
				model := &v1.Model{Name: id, Namespace: system.DefaultNamespace, Spec: v1.ModelSpec{Manifest: types.ModelManifest{Active: true, ModelProvider: system.LLMGatewayModelProvider, TargetModel: target, Usage: types.ModelUsageLLM}}}
				require.NoError(t, access.informers["Model"].GetIndexer().Add(model))
			}
			policy := &v1.ModelAccessPolicy{Name: "profile", Namespace: system.DefaultNamespace, Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, Models: []types.ModelResource{{ID: "m1-bedrock"}, {ID: "m1-openai"}},
			}}}
			require.NoError(t, access.informers["ModelAccessPolicy"].GetIndexer().Add(policy))
			server := &Server{llmGateway: engine, mapHelper: helper}
			proxy := server.newBifrostLLMProviderProxy()
			proxy.modelProvider = &v1.ModelProvider{Name: system.LLMGatewayModelProvider, Namespace: system.DefaultNamespace}
			routes := legacyTestRoutes{}
			server.addLegacyLLMProxyRoutes(routes, proxy, newAnthropicProxy())
			gateway := newUsageTestClient(t)
			for _, prefix := range []string{"aws-bedrock", "aws-bedrock-api-key"} {
				t.Run(prefix, func(t *testing.T) {
					keyID := "iam"
					if prefix == "aws-bedrock-api-key" {
						keyID = "api"
					}
					// The expected imported key has zero pool weight: losing the pin must select the wrong key.
					require.NoError(t, store.MutateProvider(t.Context(), "bedrock", func(p *llmcoreclient.Provider) error {
						for i := range p.Keys {
							p.Keys[i].Weight = 1
							if p.Keys[i].ID == keyID {
								p.Keys[i].Weight = 0
							}
						}
						return nil
					}))
					_, err := engine.Sync(t.Context())
					require.NoError(t, err)
					body := `{"model":"amazon.nova-test","input":"hi"}`
					if mode == "fallback" {
						body = `{"model":"amazon.nova-test","input":"hi","fallbacks":["openai/destination"]}`
					}
					req := httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+prefix+"/v1/responses", strings.NewReader(body))
					req.SetPathValue("path", "v1/responses")
					req.Header.Set("Authorization", "Bearer caller-token")
					req.Header.Set("X-Bf-Key", "caller-key")
					w := httptest.NewRecorder()
					require.NoError(t, routes["/api/llm-proxy/"+prefix+"/{path...}"](api.Context{Request: req, ResponseWriter: w, Storage: storage, GatewayClient: gateway, User: &user.DefaultInfo{UID: "7"}}))
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					require.NotEmpty(t, attempts)
					var recorded []attempt
					for len(attempts) > 0 {
						recorded = append(recorded, <-attempts)
					}
					for _, a := range recorded {
						if strings.HasPrefix(a.path, "/model/") {
							if keyID == "iam" {
								require.Contains(t, a.auth, "AWS4-HMAC-SHA256 Credential=legacy-access-id/")
							} else {
								require.Equal(t, "Bearer legacy-api-token", a.auth)
							}
						} else {
							want := "Bearer destination-token"
							if mode == "provider and key rule" {
								want = "Bearer explicit-token"
							}
							require.Equal(t, want, a.auth)
						}
					}
					if mode == "initial" {
						require.True(t, strings.HasPrefix(recorded[0].path, "/model/"))
					} else {
						require.True(t, strings.HasSuffix(recorded[len(recorded)-1].path, "/responses"))
					}
					if mode == "fallback" {
						require.GreaterOrEqual(t, len(recorded), 2)
						require.True(t, strings.HasPrefix(recorded[0].path, "/model/"))
					}
				})
			}
		})
	}
}
