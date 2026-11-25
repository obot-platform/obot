package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
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
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestAgentGrantFailuresAreReportedAsInvalid checks that a component that refuses an agent whose connection used the
// vMCP's preconfigured OAuth grant is reported as the grant being invalid rather than as needing OAuth, naming only the
// component, since the grant is the vMCP's; and that the gateway refuses a component whose grant broke with a bare 401
// at the component hop, which the composite answers with that 409, and with the 409 itself anywhere else. Neither
// carries a challenge.
func TestAgentGrantFailuresAreReportedAsInvalid(t *testing.T) {
	cfg := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{
			{ComponentID: "granted", Name: "ms1granted", URL: "http://localhost/mcp-connect/msi1granted", OAuthGrant: &mcp.OAuthGrantRef{ComponentID: "granted", Owner: principal.PreconfiguredOAuthOwner("vmcp1agent"), MCPID: "vmcp1agent/granted"}},
			{ComponentID: "ungranted", Name: "ms1ungranted", URL: "http://localhost/mcp-connect/msi1ungranted"},
		},
	}
	response, err := agentOAuthRequiredResponse(api.Context{Request: httptest.NewRequest(http.MethodPost, "/mcp-connect/vmcp1test", nil), User: agentUserInfo(7)}, cfg, []*mmmcpcomponent.AuthorizationError{
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
	response = preconfiguredOAuthInvalidResponse(componentHop, "granted")
	require.Equal(t, http.StatusUnauthorized, response.status)
	require.Empty(t, response.headers.Values("WWW-Authenticate"))
	require.NotEmpty(t, mmmcpcomponent.AuthorizationChallenges(&http.Response{StatusCode: response.status, Header: response.headers}), "the composite must read the refusal as the component's authorization failure")

	response = preconfiguredOAuthInvalidResponse(api.Context{User: agentKeyCaller()}, "granted")
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
			OAuthGrant:  &mcp.OAuthGrantRef{ComponentID: "granted", Owner: principal.PreconfiguredOAuthOwner("vmcp1agent"), MCPID: "vmcp1agent/granted"},
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
	grants := vmcpaccess.NewPreconfiguredOAuthStore(gateway)
	manager.SetPreconfiguredOAuth(grants)
	handler := &Handler{ctx: t.Context(), mcpSessionManager: manager, globalTokenStore: tokens, auditLogCollector: new(recordingProxyAuditCollector)}

	owner, mcpID := principal.PreconfiguredOAuthOwner(vmcp.Name), vmcpaccess.PreconfiguredOAuthMCPID(vmcp.Name, "one")
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
	invalidateGrant(t, gateway, owner, mcpID, "grant-2", "refused")
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

// userCompositeLoopbackCaller returns the principal of the composite loopback token issued for user uid's connection to
// a vMCP, with which the composite reaches each of the user's component connections.
func userCompositeLoopbackCaller(uid, instance string) user.Info {
	return &user.DefaultInfo{Name: uid, UID: uid, Groups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, Extra: map[string][]string{
		"oauth_connection_id": {instance},
		"oauth_client_id":     {"downstream-client"},
	}}
}

// TestPreconfiguredOAuthIsolationOnTheWire proxies the component hops of two users and an agent on one shared vMCP,
// with two remote components, to upstreams that record the bearer of every request. The vMCP preconfigures OAuth for
// component "pre" only. Every caller has its own token for each of its connections, which an agent must never use.
//
//   - With the grant, every request to "pre", from either user or the agent, carries the grant's token, and every
//     request to "own" carries the calling user's own token, and the agent's none.
//   - Once the grant is revoked, each user's requests to "pre" carry the user's own token again, and the agent's none.
//   - Once a new grant broke, a user's request to "pre" is refused with a bare 401 before anything reaches the
//     upstream, and no challenge is created for the user; "own" still carries the user's own token.
func TestPreconfiguredOAuthIsolationOnTheWire(t *testing.T) {
	var (
		lock    sync.Mutex
		bearers = map[string][]string{}
	)
	upstream := func(name string) *httptest.Server {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only the proxied MCP request counts: something on the test host probes listening ports.
			if r.Method != http.MethodPost {
				return
			}
			lock.Lock()
			bearers[name] = append(bearers[name], r.Header.Get("Authorization"))
			lock.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		}))
		t.Cleanup(server.Close)
		return server
	}
	pre, own := upstream("pre"), upstream("own")
	takeBearers := func() map[string][]string {
		lock.Lock()
		defer lock.Unlock()
		result := bearers
		bearers = map[string][]string{}
		return result
	}

	remote := func(id, url string) types.VMCPComponent {
		return types.VMCPComponent{ID: id, Name: id, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: url},
		}}}
	}
	vmcp := &v1.VMCP{Name: "vmcp1isolation", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{remote("pre", pre.URL), remote("own", own.URL)},
		Profiles: []types.VMCPProfile{
			{ID: "vp1users", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			{ID: "vp1seven", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
		},
	}}}
	servers := map[string]*v1.MCPServer{}
	objects := []kclient.Object{vmcp}
	for component, url := range map[string]string{"pre": pre.URL, "own": own.URL} {
		servers[component] = &v1.MCPServer{Name: "ms1isolation" + component, Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{
			VMCPID:          vmcp.Name,
			VMCPComponentID: component,
			Manifest:        types.MCPServerManifest{Name: component, Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteRuntimeConfig{URL: url}},
		}}
		objects = append(objects, servers[component])
	}

	// The callers, each with an instance of the vMCP and a connection to each component.
	type caller struct {
		owner       string
		loopback    user.Info
		connections map[string]string
	}
	callers := map[string]*caller{}
	for name, owner := range map[string]string{"user 101": "101", "user 102": "102", "agent 7": "agent:7"} {
		instance := &v1.VMCPInstance{Name: "vmcpi1isolation" + strings.ReplaceAll(owner, ":", ""), Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{
			UserID:   owner,
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		}}
		c := &caller{owner: owner, connections: map[string]string{}}
		if name == "agent 7" {
			c.loopback = gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, map[string][]string{
				"oauth_connection_id": {instance.Name},
				"oauth_client_id":     {"downstream-client"},
			})
		} else {
			c.loopback = userCompositeLoopbackCaller(owner, instance.Name)
		}
		objects = append(objects, instance)
		for component, server := range servers {
			connection := &v1.MCPServerInstance{Name: "msi1isolation" + component + strings.ReplaceAll(owner, ":", ""), Namespace: system.DefaultNamespace, Spec: v1.MCPServerInstanceSpec{
				UserID:          owner,
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: component,
				MCPServerName:   server.Name,
			}}
			objects = append(objects, connection)
			c.connections[component] = connection.Name
		}
		callers[name] = c
	}

	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPClientSession{}, &v1.MCPServer{}).
		WithObjects(objects...).
		Build())
	gateway, db := newHeadlessTestGatewayClient(t)
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
	manager.SetPreconfiguredOAuth(vmcpaccess.NewPreconfiguredOAuthStore(gateway))
	handler := &Handler{ctx: t.Context(), mcpSessionManager: manager, globalTokenStore: tokens, auditLogCollector: new(recordingProxyAuditCollector)}

	// Every caller's own token for each of its connections, keyed by itself, as a user's own authorization stores it.
	conf := &oauth2.Config{ClientID: "client"}
	for _, c := range callers {
		for component, connection := range c.connections {
			require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), c.owner, connection, servers[component].Spec.Manifest.RemoteConfig.URL, "", conf,
				&oauth2.Token{AccessToken: "own-" + strings.ReplaceAll(c.owner, ":", "") + "-" + component, TokenType: "Bearer"}))
		}
	}
	grantOwner, grantMCPID := principal.PreconfiguredOAuthOwner(vmcp.Name), vmcpaccess.PreconfiguredOAuthMCPID(vmcp.Name, "pre")
	storeGrant := func(grantID string) {
		t.Helper()
		require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), grantOwner, grantMCPID, pre.URL, grantID, "1", conf,
			&oauth2.Token{AccessToken: "vmcp-grant-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))
	}

	proxy := func(name, component string) *httptest.ResponseRecorder {
		t.Helper()
		c := callers[name]
		connection := c.connections[component]
		request := httptest.NewRequest(http.MethodPost, "http://localhost:8080/mcp-connect/"+connection, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
		request.SetPathValue("mcp_id", connection)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		require.NoError(t, handler.Proxy(api.Context{
			Request:        request,
			ResponseWriter: recorder,
			Storage:        storageClient,
			GatewayClient:  gateway,
			APIBaseURL:     "https://obot.example/api",
			User:           c.loopback,
		}))
		return recorder
	}
	// requireBearers proxies one request of each caller, in a fixed order, to each component, and requires that each
	// upstream received the bearers in want, in that order.
	order := []string{"user 101", "user 102", "agent 7"}
	requireBearers := func(label string, want map[string][]string) {
		t.Helper()
		for _, component := range []string{"pre", "own"} {
			for _, name := range order {
				recorder := proxy(name, component)
				require.Equal(t, http.StatusOK, recorder.Code, "%s: %s to %s: %s", label, name, component, recorder.Body.String())
			}
		}
		require.Equal(t, want, takeBearers(), label)
	}
	ownBearers := []string{"Bearer own-101-own", "Bearer own-102-own", ""}

	storeGrant("grant-1")
	requireBearers("preconfigured", map[string][]string{
		"pre": {"Bearer vmcp-grant-token", "Bearer vmcp-grant-token", "Bearer vmcp-grant-token"},
		"own": ownBearers,
	})

	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwner(t.Context(), grantOwner))
	requireBearers("revoked", map[string][]string{
		"pre": {"Bearer own-101-pre", "Bearer own-102-pre", ""},
		"own": ownBearers,
	})

	storeGrant("grant-2")
	invalidateGrant(t, gateway, grantOwner, grantMCPID, "grant-2", "invalid_grant")
	for _, name := range []string{"user 101", "user 102"} {
		recorder := proxy(name, "pre")
		require.Equal(t, http.StatusUnauthorized, recorder.Code, "%s: %s", name, recorder.Body.String())
		require.Empty(t, recorder.Header().Values("WWW-Authenticate"), name)
		recorder = proxy(name, "own")
		require.Equal(t, http.StatusOK, recorder.Code, "%s: %s", name, recorder.Body.String())
	}
	require.Equal(t, map[string][]string{"own": {"Bearer own-101-own", "Bearer own-102-own"}}, takeBearers())
	requireNoOAuthState(t, db)
}

// TestUsersAreToldAnAdministratorMustAuthorizeAgain checks what a user's vMCP connection answers when a component that
// the vMCP preconfigures OAuth for refuses it, at the composite: whether the component hop found the grant broken (a
// bare 401) or the upstream refused the grant's token (a challenge), the user gets a 409 that names the component and
// says an administrator must authorize it again, never a challenge to authorize it themselves, and no challenge is
// retained. A component without preconfigured OAuth is left to the user's own authorization, as before.
func TestUsersAreToldAnAdministratorMustAuthorizeAgain(t *testing.T) {
	granted := mcp.ComponentServer{
		ComponentID: "granted",
		Name:        "ms1granted",
		DisplayName: "Granted",
		URL:         "http://localhost/mcp-connect/msi1granted",
		OAuthGrant:  &mcp.OAuthGrantRef{ComponentID: "granted", Owner: principal.PreconfiguredOAuthOwner("vmcp1users"), MCPID: "vmcp1users/granted"},
	}
	own := mcp.ComponentServer{ComponentID: "own", Name: "ms1own", DisplayName: "Own", URL: "http://localhost/mcp-connect/msi1own"}
	cfg := mcp.ServerConfig{Runtime: types.RuntimeVMCP, Components: []mcp.ComponentServer{granted, own}}
	readFailure := func(t *testing.T, body []byte) preconfiguredOAuthFailure {
		t.Helper()
		var failure preconfiguredOAuthFailure
		require.NoError(t, json.Unmarshal(body, &failure), string(body))
		return failure
	}

	// Any challenged component with preconfigured OAuth makes it the administrator's, and no challenge is answered.
	response, err := userPreconfiguredOAuthResponse(cfg, []*mmmcpcomponent.AuthorizationError{
		{URL: own.URL, StatusCode: http.StatusUnauthorized},
		{URL: granted.URL, StatusCode: http.StatusUnauthorized},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, response.status)
	require.Empty(t, response.headers.Values("WWW-Authenticate"))
	failure := readFailure(t, response.body)
	require.Equal(t, preconfiguredOAuthInvalidError, failure.Error)
	require.Equal(t, []string{"granted"}, failure.ComponentIDs)
	require.Contains(t, failure.Message, `"Granted"`)
	require.Contains(t, failure.Message, "An administrator must authorize it again")

	// A component without preconfigured OAuth is the user's to authorize, as before.
	response, err = userPreconfiguredOAuthResponse(cfg, []*mmmcpcomponent.AuthorizationError{{URL: own.URL, StatusCode: http.StatusUnauthorized}})
	require.NoError(t, err)
	require.Nil(t, response)
	// A challenge that is not one of the connection's components is refused.
	_, err = userPreconfiguredOAuthResponse(cfg, []*mmmcpcomponent.AuthorizationError{{URL: "http://localhost/mcp-connect/elsewhere"}})
	require.Error(t, err)

	// Outside the component hop, a user is answered with the 409 directly.
	response = preconfiguredOAuthInvalidResponse(api.Context{User: &user.DefaultInfo{UID: "101", Groups: []string{types.GroupAuthenticated}}}, "granted")
	require.Equal(t, http.StatusConflict, response.status)
	require.Equal(t, preconfiguredOAuthInvalidError, readFailure(t, response.body).Error)

	// Through the composite, for a component hop that found the grant broken and for an upstream that refused it.
	for name, refuse := range map[string]http.HandlerFunc{
		"the grant broke": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "MCP server requires authentication", http.StatusUnauthorized)
		},
		"the upstream refused the grant": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", resource_metadata="https://provider.example.com/.well-known/oauth-protected-resource"`)
			http.Error(w, "invalid token", http.StatusUnauthorized)
		},
	} {
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(refuse)
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
			component := granted
			component.URL = upstream.URL
			compositeCfg := mcp.ServerConfig{Runtime: types.RuntimeVMCP, Components: []mcp.ComponentServer{component}}
			request := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect-composite/vmcpi1user", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
			request.SetPathValue("mcp_id", "vmcpi1user")
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
				User:           userCompositeLoopbackCaller("101", "vmcpi1user"),
			}
			ctx := context.WithValue(request.Context(), compositeAPIContextKey{}, req)
			ctx = context.WithValue(ctx, upstreamCaptureConfigKey{}, compositeCfg)
			ctx = mmmcp.ContextWithConfig(ctx, mcp.MMMCPConfig(compositeCfg, nil))
			composite.HTTPHandler().ServeHTTP(recorder, request.WithContext(ctx))
			require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
			require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
			failure := readFailure(t, recorder.Body.Bytes())
			require.Equal(t, preconfiguredOAuthInvalidError, failure.Error)
			require.Equal(t, []string{"granted"}, failure.ComponentIDs)
			requireNoOAuthState(t, db)
		})
	}
}

// invalidateGrant marks the grant grantID of owner for mcpID invalid with reason, as a refused refresh does
// (gatewayclient.MCPOAuthRefresh.Invalidate).
func invalidateGrant(t *testing.T, gateway *gatewayclient.Client, owner, mcpID, grantID, reason string) {
	t.Helper()
	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), owner, mcpID, func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate(grantID, reason)
		return err
	}))
}
