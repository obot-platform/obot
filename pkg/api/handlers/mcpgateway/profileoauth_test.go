package mcpgateway

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/mmmcp"
	mmmcpcomponent "github.com/obot-platform/mmmcp/component"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestAgentGrantFailuresAreReportedAsInvalid checks that a component that refuses an agent whose connection used its
// profile's grant is reported as the grant being invalid, in the profile that holds it, rather than as needing OAuth;
// and that the gateway refuses a component whose grant broke with a bare 401 at the component hop, which the
// composite answers with that 409, and with the 409 itself anywhere else. Neither carries a challenge.
func TestAgentGrantFailuresAreReportedAsInvalid(t *testing.T) {
	cfg := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{
			{ComponentID: "granted", Name: "ms1granted", URL: "http://localhost/mcp-connect/msi1granted", OAuthGrant: &mcp.OAuthGrantRef{ComponentID: "granted", ProfileID: "vp1agents"}},
			{ComponentID: "ungranted", Name: "ms1ungranted", URL: "http://localhost/mcp-connect/msi1ungranted"},
		},
	}
	response, err := agentOAuthRequiredResponse(cfg, []*mmmcpcomponent.AuthorizationError{
		{URL: "http://localhost/mcp-connect/msi1granted", StatusCode: http.StatusUnauthorized},
		{URL: "http://localhost/mcp-connect/msi1ungranted", StatusCode: http.StatusUnauthorized},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, response.status)
	require.Empty(t, response.headers.Values("WWW-Authenticate"))
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error: vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{
			{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid, ProfileID: "vp1agents"},
			{ComponentID: "ungranted", Code: types.AgentReadinessOAuthRequired},
		},
	}, readAgentFailure(t, bytes.NewReader(response.body)))

	componentHop := api.Context{User: agentLoopbackCaller("vmcpi1agent")}
	response = agentOAuthGrantInvalidResponse(componentHop, "granted", "vp1agents", "")
	require.Equal(t, http.StatusUnauthorized, response.status)
	require.Empty(t, response.headers.Values("WWW-Authenticate"))
	require.NotEmpty(t, mmmcpcomponent.AuthorizationChallenges(&http.Response{StatusCode: response.status, Header: response.headers}), "the composite must read the refusal as the component's authorization failure")

	response = agentOAuthGrantInvalidResponse(api.Context{User: agentKeyCaller()}, "granted", "vp1agents", "")
	require.Equal(t, http.StatusConflict, response.status)
	require.Empty(t, response.headers.Values("WWW-Authenticate"))
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid, ProfileID: "vp1agents"}},
	}, readAgentFailure(t, bytes.NewReader(response.body)))

	// Through the composite: the component's bare 401 becomes the agent's 409, naming the grant as invalid.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "MCP server requires authentication", http.StatusUnauthorized)
	}))
	defer upstream.Close()
	gateway, db := newHeadlessTestGatewayClient(t)
	storageClient := newMCPProxyTestStorage()
	manager, err := mcp.NewSessionManager(t.Context(), false, false, mcp.NewGlobalTokenStore(gateway), nil, "https://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storageClient, storageClient, storageClient, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{AuthorizationResponse: compositeAuthorizationResponse(manager)})
	require.NoError(t, err)
	defer composite.Close()
	compositeCfg := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{{
			ComponentID: "granted",
			Name:        "ms1granted",
			DisplayName: "granted",
			URL:         upstream.URL,
			OAuthGrant:  &mcp.OAuthGrantRef{ComponentID: "granted", ProfileID: "vp1agents"},
		}},
	}
	request := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect-composite/vmcpi1agent", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	request.SetPathValue("mcp_id", "vmcpi1agent")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Method", "tools/list")
	request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	recorder := httptest.NewRecorder()
	req := api.Context{
		Request:        request,
		ResponseWriter: recorder,
		APIBaseURL:     "https://obot.example/api",
		Storage:        storageClient,
		GatewayClient:  gateway,
		User:           agentLoopbackCaller("vmcpi1agent"),
	}
	ctx := context.WithValue(request.Context(), compositeAPIContextKey{}, req)
	ctx = context.WithValue(ctx, upstreamCaptureConfigKey{}, compositeCfg)
	ctx = mmmcp.ContextWithConfig(ctx, mcp.MMMCPConfig(compositeCfg, nil))
	composite.HTTPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid, ProfileID: "vp1agents"}},
	}, readAgentFailure(t, bytes.NewReader(recorder.Body.Bytes())))
	requireNoOAuthState(t, db)
}

// TestComponentHopUsesTheProfileGrant proxies an agent's composite loopback to its connection to a shared component:
// the upstream receives the token of the grant the agent's profile holds, never one of the agent's own; a grant that
// broke refuses the connection with a bare 401 before anything reaches the upstream; and a refresh that fails for a
// reason that may pass is a 503 that leaves the grant valid.
func TestComponentHopUsesTheProfileGrant(t *testing.T) {
	var (
		lock           sync.Mutex
		authorizations []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only the proxied MCP request counts: something on the test host probes listening ports.
		if r.Method != http.MethodPost {
			return
		}
		lock.Lock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		lock.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer upstream.Close()
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider outage", http.StatusServiceUnavailable)
	}))
	defer tokenEndpoint.Close()

	vmcp := &v1.VMCP{Name: "vmcp1hop", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{
			ID:   "one",
			Name: "one",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: upstream.URL},
			}},
		}},
		Profiles: []types.VMCPProfile{{ID: "vp1agents", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}},
	}}}
	shared := &v1.MCPServer{Name: "ms1hop", Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{
		VMCPID:          vmcp.Name,
		VMCPComponentID: "one",
		Manifest:        types.MCPServerManifest{Name: "one", Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteRuntimeConfig{URL: upstream.URL}},
	}}
	instance := &v1.VMCPInstance{Name: "vmcpi1hop", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{UserID: "agent:7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	connection := &v1.MCPServerInstance{Name: "msi1hop", Namespace: system.DefaultNamespace, Spec: v1.MCPServerInstanceSpec{
		UserID:          "agent:7",
		VMCPInstanceID:  instance.Name,
		VMCPComponentID: "one",
		MCPServerName:   shared.Name,
	}}
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPClientSession{}, &v1.MCPServer{}).
		WithObjects(vmcp, shared, instance, connection).
		Build())
	gateway, _ := newHeadlessTestGatewayClient(t)
	tokens := mcp.NewGlobalTokenStore(gateway)
	// The runtime's cluster, where the gateway removes any deployment of a remote server before reaching it.
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion, appsv1.SchemeGroupVersion})
	for _, kind := range []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind("Secret"),
		corev1.SchemeGroupVersion.WithKind("Service"),
		corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"),
		appsv1.SchemeGroupVersion.WithKind("Deployment"),
	} {
		mapper.Add(kind, meta.RESTScopeNamespace)
	}
	cluster := fake.NewClientBuilder().WithScheme(k8sscheme.Scheme).WithRESTMapper(mapper).Build()
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "https://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, cluster, cluster, storageClient, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	grants := vmcpaccess.NewProfileOAuthGrantStore(gateway)
	manager.SetProfileOAuthGrants(grants)
	handler := &Handler{ctx: t.Context(), mcpSessionManager: manager, globalTokenStore: tokens, auditLogCollector: new(recordingProxyAuditCollector)}

	owner, mcpID := principal.ProfileGrant(vmcp.Name, "vp1agents"), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one")
	conf := &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: tokenEndpoint.URL, AuthStyle: oauth2.AuthStyleInParams}}
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, upstream.URL, "grant-1", "1", conf, &oauth2.Token{AccessToken: "profile-grant-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))
	// The agent's own token for its connection, which it must never use.
	require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "agent:7", connection.Name, upstream.URL, "", conf, &oauth2.Token{AccessToken: "agent-own-token"}))

	proxy := func() *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "http://localhost:8080/mcp-connect/"+connection.Name, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
		request.SetPathValue("mcp_id", connection.Name)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		require.NoError(t, handler.Proxy(api.Context{
			Request:        request,
			ResponseWriter: recorder,
			Storage:        storageClient,
			GatewayClient:  gateway,
			APIBaseURL:     "https://obot.example/api",
			User:           agentLoopbackCaller(instance.Name),
		}))
		return recorder
	}

	recorder := proxy()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	lock.Lock()
	require.Equal(t, []string{"Bearer profile-grant-token"}, authorizations)
	authorizations = nil
	lock.Unlock()

	// An expired grant whose refresh fails for a reason that may pass: 503, and the grant stays valid.
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, upstream.URL, "grant-2", "1", conf, &oauth2.Token{AccessToken: "expired", RefreshToken: "refresh", TokenType: "Bearer", Expiry: time.Now().Add(-time.Hour)}))
	recorder = proxy()
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	stored, err := gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, upstream.URL)
	require.NoError(t, err)
	require.Nil(t, stored.InvalidatedAt)

	// A grant that broke: a bare 401, without a challenge, and nothing reaches the upstream.
	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), owner, mcpID, func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-2", "refused")
		return err
	}))
	recorder = proxy()
	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	lock.Lock()
	require.Empty(t, authorizations)
	lock.Unlock()

	// Without any grant, the agent reaches the upstream with no token: never with its own.
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{owner}))
	recorder = proxy()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	lock.Lock()
	require.Equal(t, []string{""}, authorizations)
	lock.Unlock()
}
