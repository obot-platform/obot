package mcpgateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type sessionCreateErrorStorage struct{ storage.Client }

// Exercise the real proxy handler with a remote internal system server; no runtime is started.
func newSessionProxyTestHandler(t *testing.T, client storage.Client, upstreamURL string) *Handler {
	t.Helper()
	gateway := newLocalAgentAuditLogTestGatewayClient(t)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion, appsv1.SchemeGroupVersion})
	for _, kind := range []string{"Secret", "Service", "PersistentVolumeClaim"} {
		mapper.Add(corev1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
	}
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), meta.RESTScopeRoot)
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
	kube := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRESTMapper(mapper).Build()
	tokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "https://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, kube, kube, client, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)

	server := &v1.SystemMCPServer{
		Name:      "sms1sessiontest",
		Namespace: system.DefaultNamespace,
		Spec: v1.SystemMCPServerSpec{Manifest: types.SystemMCPServerManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteRuntimeConfig{URL: upstreamURL},
		}},
	}
	require.NoError(t, client.Create(t.Context(), server))

	return &Handler{
		ctx:               t.Context(),
		mcpSessionManager: manager,
		globalTokenStore:  tokens,
		auditLogCollector: new(recordingProxyAuditCollector),
	}
}

func proxySessionTestRequest(t *testing.T, handler *Handler, client storage.Client, uid, method, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	params := `{"name":"write"}`
	if method == "initialize" {
		params = `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}`
	}
	req := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect/sms1sessiontest", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`))
	req.SetPathValue("mcp_id", "sms1sessiontest")
	if sessionID != "" {
		req.Header.Set(mcpSessionHeader, sessionID)
	}
	recorder := httptest.NewRecorder()
	err := handler.Proxy(api.Context{
		Request:        req,
		ResponseWriter: recorder,
		Storage:        client,
		APIBaseURL:     "https://obot.example/api",
		User:           &user.DefaultInfo{UID: uid, Groups: []string{types.GroupAuthenticated, types.GroupCompositeMCP}},
	})
	require.NoError(t, err)

	return recorder
}

func (s sessionCreateErrorStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if _, ok := obj.(*v1.MCPClientSession); ok {
		return errors.New("session storage unavailable")
	}

	return s.Client.Create(ctx, obj, opts...)
}

func TestSessionProxyPreservesSuccessfulResultWhenSessionSaveFails(t *testing.T) {
	var calls atomic.Int32
	result := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"written"}]}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(mcpSessionHeader, "new-session")
		_, _ = io.WriteString(w, result)
	}))
	defer upstream.Close()
	client := sessionCreateErrorStorage{Client: newMCPProxyTestStorage()}
	handler := newSessionProxyTestHandler(t, client, upstream.URL)

	response := proxySessionTestRequest(t, handler, client, "42", "tools/call", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.JSONEq(t, result, response.Body.String())
	require.Equal(t, int32(1), calls.Load())

	response = proxySessionTestRequest(t, handler, client, "42", "tools/call", "new-session")
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Equal(t, int32(1), calls.Load(), "failed registration must not permit unchecked reuse")
}

func TestSessionProxyInitializeDiscardsStaleID(t *testing.T) {
	for _, test := range []struct {
		name         string
		auditEnabled bool
	}{
		{
			name:         "audited",
			auditEnabled: true,
		},
		{
			name: "unaudited",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get(mcpSessionHeader) != "" {
					http.Error(w, "stale ID forwarded", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set(mcpSessionHeader, "fresh-session")
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
			}))
			defer upstream.Close()
			client := newMCPProxyTestStorage()
			handler := newSessionProxyTestHandler(t, client, upstream.URL)
			if !test.auditEnabled {
				handler.auditLogCollector = nil
			}

			response := proxySessionTestRequest(t, handler, client, "42", "initialize", "unknown-session")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, "fresh-session", response.Header().Get(mcpSessionHeader))
			require.Equal(t, int32(1), calls.Load())

			response = proxySessionTestRequest(t, handler, client, "42", "tools/call", "unknown-session")
			require.Equal(t, http.StatusNotFound, response.Code)
			require.Equal(t, int32(1), calls.Load(), "stale tool calls must not reach upstream")

			if test.auditEnabled {
				collector := handler.auditLogCollector.(*recordingProxyAuditCollector)
				require.Empty(t, collector.entries[0].SessionID, "fresh initialization must not be attributed to the stale ID")
			}
		})
	}
}

func TestSessionProxyRecordsRotatedID(t *testing.T) {
	for _, auditEnabled := range []bool{true, false} {
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set(mcpSessionHeader, "rotated-session")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
		}))
		defer upstream.Close()
		client := newMCPProxyTestStorage()
		handler := newSessionProxyTestHandler(t, client, upstream.URL)
		if !auditEnabled {
			handler.auditLogCollector = nil
		}
		spec := v1.MCPClientSessionSpec{MCPServerID: "sms1sessiontest", ConnectionID: "sms1sessiontest", UserID: "42", ClientName: "original-client", ClientVersion: "1"}
		require.NoError(t, saveMCPClientSession(t.Context(), client, &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"original-session"}}}, spec, false))

		response := proxySessionTestRequest(t, handler, client, "42", "tools/call", "original-session")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		session, err := loadMCPClientSession(t.Context(), client, spec, "rotated-session")
		require.NoError(t, err)
		require.Equal(t, "original-client", session.Spec.ClientName)

		response = proxySessionTestRequest(t, handler, client, "42", "tools/call", "rotated-session")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		response = proxySessionTestRequest(t, handler, client, "99", "tools/call", "rotated-session")
		require.Equal(t, http.StatusNotFound, response.Code)
		require.Equal(t, int32(2), calls.Load())
	}
}

func TestSessionProxyAllowsStatelessUpstream(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get(mcpSessionHeader) != "" {
			http.Error(w, "unexpected session ID on stateless upstream", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	}))
	defer upstream.Close()
	client := newMCPProxyTestStorage()
	handler := newSessionProxyTestHandler(t, client, upstream.URL)

	// A sessionless client can make requests without acquiring an ID.
	response := proxySessionTestRequest(t, handler, client, "42", "tools/call", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Empty(t, response.Header().Get(mcpSessionHeader))

	// Audited initialization creates a virtual ID for Obot's own tracking only.
	response = proxySessionTestRequest(t, handler, client, "42", "initialize", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	virtualID := response.Header().Get(mcpSessionHeader)
	require.NotEmpty(t, virtualID)

	response = proxySessionTestRequest(t, handler, client, "42", "tools/call", virtualID)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, int32(3), calls.Load())
}
