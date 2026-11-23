package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/valyala/fasthttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Pin the recording seam to Bifrost v1.12.0 without relaxing production TLS policy.
func recordCodexTransport(t *testing.T, engine *llmengine.Engine, server *httptest.Server) {
	t.Helper()
	provider := engine.Client().GetProviderByKey(schemas.OpenAI).(*openai.OpenAIProvider)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, name := range []string{"client", "streamingClient"} {
		field := reflect.ValueOf(provider).Elem().FieldByName(name)
		client := *(**fasthttp.Client)(unsafe.Pointer(field.UnsafeAddr()))
		client.TLSConfig = &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}
		client.Dial = func(addr string) (net.Conn, error) {
			if addr != "chatgpt.com:443" && addr != "api.openai.com:443" {
				return nil, fmt.Errorf("unexpected test destination %s", addr)
			}
			return net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
		}
	}
}

func TestPublicCodexRecording(t *testing.T) {
	const bearer = "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJmaXh0dXJlIn0.c2ln"
	const catalog = `{"models":[{"slug":"test","model_instructions":{"instructions_template":"keep"},"future":42},{"slug":"forbidden","future":43}],"future_catalog":true}`
	received := make(chan *http.Request, 4)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			if r.URL.Query().Get("client_version") == "unexpected" {
				_, _ = io.WriteString(w, `{"catalog":[{"slug":"forbidden"}]}`)
				return
			}
			_, _ = io.WriteString(w, catalog)
		} else {
			_, _ = io.WriteString(w, `{"id":"resp_fixture","object":"response","model":"test","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
		}
	}))
	defer upstream.Close()
	proxy, req := directProxy(t, &directFixture{})
	store, err := llmcoreclient.New(req.Storage, directCodec{})
	require.NoError(t, err)
	input, output := guardrails.New(req.GatewayClient, nil).Plugins()
	engine := llmengine.New()
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: t.TempDir(), FirstPlugin: input, LastPlugin: output}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	recordCodexTransport(t, engine, upstream)
	setPublicChatGPTSubscriptions(t, req, true)
	_, err = engine.Sync(t.Context())
	require.NoError(t, err)
	_, tokenClient := newTokenRequestTestServer(t)
	tokens, err := persistent.NewTokenService("https://obot.example.com", tokenClient)
	require.NoError(t, err)
	require.NoError(t, tokens.EnsureJWK(t.Context()))
	gateway := &Server{llmGateway: engine, tokenService: tokens}
	proxy.backend = gateway.newBifrostLLMProviderProxy().backend
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "inference without configured keys",
			method: http.MethodPost,
			path:   "v1/responses",
			body:   `{"model":"test","input":"hi"}`,
		},
		{
			name:   "native catalog access filtering",
			method: http.MethodGet,
			path:   "v1/models?client_version=0.162.1&provider=anthropic",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/api/llm-proxy/"+tc.path, strings.NewReader(tc.body))
			r.SetPathValue("path", strings.Split(tc.path, "?")[0])
			r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
			r.Header.Set("Authorization", bearer)
			r.Header.Set("ChatGPT-Account-ID", "workspace-fixture")
			r.Header.Set(HeaderObotAPIKey, "gateway-fixture")
			r.Header.Set("OpenAI-Organization", "caller-organization")
			r.Header.Set("Cookie", "caller-cookie")
			w := httptest.NewRecorder()
			call := req
			call.Request, call.ResponseWriter = r, w
			require.NoError(t, proxy.proxy(call))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			select {
			case sent := <-received:
				require.Equal(t, "chatgpt.com", sent.Host)
				require.Equal(t, bearer, sent.Header.Get("Authorization"))
				require.Equal(t, "workspace-fixture", sent.Header.Get("ChatGPT-Account-ID"))
				require.Empty(t, sent.Header.Get(HeaderObotAPIKey))
				require.Empty(t, sent.Header.Get("OpenAI-Organization"))
				require.Empty(t, sent.Header.Get("Cookie"))
				if tc.method == http.MethodGet {
					require.Equal(t, "/backend-api/codex/models?client_version=0.162.1", sent.URL.RequestURI())
					require.Equal(t, "test", gjson.Get(w.Body.String(), "models.0.slug").String())
					require.Equal(t, int64(1), gjson.Get(w.Body.String(), "models.#").Int())
					require.Equal(t, "keep", gjson.Get(w.Body.String(), "models.0.model_instructions.instructions_template").String())
					require.True(t, gjson.Get(w.Body.String(), "future_catalog").Bool())
				} else {
					require.Equal(t, "/backend-api/codex/responses", sent.URL.Path)
				}
			case <-time.After(time.Second):
				t.Fatal("no upstream request")
			}
		})
	}
	t.Run("unexpected catalog is not forwarded", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/models?client_version=unexpected", nil)
		r.SetPathValue("path", "v1/models")
		r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
		r.Header.Set("Authorization", bearer)
		w := httptest.NewRecorder()
		call := req
		call.Request, call.ResponseWriter = r, w
		require.NoError(t, proxy.proxy(call))
		require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "forbidden")
		require.Len(t, received, 1)
		<-received
	})

	owner, err := tokenClient.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "fixture",
		ProviderUserID:   "fixture",
		Email:            "fixture@example.com",
	}, "", types.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	require.NoError(t, err)
	for _, expires := range []time.Time{time.Now().Add(time.Hour), time.Now().Add(-time.Hour)} {
		_, localToken, err := tokens.NewToken(t.Context(), persistent.TokenContext{
			Audience:   "https://obot.example.com/mcp-connect/fixture",
			UserID:     strconv.FormatUint(uint64(owner.ID), 10),
			UserGroups: []string{types.GroupMCP, types.GroupAuthenticated},
			ExpiresAt:  persistent.NewTime(expires),
		})
		require.NoError(t, err)
		for _, path := range []string{"v1/models", "v1/responses"} {
			r := httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+path, strings.NewReader(`{"model":"test","input":"hi"}`))
			if path == "v1/models" {
				r = httptest.NewRequest(http.MethodGet, "/api/llm-proxy/"+path, nil)
			}
			r.SetPathValue("path", path)
			r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
			r.Header.Set("Authorization", "Bearer "+localToken)
			r.Header.Set(HeaderObotAPIKey, "gateway-fixture")
			call := req
			call.Request, call.ResponseWriter = r, httptest.NewRecorder()
			require.ErrorContains(t, proxy.proxy(call), "Obot tokens cannot be used as ChatGPT subscription credentials")
			require.Empty(t, received, "locally issued tokens must never reach the recording server")
		}
	}

	// Revocation takes effect on the same backend, without cache expiry or reconfiguration.
	setPublicChatGPTSubscriptions(t, req, false)
	r := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/models", nil)
	r.SetPathValue("path", "v1/models")
	r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
	r.Header.Set("Authorization", bearer)
	refused := func() error {
		r := r.Clone(t.Context())
		r.Header.Set("Authorization", bearer)
		req.Request, req.ResponseWriter = r, httptest.NewRecorder()
		return proxy.proxy(req)
	}
	require.Error(t, refused())
	require.Empty(t, received)

	// An unreadable enabled provider also fails closed instead of reusing its last flag.
	setPublicChatGPTSubscriptions(t, req, true)
	var rows v1.LLMGatewayProviderList
	require.NoError(t, req.Storage.List(t.Context(), &rows))
	for i := range rows.Items {
		if rows.Items[i].Spec.ID == "openai" {
			rows.Items[i].Spec.Payload = "unreadable provider"
			require.NoError(t, req.Storage.Update(t.Context(), &rows.Items[i]))
		}
	}
	require.Error(t, refused())
	require.Empty(t, received)
	require.NoError(t, store.DeleteProvider(t.Context(), "openai"))
	require.Error(t, refused(), "a deleted provider cannot admit a new subscription")
	require.Empty(t, received)
}

func TestCodexAttemptsStayOnOpenAI(t *testing.T) {
	attempts := &gatewayAttempts{budgets: map[string][]string{}, subscriptionProvider: "openai"}
	check := attempts.check(nil, nil, api.Context{}, nil)
	for _, target := range []string{"anthropic/test", "custom-openai/test"} {
		err := check(t.Context(), guardrails.Attempt{Model: target, Subscription: true})
		var refused *guardrails.AttemptError
		require.ErrorAs(t, err, &refused)
		require.Equal(t, http.StatusForbidden, refused.Status)
	}
	require.False(t, attempts.paidBySubscription())
}

func TestCodexCatalogRejectsUnfilterableResponse(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "unexpected envelope",
			body: `{"data":[{"id":"forbidden"}]}`,
		},
		{
			name: "models is not an array",
			body: `{"models":{"slug":"forbidden"}}`,
		},
		{
			name: "invalid JSON with a parseable array",
			body: `{"models":[{"slug":"forbidden"}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
			r.Header.Set("Authorization", "Bearer eyJfixture.payload.signature")
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Request:    r,
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}
			require.Error(t, filterModelListResponse(resp, map[string]bool{}, false))
			require.NoError(t, resp.Body.Close())
		})
	}
}

func TestCodexCatalogRequiresOpenAIGrant(t *testing.T) {
	for _, granted := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
		r.Header.Set("Authorization", "Bearer eyJfixture.payload.signature")
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Request:    r,
			Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"test","future":42}],"first_id":"catalog-first","last_id":"catalog-last"}`)),
		}
		require.NoError(t, filterModelListResponse(resp, map[string]bool{"anthropic/test": true, "test": true, "openai/test": granted}, false))
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, granted, gjson.GetBytes(body, "models.0").Exists())
		require.Equal(t, "catalog-first", gjson.GetBytes(body, "first_id").String())
		require.Equal(t, "catalog-last", gjson.GetBytes(body, "last_id").String())
	}
}

func TestPublicCodexNativeModelNamespace(t *testing.T) {
	proxy, req := directProxy(t, &directFixture{})
	b := &directAccessBackend{storage: req.Storage, informers: map[string]cache.SharedIndexInformer{}}
	helper, err := modelaccesspolicy.NewHelper(t.Context(), b)
	require.NoError(t, err)
	proxy.mapHelper = helper

	// Both first-party providers expose the slug; the newer foreign model must
	// not shadow the OpenAI model advertised by the native Codex catalog.
	var model v1.Model
	require.NoError(t, req.Storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "m1-test"}, &model))
	require.NoError(t, b.informers["Model"].GetIndexer().Add(&model))
	foreign := model.DeepCopy()
	foreign.Name = "m1-foreign"
	foreign.Spec.Manifest.TargetModel = "anthropic/test"
	foreign.CreationTimestamp = metav1.NewTime(model.CreationTimestamp.Add(time.Second))
	require.NoError(t, b.informers["Model"].GetIndexer().Add(foreign))
	policy := &v1.ModelAccessPolicy{
		Name:      "profile",
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
			Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
			Models:   []types.ModelResource{{ID: model.Name}, {ID: foreign.Name}},
		}},
	}
	require.NoError(t, b.informers["ModelAccessPolicy"].GetIndexer().Add(policy))
	setPublicChatGPTSubscriptions(t, req, true)
	_, err = proxy.backend.(*bifrostProviderBackend).engine.Sync(t.Context())
	require.NoError(t, err)

	for _, reference := range []string{"test", "openai/test", "m1-test"} {
		t.Run(reference, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":%q,"input":"hi"}`, reference)))
			r.SetPathValue("path", "v1/responses")
			r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
			r.Header.Set("Authorization", "Bearer eyJfixture.payload.signature")
			w := httptest.NewRecorder()
			call := req
			call.Request, call.ResponseWriter = r, w
			require.NoError(t, proxy.proxy(call))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		})
	}
}

func setPublicChatGPTSubscriptions(t *testing.T, req api.Context, enabled bool) {
	t.Helper()
	store, err := llmcoreclient.New(req.Storage, directCodec{})
	require.NoError(t, err)
	require.NoError(t, store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
		p.SubscriptionPassthrough = new(enabled)
		return nil
	}))
}
