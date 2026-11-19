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
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
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
	"k8s.io/apiserver/pkg/authentication/user"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestAgentGrantFailuresAreReportedAsInvalid checks that a component that refuses an agent whose connection used the
// vMCP's agent OAuth grant is reported as the grant being invalid rather than as needing OAuth, naming only the
// component, since the grant is the vMCP's; and that the gateway refuses a component whose grant broke with a bare 401
// at the component hop, which the composite answers with that 409, and with the 409 itself anywhere else. Neither
// carries a challenge.
func TestAgentGrantFailuresAreReportedAsInvalid(t *testing.T) {
	cfg := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{
			{ComponentID: "granted", Name: "ms1granted", URL: "http://localhost/mcp-connect/msi1granted", OAuthGrant: &mcp.OAuthGrantRef{ComponentID: "granted", Owner: principal.AgentOAuthGrantOwner("vmcp1agent"), MCPID: "vmcp1agent/granted"}},
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
			{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid},
			{ComponentID: "ungranted", Code: types.AgentReadinessOAuthRequired},
		},
	}, readAgentFailure(t, bytes.NewReader(response.body)))

	componentHop := api.Context{User: agentLoopbackCaller("vmcpi1agent")}
	response = agentOAuthGrantInvalidResponse(componentHop, "granted")
	require.Equal(t, http.StatusUnauthorized, response.status)
	require.Empty(t, response.headers.Values("WWW-Authenticate"))
	require.NotEmpty(t, mmmcpcomponent.AuthorizationChallenges(&http.Response{StatusCode: response.status, Header: response.headers}), "the composite must read the refusal as the component's authorization failure")

	response = agentOAuthGrantInvalidResponse(api.Context{User: agentKeyCaller()}, "granted")
	require.Equal(t, http.StatusConflict, response.status)
	require.Empty(t, response.headers.Values("WWW-Authenticate"))
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid}},
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
			OAuthGrant:  &mcp.OAuthGrantRef{ComponentID: "granted", Owner: principal.AgentOAuthGrantOwner("vmcp1agent"), MCPID: "vmcp1agent/granted"},
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
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid}},
	}, readAgentFailure(t, bytes.NewReader(recorder.Body.Bytes())))
	requireNoOAuthState(t, db)
}

// TestComponentHopUsesTheVMCPGrant proxies agents' composite loopbacks to their connections to a shared component: the
// upstream receives the token of the vMCP's one grant for the component, the same for every agent a profile grants the
// component, whichever profile does, and never one of an agent's own; an agent whose profiles do not grant the
// component gets no token at all; a grant that broke refuses the connection with a bare 401 before anything reaches
// the upstream; and a refresh that fails for a reason that may pass is a 503 that leaves the grant valid.
func TestComponentHopUsesTheVMCPGrant(t *testing.T) {
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
		Profiles: []types.VMCPProfile{
			{ID: "vp1seven", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			{ID: "vp1nine", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "9"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"one": {}}}},
			// Agent 8 is named, but granted no component.
			{ID: "vp1eight", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{}}},
		},
	}}}
	shared := &v1.MCPServer{Name: "ms1hop", Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{
		VMCPID:          vmcp.Name,
		VMCPComponentID: "one",
		Manifest:        types.MCPServerManifest{Name: "one", Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteRuntimeConfig{URL: upstream.URL}},
	}}
	agentConnection := func(agentID string) (*v1.VMCPInstance, *v1.MCPServerInstance) {
		owner := "agent:" + agentID
		instance := &v1.VMCPInstance{Name: "vmcpi1hop" + agentID, Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
		return instance, &v1.MCPServerInstance{Name: "msi1hop" + agentID, Namespace: system.DefaultNamespace, Spec: v1.MCPServerInstanceSpec{
			UserID:          owner,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: "one",
			MCPServerName:   shared.Name,
		}}
	}
	instance, connection := agentConnection("7")
	nineInstance, nineConnection := agentConnection("9")
	eightInstance, eightConnection := agentConnection("8")
	// loopback is the composite loopback caller of an agent's instance, as agentLoopbackCaller is for agent 7's.
	loopback := func(agentID uint, instance *v1.VMCPInstance) user.Info {
		return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: agentID}, []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, map[string][]string{
			"oauth_connection_id": {instance.Name},
			"oauth_client_id":     {"downstream-client"},
		})
	}
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPClientSession{}, &v1.MCPServer{}).
		WithObjects(vmcp, shared, instance, connection, nineInstance, nineConnection, eightInstance, eightConnection).
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
	grants := vmcpaccess.NewAgentOAuthGrantStore(gateway)
	manager.SetAgentOAuthGrants(grants)
	handler := &Handler{ctx: t.Context(), mcpSessionManager: manager, globalTokenStore: tokens, auditLogCollector: new(recordingProxyAuditCollector)}

	owner, mcpID := principal.AgentOAuthGrantOwner(vmcp.Name), vmcpaccess.AgentOAuthGrantMCPID(vmcp.Name, "one")
	conf := &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: tokenEndpoint.URL, AuthStyle: oauth2.AuthStyleInParams}}
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, upstream.URL, "grant-1", "1", conf, &oauth2.Token{AccessToken: "vmcp-grant-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))
	// The agents' own tokens for their connections, which they must never use.
	for _, own := range []*v1.MCPServerInstance{connection, nineConnection, eightConnection} {
		require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), own.Spec.UserID, own.Name, upstream.URL, "", conf, &oauth2.Token{AccessToken: "agent-own-token"}))
	}

	proxyAs := func(caller user.Info, connection *v1.MCPServerInstance) *httptest.ResponseRecorder {
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
			User:           caller,
		}))
		return recorder
	}
	proxy := func() *httptest.ResponseRecorder {
		t.Helper()
		return proxyAs(agentLoopbackCaller(instance.Name), connection)
	}

	recorder := proxy()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	// Agent 9, granted the component by another profile, uses the same grant; agent 8, granted nothing, uses no token.
	recorder = proxyAs(loopback(9, nineInstance), nineConnection)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	recorder = proxyAs(loopback(8, eightInstance), eightConnection)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	lock.Lock()
	require.Equal(t, []string{"Bearer vmcp-grant-token", "Bearer vmcp-grant-token", ""}, authorizations)
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
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwner(t.Context(), owner))
	recorder = proxy()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	lock.Lock()
	require.Equal(t, []string{""}, authorizations)
	lock.Unlock()
}
