package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type legacyTestRoutes map[string]api.HandlerFunc

func TestNormalizeLegacyProxyPath(t *testing.T) {
	for _, tc := range []struct {
		provider string
		path     string
		want     string
	}{
		{
			provider: "openai",
			path:     "chat/completions",
			want:     "v1/chat/completions",
		},
		{
			provider: "anthropic",
			path:     "/v1/messages/",
			want:     "v1/messages",
		},
		{
			provider: "aws-bedrock",
			path:     "anthropic/v1/messages",
			want:     "v1/messages",
		},
		{
			provider: "aws-bedrock-api-key",
			path:     "openai/chat/completions",
			want:     "v1/chat/completions",
		},
		{
			provider: "azure",
			path:     "openai/v1/models",
			want:     "v1/models",
		},
		{
			provider: "azure-entra",
			path:     "responses",
			want:     "v1/responses",
		},
		{
			provider: "generic-responses",
			path:     "v1/responses",
			want:     "v1/responses",
		},
	} {
		t.Run(tc.provider, func(t *testing.T) { require.Equal(t, tc.want, normalizeLegacyProxyPath(tc.provider, tc.path)) })
	}
}

func (r legacyTestRoutes) HandleFunc(pattern string, h api.HandlerFunc) { r[pattern] = h }

func TestLegacyRouteUsesSharedProxy(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
		t.Run(endpoint, func(t *testing.T) {
			fixture := &directFixture{}
			proxy, req := directProxy(t, fixture)
			routes := legacyTestRoutes{}
			(&Server{}).addLegacyLLMProxyRoutes(routes, proxy, newAnthropicProxy())
			require.Len(t, routes, len(legacyProxySources)+4)
			req.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/openai/"+endpoint+"?compat=kept", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"hi"}],"input":"hi","max_tokens":8}`))
			req.SetPathValue("path", endpoint)
			req.Request.Header.Set("Authorization", "Bearer caller-secret")
			req.Request.Header.Set("X-Bf-Key", "untrusted")
			w := httptest.NewRecorder()
			req.ResponseWriter = w
			require.NoError(t, routes["/api/llm-proxy/openai/{path...}"](req))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, "kept", fixture.query["compat"])
			require.Empty(t, fixture.headers["authorization"])
			require.Empty(t, fixture.headers["x-bf-key"])
		})
	}
}

func TestLegacyNativeModelSkipsUnscopedAliasReads(t *testing.T) {
	for _, tc := range []struct {
		name       string
		source     string
		aliasReads int
	}{
		{
			name:       "legacy native name",
			source:     system.OpenAIModelProvider,
			aliasReads: 0,
		},
		{
			name:       "unified native name",
			aliasReads: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy, req := directProxy(t, &directFixture{})
			reads := 0
			req.Storage = interceptor.NewClient(req.Storage.(kclient.WithWatch), interceptor.Funcs{
				Get: func(ctx context.Context, client kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
					if _, ok := obj.(*v1.Alias); ok {
						reads++
					}
					return client.Get(ctx, key, obj, opts...)
				},
			})
			ctx := llmengine.WithLegacySource(t.Context(), tc.source)
			req.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/responses", strings.NewReader(`{"model":"test","input":"hi"}`)).WithContext(ctx)
			req.SetPathValue("path", "v1/responses")
			w := httptest.NewRecorder()
			req.ResponseWriter = w
			require.NoError(t, proxy.proxy(req))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, tc.aliasReads, reads)
		})
	}
}
