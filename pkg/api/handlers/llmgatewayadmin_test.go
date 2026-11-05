package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: dir}))
	t.Cleanup(engine.Shutdown)
	return NewLLMGatewayHandler(engine, nil), store
}

func callAdmin(t *testing.T, handler *LLMGatewayHandler, storage kclient.WithWatch, method, path, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	const prefix = "/api/llm-gateway/admin/"
	request := httptest.NewRequest(method, prefix+path, strings.NewReader(body))
	request.SetPathValue("path", strings.TrimPrefix(request.URL.Path, prefix))
	response := httptest.NewRecorder()
	err := handler.Proxy(api.Context{Request: request, ResponseWriter: response, Storage: storage})
	return response, err
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
			method: http.MethodPost,
			path:   "providers/openai",
			code:   http.StatusMethodNotAllowed,
		},
		{
			method: http.MethodPost,
			path:   "providers/openai/keys/" + key.ID,
			code:   http.StatusMethodNotAllowed,
		},
		{
			method: http.MethodPut,
			path:   "providers/openai/keys",
			code:   http.StatusMethodNotAllowed,
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
			code:   http.StatusMethodNotAllowed,
		},
		{
			method: http.MethodPost,
			path:   "proxy-config",
			code:   http.StatusMethodNotAllowed,
		},
		{
			method: http.MethodPut,
			path:   "governance/routing-rules",
			code:   http.StatusMethodNotAllowed,
		},
		{
			method: http.MethodDelete,
			path:   "governance/pricing-overrides",
			code:   http.StatusMethodNotAllowed,
		},
		{
			method: http.MethodGet,
			path:   "governance/pricing-overrides/id",
			code:   http.StatusMethodNotAllowed,
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
	request.SetPathValue("path", "providers")
	response := httptest.NewRecorder()
	require.NoError(t, h.Proxy(api.Context{Request: request, ResponseWriter: response, Storage: storage}))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Equal(t, http.StatusOK, response.Code)
	_, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	var mp v1.ModelProvider
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(llmengine.ModelProviderResource()), &mp))
	require.NotEmpty(t, mp.Annotations[v1.ModelProviderSyncAnnotation], "disconnects must not suppress post-commit model discovery")
}
