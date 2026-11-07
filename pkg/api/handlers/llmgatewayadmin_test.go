package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type adminTestCodec struct{}

func (adminTestCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func (adminTestCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func newAdminTestHandler(t *testing.T, storage kclient.WithWatch) (*LLMGatewayHandler, *llmcoreclient.Client) {
	t.Helper()
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "")
	store, err := llmcoreclient.New(storage, adminTestCodec{})
	require.NoError(t, err)
	dir := t.TempDir()
	modeldatatest.Write(t, dir)
	engine := llmengine.New()
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{
		Store:            store,
		DataDir:          dir,
		TriggerModelSync: func(ctx context.Context) error { return llmengine.TriggerModelSync(ctx, storage) },
	}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	return NewLLMGatewayHandler(engine, nil), store
}

func callAdmin(t *testing.T, handler *LLMGatewayHandler, storage kclient.WithWatch, method, path, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	request := httptest.NewRequest(method, gatewayAdminPrefix+path, strings.NewReader(body))
	return callAdminRequest(t, handler, storage, request)
}

func newAdminTestServer(t *testing.T, handler *LLMGatewayHandler, storage kclient.WithWatch, gateway *gatewayclient.Client) *server.Server {
	t.Helper()
	logger, err := audit.New(t.Context(), audit.Options{AuditLogsMode: audit.ModeOff})
	require.NoError(t, err)
	limiter, err := ratelimiter.New(ratelimiter.Options{AuthenticatedRateLimit: 100, UnauthenticatedRateLimit: 100})
	require.NoError(t, err)
	// Admin permissions exercise the real route wrappers; the anonymous marker skips activity writes.
	principal := &user.DefaultInfo{UID: "7", Groups: []string{types.GroupAdmin, authz.UnauthenticatedGroup}}
	authentication := authn.NewAuthenticator(authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
		return &authenticator.Response{User: principal}, true, nil
	}))
	mux := server.NewServer(storage, gateway, nil, system.DefaultNamespace, authentication,
		authz.NewAuthorizer(gateway, storage, storage, false, nil, nil, nil), nil, logger, limiter, "http://obot.test", nil, &license.Provider{})
	handler.RegisterAdminRoutes(mux)
	mux.HTTPHandle("/api/", http.NotFoundHandler())
	return mux
}

func callAdminRequest(t *testing.T, handler *LLMGatewayHandler, storage kclient.WithWatch, request *http.Request) (*httptest.ResponseRecorder, error) {
	t.Helper()
	response := httptest.NewRecorder()
	newAdminTestServer(t, handler, storage, nil).ServeHTTP(response, request)
	if response.Code >= http.StatusBadRequest {
		return response, types.NewErrHTTP(response.Code, response.Body.String())
	}
	return response, nil
}

func TestGatewayAdminUnsupportedOperationsCannotMutate(t *testing.T) {
	storage := newFakeStorage(t, llmengine.ModelProviderResource())
	h, store := newAdminTestHandler(t, storage)
	_, err := h.engine.CreateProvider(t.Context(), json.RawMessage(`{"provider":"openai"}`))
	require.NoError(t, err)
	key, err := h.engine.CreateKey(t.Context(), "openai", json.RawMessage(`{"name":"primary","value":"secret"}`))
	require.NoError(t, err)
	before, err := store.Load(t.Context())
	require.NoError(t, err)

	for _, tc := range []struct {
		method string
		path   string
		code   int
	}{
		{
			method: http.MethodPut,
			path:   "governance/virtual-keys",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "plugins/obot-guardrails-input",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "feature-flags/warp",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "cache/clear",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "mcp/clients",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "providers/%2eopenai",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "providers/openai%2Fextra",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "providers/openai%5Cextra",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPost,
			path:   "providers/openai",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPost,
			path:   "providers/openai/keys/" + key.ID,
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "providers/openai/keys",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "providers/openai/keys/" + key.ID + "/extra",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodGet,
			path:   "config/extra",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodDelete,
			path:   "config",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPost,
			path:   "proxy-config",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPut,
			path:   "governance/routing-rules",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodDelete,
			path:   "governance/pricing-overrides",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodGet,
			path:   "governance/pricing-overrides/id",
			code:   http.StatusNotFound,
		},
		{
			method: http.MethodPost,
			path:   "pricing/force-sync/extra",
			code:   http.StatusNotFound,
		},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			_, err := callAdmin(t, h, storage, tc.method, tc.path, `{"network_config":{"base_url":"http://169.254.169.254"}}`)
			httpErr, ok := errors.AsType[*types.ErrHTTP](err)
			require.True(t, ok, "%v", err)
			require.Equal(t, tc.code, httpErr.Code)
			after, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, before, after, "unsupported operation must not write")
		})
	}
}

func TestGatewayAdminEnvelopesAndDurableFollowup(t *testing.T) {
	storage := newFakeStorage(t, llmengine.ModelProviderResource())
	h, store := newAdminTestHandler(t, storage)
	response, err := callAdmin(t, h, storage, http.MethodPost, "providers", `{"provider":"openai"}`)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/json", response.Header().Get("Content-Type"))
	var created map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	require.Equal(t, "openai", created["name"])
	var mp v1.ModelProvider
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(llmengine.ModelProviderResource()), &mp))
	require.NotEmpty(t, mp.Annotations[v1.ModelProviderSyncAnnotation], "durable writes must still trigger model discovery")
	providers, err := h.engine.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Empty(t, providers, "the HTTP write must leave runtime apply to reconciliation")

	response, err = callAdmin(t, h, storage, http.MethodGet, "providers", "")
	require.NoError(t, err)
	require.Contains(t, response.Body.String(), `"providers":[`)
	ruleResponse, err := callAdmin(t, h, storage, http.MethodPost, "governance/routing-rules", `{"name":"test","cel_expression":"true","targets":[{"provider":"openai","model":"gpt-4o-mini","weight":1}]}`)
	require.NoError(t, err)
	var rule struct {
		Rule struct {
			ID string `json:"id"`
		} `json:"rule"`
	}
	require.NoError(t, json.Unmarshal(ruleResponse.Body.Bytes(), &rule))
	require.NotEmpty(t, rule.Rule.ID)
	for _, path := range []string{"providers/openai", "governance/routing-rules/" + rule.Rule.ID} {
		response, err := callAdmin(t, h, storage, http.MethodDelete, path, "")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "application/json", response.Header().Get("Content-Type"))
		require.JSONEq(t, `{"status":"deleted"}`, response.Body.String())
	}
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Empty(t, loaded.Config.Providers)
	require.Empty(t, loaded.Config.Rules)
}

func TestGatewayAdminStorageErrorStatus(t *testing.T) {
	storage := newFakeStorage(t, llmengine.ModelProviderResource())
	var failure error
	var readFailure error
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if readFailure != nil {
				return readFailure
			}
			return c.List(ctx, list, opts...)
		},
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			if _, ok := obj.(*v1.LLMGatewayProvider); ok && failure != nil {
				return failure
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	h, store := newAdminTestHandler(t, wrapped)
	before, err := store.Load(t.Context())
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{
			name: "conflict",
			err:  apierrors.NewConflict(schema.GroupResource{Resource: "llmgatewayproviders"}, "openai", errors.New("changed")),
			code: http.StatusConflict,
		},
		{
			name: "already exists",
			err:  apierrors.NewAlreadyExists(schema.GroupResource{Resource: "llmgatewayproviders"}, "openai"),
			code: http.StatusConflict,
		},
		{
			name: "missing resource",
			err:  apierrors.NewNotFound(schema.GroupResource{Resource: "llmgatewayproviders"}, "openai"),
			code: http.StatusNotFound,
		},
		{
			name: "storage unavailable",
			err:  errors.New("bifrost storage unavailable"),
			code: http.StatusServiceUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure = tc.err
			_, err := callAdmin(t, h, storage, http.MethodPost, "providers", `{"provider":"openai"}`)
			httpErr, ok := errors.AsType[*types.ErrHTTP](err)
			require.True(t, ok, "%v", err)
			require.Equal(t, tc.code, httpErr.Code)
			require.NotContains(t, httpErr.Message, "bifrost")
			after, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
	// A failed aggregate read remains unavailable even when storage reports NotFound;
	// only a successful read can establish that a particular provider is absent.
	readFailure = apierrors.NewNotFound(schema.GroupResource{Resource: "llmgatewayproviders"}, "openai")
	_, err = callAdmin(t, h, storage, http.MethodGet, "providers", "")
	httpErr, ok := errors.AsType[*types.ErrHTTP](err)
	require.True(t, ok, "%v", err)
	require.Equal(t, http.StatusServiceUnavailable, httpErr.Code)
}

func TestGatewayAdminCanceledWriteStillAnnounces(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	storage := newFakeStorage(t, llmengine.ModelProviderResource())
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			err := c.Create(ctx, obj, opts...)
			if _, ok := obj.(*v1.LLMGatewayProvider); ok && err == nil {
				cancel()
			}
			return err
		},
	})
	h, store := newAdminTestHandler(t, wrapped)
	request := httptest.NewRequest(http.MethodPost, "/api/llm-gateway/admin/providers", strings.NewReader(`{"provider":"openai"}`)).WithContext(ctx)
	response, err := callAdminRequest(t, h, storage, request)
	require.NoError(t, err)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Equal(t, http.StatusOK, response.Code)
	_, err = store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	var mp v1.ModelProvider
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(llmengine.ModelProviderResource()), &mp))
	require.NotEmpty(t, mp.Annotations[v1.ModelProviderSyncAnnotation], "disconnects must not suppress post-commit model discovery")
}

func TestGatewayAdminMuxKeyRoutes(t *testing.T) {
	storage := newFakeStorage(t, llmengine.ModelProviderResource())
	h, store := newAdminTestHandler(t, storage)
	_, err := h.engine.CreateProvider(t.Context(), json.RawMessage(`{"provider":"openai"}`))
	require.NoError(t, err)

	response, err := callAdmin(t, h, storage, http.MethodPost, "providers/openai/keys/", `{"name":"primary","value":"secret"}`)
	require.NoError(t, err)
	var key schemas.Key
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &key))
	require.NotEmpty(t, key.ID)

	path := "providers/openai/keys/" + key.ID
	response, err = callAdmin(t, h, storage, http.MethodPut, path, `{"name":"renamed"}`)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &key))
	require.Equal(t, "renamed", key.Name)

	before, err := store.Load(t.Context())
	require.NoError(t, err)
	_, err = callAdmin(t, h, storage, http.MethodHead, path, "")
	require.NoError(t, err)
	after, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, before, after, "HEAD must remain a read")

	_, err = callAdmin(t, h, storage, http.MethodDelete, path, "")
	require.NoError(t, err)
	provider, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Empty(t, provider.Keys)
}

func TestGatewayAdminNamedHandlerSafeguards(t *testing.T) {
	storage := newFakeStorage(t, llmengine.ModelProviderResource())
	h, store := newAdminTestHandler(t, storage)
	gateway := newHandlerTestGateway(t)
	require.NoError(t, gateway.SetLLMGatewaySettings(t.Context(), gatewaytypes.LLMGatewaySettings{SubscriptionPassthrough: true}))
	for _, provider := range []string{"azure", "anthropic"} {
		_, err := h.engine.CreateProvider(t.Context(), json.RawMessage(`{"provider":"`+provider+`"}`))
		require.NoError(t, err)
	}
	mux := newAdminTestServer(t, h, storage, gateway)
	before, err := store.Load(t.Context())
	require.NoError(t, err)
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{
			method: http.MethodPost,
			path:   "providers",
			body:   `{"provider":"openai","network_config":{"extra_headers":{"Authorization":"env.PRIVATE_KEY"}}}`,
		},
		{
			method: http.MethodPut,
			path:   "providers/azure",
			body:   `{"provider":"openai","network_config":{"insecure_skip_verify":true}}`,
		},
		{
			method: http.MethodPost,
			path:   "providers/azure/keys",
			body:   `{"name":"ambient","azure_key_config":{"endpoint":"https://attacker.example"}}`,
		},
		{
			method: http.MethodPut,
			path:   "providers/anthropic",
			body:   `{"proxy_config":{"skip_tls_verify":true}}`,
		},
		{
			method: http.MethodPut,
			path:   "config",
			body:   `{"client_config":{},"Auth_Config":{"is_enabled":true}}`,
		},
		{
			method: http.MethodPut,
			path:   "proxy-config",
			body:   `{"skip_tls_verify":true}`,
		},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(tc.method, gatewayAdminPrefix+tc.path, strings.NewReader(tc.body)))
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			after, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, before, after, "rejected configuration must not be persisted")
		})
	}

	// Subscription settings must follow a committed provider deletion even after disconnect.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, gatewayAdminPrefix+"providers/anthropic", nil).WithContext(ctx))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	settings, err := gateway.GetLLMGatewaySettings(t.Context())
	require.NoError(t, err)
	require.False(t, settings.SubscriptionPassthrough)
	_, err = store.GetProvider(t.Context(), "anthropic")
	require.True(t, apierrors.IsNotFound(err))
}

func TestGatewayAdminRejectsPartialAzureCredentialClear(t *testing.T) {
	for _, patch := range []string{`{"azure_key_config":{"client_secret":""}}`, `{"azure_key_config":null}`} {
		t.Run(patch, func(t *testing.T) {
			storage := newFakeStorage(t, llmengine.ModelProviderResource())
			h, store := newAdminTestHandler(t, storage)
			_, err := callAdmin(t, h, storage, http.MethodPost, "providers", `{"provider":"azure"}`)
			require.NoError(t, err)
			_, err = callAdmin(t, h, storage, http.MethodPost, "providers/azure/keys", `{
		"name":"owned", "azure_key_config":{
			"endpoint":"https://8.8.8.8", "client_id":"client", "client_secret":"secret", "tenant_id":"tenant"
		}, "aliases":{"friendly":{"model_id":"current","endpoint":"https://8.8.8.8"}}
	}`)
			require.NoError(t, err)
			before, err := store.GetProvider(t.Context(), "azure")
			require.NoError(t, err)
			require.Len(t, before.Keys, 1)
			_, err = callAdmin(t, h, storage, http.MethodPut, "providers/azure/keys/"+before.Keys[0].ID, patch)
			httpErr, ok := errors.AsType[*types.ErrHTTP](err)
			require.True(t, ok, "%v", err)
			require.Equal(t, http.StatusBadRequest, httpErr.Code)
			after, err := store.GetProvider(t.Context(), "azure")
			require.NoError(t, err)
			require.Equal(t, before, after, "reject the merged key without persisting credential changes")
		})
	}
}

func TestGatewayAdminErrorPreservesStatusAndScrubsMessage(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusBadGateway, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			original := &apierrors.StatusError{ErrStatus: metav1.Status{
				Code:    int32(code),
				Message: "Bifrost configuration unavailable",
			}}
			err := gatewayAdminError(fmt.Errorf("admin operation: %w", original))
			httpErr, ok := errors.AsType[*types.ErrHTTP](err)
			require.True(t, ok)
			require.Equal(t, code, httpErr.Code)
			require.Equal(t, "admin operation: LLM gateway configuration unavailable", httpErr.Message)
		})
	}
	httpErr, ok := errors.AsType[*types.ErrHTTP](gatewayAdminError(errors.New("invalid configuration")))
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestGatewayAdminAzureRefreshReturnsPending(t *testing.T) {
	storage := newFakeStorage(t, llmengine.ModelProviderResource())
	handler, store := newAdminTestHandler(t, storage)
	require.NoError(t, store.CreateProvider(t.Context(), "azure", llmcoreclient.Provider{}))
	before, err := store.Load(t.Context())
	require.NoError(t, err)

	response, err := callAdmin(t, handler, storage, http.MethodPost, "providers/azure/refresh-models", `{}`)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, response.Code)
	require.JSONEq(t, `{"status":"pending"}`, response.Body.String())
	var mp v1.ModelProvider
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(llmengine.ModelProviderResource()), &mp))
	require.Equal(t, "true", mp.Annotations[v1.ModelProviderSyncAnnotation], "enqueue model sync exactly once")
	after, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, before.Config, after.Config, "the caller queues work without fetching or publishing discovery")
}
