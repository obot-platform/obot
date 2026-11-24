package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	ssservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	agentFlowVMCP      = "vmcp1agentflow"
	agentFlowComponent = "remote"
	// agentFlowOther is a component that needs no OAuth, the only one agent 10's profile grants.
	agentFlowOther = "other"
	// agentFlowUser is the user, matched by the vMCP's profile for every user, who connects to the vMCP alongside the
	// agents.
	agentFlowUser = "101"
)

// agentFlowProvider is a remote MCP server protected by its own OAuth authorization server, as a real provider is: it
// registers clients dynamically, issues authorization codes bound to a PKCE challenge and redirect URI, exchanges each
// code once for a fresh access and refresh token, rotates refresh tokens on every refresh, and answers invalid_grant
// for a refresh token it no longer honors. Its MCP endpoint serves only requests whose bearer is an access token it
// issued and has not expired, and records the bearer each JSON-RPC request carried.
type agentFlowProvider struct {
	server *httptest.Server

	lock sync.Mutex
	// codes are the outstanding authorization codes, each usable once.
	codes map[string]agentFlowCode
	// access holds the access tokens the MCP endpoint accepts.
	access map[string]bool
	// refresh holds the refresh tokens the token endpoint honors.
	refresh map[string]bool
	issued  int
	// requests records, by JSON-RPC ID, the bearer each MCP request carried.
	requests map[string]string
	// scopes are the scopes each authorization request asked for, in order.
	scopes []string
	// probes are the requests that were not an agent's (whose JSON-RPC IDs are strings), while late is set.
	probes []lateProbe

	// late makes the server let anyone initialize, and only challenge, with scope "tools:read", once tools are
	// listed. It then answers the session requests of a probe without a token.
	late atomic.Bool

	exchanges       atomic.Int32
	refreshes       atomic.Int32
	failedRefreshes atomic.Int32
}

type agentFlowCode struct {
	challenge, redirectURI string
}

// agentFlowFixture is a shared vMCP whose remote component is the provider's MCP server, with a profile for agent 7,
// one for agents 8 and 9, which both grant every component, and one for agent 10, which grants only a component that
// needs no OAuth. Agents 7, 8, 9 and 10 each have a connection to the remote component's shared server; agent 10's is
// one no profile of its grants it. It holds the handlers of the whole path: the administrator's authorization of the
// vMCP's grant, the provider's callback, and the MCP gateway that proxies an agent's requests to the component.
type agentFlowFixture struct {
	provider *agentFlowProvider
	storage  storage.Client
	db       *gorm.DB
	gateway  *gatewayclient.Client
	grants   *vmcpconfig.PreconfiguredOAuthStore
	manager  *mcp.SessionManager
	factory  *MCPOAuthHandlerFactory
	handler  *handler
	proxy    *mcpgateway.Handler
	key      kclient.ObjectKey
	// connections are the agents' connections to the shared component, by agent ID.
	connections map[uint]string
	// instances are the agents' vMCP instances, by agent ID.
	instances map[uint]string
	// userConnection and userInstance are user agentFlowUser's connection to the shared component and vMCP instance.
	userConnection, userInstance string
}

func newAgentFlowProvider(t *testing.T) *agentFlowProvider {
	t.Helper()
	p := &agentFlowProvider{
		codes:    map[string]agentFlowCode{},
		access:   map[string]bool{},
		refresh:  map[string]bool{},
		requests: map[string]string{},
	}
	var base string
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			p.serveMCP(w, r, base)
		case "/metadata":
			writeAgentFlowJSON(w, map[string]any{
				"resource":              base + "/mcp",
				"authorization_servers": []string{base},
				"scopes_supported":      []string{"read"},
			})
		case "/.well-known/oauth-authorization-server":
			writeAgentFlowJSON(w, map[string]any{
				"issuer":                                base,
				"response_types_supported":              []string{"code"},
				"authorization_endpoint":                base + "/authorize",
				"token_endpoint":                        base + "/token",
				"registration_endpoint":                 base + "/register",
				"token_endpoint_auth_methods_supported": []string{"none"},
				"code_challenge_methods_supported":      []string{"S256"},
				"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			})
		case "/register":
			writeAgentFlowJSON(w, map[string]any{"client_id": "registered-client", "token_endpoint_auth_method": "none"})
		case "/authorize":
			p.authorize(w, r)
		case "/token":
			p.token(w, r)
		default:
			// Discovery tries other well-known locations first.
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	base = p.server.URL
	t.Cleanup(p.server.Close)
	return p
}

func writeAgentFlowJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (p *agentFlowProvider) mcpURL() string {
	return p.server.URL + "/mcp"
}

// serveMCP answers an MCP request: with a challenge when it has no valid access token, and otherwise with an empty
// result for its JSON-RPC ID, after recording the bearer it carried.
func (p *agentFlowProvider) serveMCP(w http.ResponseWriter, r *http.Request, base string) {
	late := p.late.Load()
	if r.Method != http.MethodPost && (!late || r.Method != http.MethodDelete) {
		// Something on the test host may probe listening ports.
		return
	}
	body, _ := io.ReadAll(r.Body)
	var message struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &message)
	bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

	p.lock.Lock()
	valid := bearer != "" && p.access[bearer]
	if bearer != "" {
		p.requests[fmt.Sprint(message.ID)] = bearer
	}
	if _, agents := message.ID.(string); late && !agents {
		method := message.Method
		if method == "" {
			method = r.Method
		}
		p.probes = append(p.probes, lateProbe{method: method, header: r.Header.Clone()})
	}
	p.lock.Unlock()

	switch {
	case late && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
		return
	case late && message.Method == "initialize":
		w.Header().Set("Mcp-Session-Id", "late-session")
		writeAgentFlowJSON(w, map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "late", "version": "test"},
		}})
		return
	case late && message.Method == "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
		return
	case !valid && late:
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base+`/metadata", scope="tools:read"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	case !valid:
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base+`/metadata"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeAgentFlowJSON(w, map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": map[string]any{"tools": []any{}}})
}

// takeProbes returns the probes the server received while late was set, and forgets them.
func (p *agentFlowProvider) takeProbes() []lateProbe {
	p.lock.Lock()
	defer p.lock.Unlock()
	probes := p.probes
	p.probes = nil
	return probes
}

// lastScope returns the scope the last authorization request asked for.
func (p *agentFlowProvider) lastScope() string {
	p.lock.Lock()
	defer p.lock.Unlock()
	if len(p.scopes) == 0 {
		return ""
	}
	return p.scopes[len(p.scopes)-1]
}

// authorize is the provider's authorization endpoint. The administrator has consented: it redirects to the client's
// redirect URI with a new code, bound to the PKCE challenge and redirect URI of this request.
func (p *agentFlowProvider) authorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	redirectURI := query.Get("redirect_uri")
	if query.Get("response_type") != "code" || query.Get("client_id") != "registered-client" || redirectURI == "" || query.Get("state") == "" ||
		query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	p.lock.Lock()
	p.issued++
	code := fmt.Sprintf("code-%d", p.issued)
	p.codes[code] = agentFlowCode{challenge: query.Get("code_challenge"), redirectURI: redirectURI}
	p.scopes = append(p.scopes, query.Get("scope"))
	p.lock.Unlock()

	target, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "invalid redirect URI", http.StatusBadRequest)
		return
	}
	values := target.Query()
	values.Set("code", code)
	values.Set("state", query.Get("state"))
	target.RawQuery = values.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// token is the provider's token endpoint: a code is exchanged once, with the verifier of its challenge and its
// redirect URI; a refresh token is honored once and replaced.
func (p *agentFlowProvider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	invalidGrant := func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		code, ok := p.codes[r.Form.Get("code")]
		delete(p.codes, r.Form.Get("code"))
		verifier := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(verifier[:]) != code.challenge || r.Form.Get("redirect_uri") != code.redirectURI {
			invalidGrant()
			return
		}
		p.exchanges.Add(1)
	case "refresh_token":
		if !p.refresh[r.Form.Get("refresh_token")] {
			p.failedRefreshes.Add(1)
			invalidGrant()
			return
		}
		// Rotated: the refresh token just used is never honored again.
		delete(p.refresh, r.Form.Get("refresh_token"))
		p.refreshes.Add(1)
	default:
		http.Error(w, "unsupported grant type", http.StatusBadRequest)
		return
	}
	p.issued++
	access, refresh := fmt.Sprintf("access-%d", p.issued), fmt.Sprintf("refresh-%d", p.issued)
	p.access[access] = true
	p.refresh[refresh] = true
	writeAgentFlowJSON(w, map[string]any{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"scope":         "read",
	})
}

// expire makes the provider refuse an access token, as it does once the token's lifetime ends.
func (p *agentFlowProvider) expire(access string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	delete(p.access, access)
}

// revoke makes the provider refuse a refresh token, as it does when the user revokes the application's access.
func (p *agentFlowProvider) revoke(refresh string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	delete(p.refresh, refresh)
}

// bearers returns the bearer that each recorded MCP request whose JSON-RPC ID has prefix carried, and forgets them.
func (p *agentFlowProvider) bearers(prefix string) map[string]string {
	p.lock.Lock()
	defer p.lock.Unlock()
	result := map[string]string{}
	for id, bearer := range p.requests {
		if strings.HasPrefix(id, prefix) {
			result[id] = bearer
			delete(p.requests, id)
		}
	}
	return result
}

func newAgentFlowFixture(t *testing.T) *agentFlowFixture {
	t.Helper()
	provider := newAgentFlowProvider(t)
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	vmcp := &v1.VMCP{Name: agentFlowVMCP, Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{
			{
				ID:                      agentFlowComponent,
				Name:                    "Remote",
				MCPServerCatalogEntryID: "entry",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: provider.mcpURL()},
				}},
			},
			{
				ID:                      agentFlowOther,
				Name:                    "Other",
				MCPServerCatalogEntryID: "entry-other",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:   types.RuntimeNPX,
					NPXConfig: &types.NPXRuntimeConfig{Package: "other"},
				}},
			},
		},
		Profiles: []types.VMCPProfile{
			profile("vp1seven", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			{
				ID:          "vp1shared",
				Name:        "shared",
				Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}, {Type: types.SubjectTypeAgent, ID: "9"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			},
			{
				ID:          "vp1ten",
				Name:        "ten",
				Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "10"}},
				Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{agentFlowOther: {}}},
			},
			// Every user, never an agent.
			profile("vp1users", types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
		},
	}}}
	// The component's one shared server, which every agent's connection reaches.
	shared := &v1.MCPServer{Name: "ms1agentflow", Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{
		VMCPID:          vmcp.Name,
		VMCPComponentID: agentFlowComponent,
		Manifest:        types.MCPServerManifest{Name: "Remote", Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteRuntimeConfig{URL: provider.mcpURL()}},
	}}
	objects := []kclient.Object{vmcp, shared}
	connections, instances := map[uint]string{}, map[uint]string{}
	for _, id := range []uint{7, 8, 9, 10} {
		caller := agentFlowKeyCaller(id)
		instance := &v1.VMCPInstance{Name: fmt.Sprintf("vmcpi1agentflow%d", id), Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{
			UserID:   caller.GetUID(),
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		}}
		// What the instance controller records once it has synchronized the instance.
		instance.Status.ConfigurationCheckHash = vmcpconfig.ConfigurationCheckHash(vmcpconfig.EnabledComponents(caller, *vmcp, vmcpconfig.ComponentsForInstance(*vmcp, *instance)), "")
		connection := &v1.MCPServerInstance{Name: fmt.Sprintf("msi1agentflow%d", id), Namespace: system.DefaultNamespace, Spec: v1.MCPServerInstanceSpec{
			UserID:          caller.GetUID(),
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: agentFlowComponent,
			MCPServerName:   shared.Name,
		}}
		objects = append(objects, instance, connection)
		connections[id], instances[id] = connection.Name, instance.Name
	}
	userInstance := &v1.VMCPInstance{Name: "vmcpi1agentflowuser", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{
		UserID:   agentFlowUser,
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
	}}
	userInstance.Status.ConfigurationCheckHash = vmcpconfig.ConfigurationCheckHash(vmcpconfig.EnabledComponents(agentFlowUserCaller(), *vmcp, vmcpconfig.ComponentsForInstance(*vmcp, *userInstance)), "")
	userConnection := &v1.MCPServerInstance{Name: "msi1agentflowuser", Namespace: system.DefaultNamespace, Spec: v1.MCPServerInstanceSpec{
		UserID:          agentFlowUser,
		VMCPInstanceID:  userInstance.Name,
		VMCPComponentID: agentFlowComponent,
		MCPServerName:   shared.Name,
	}}
	objects = append(objects, userInstance, userConnection)
	storageClient := storage.Client(clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		WithStatusSubresource(&v1.VMCP{}, &v1.MCPServer{}, &v1.MCPClientSession{}).
		WithObjects(objects...).
		Build())

	services, err := ssservices.New(ssservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })

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
	cluster := clientfake.NewClientBuilder().WithScheme(k8sscheme.Scheme).WithRESTMapper(mapper).Build()

	tokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "http://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, cluster, cluster, storageClient, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	grants := vmcpconfig.NewPreconfiguredOAuthStore(gateway)
	grants.SetComponentTargets(manager)
	manager.SetPreconfiguredOAuth(grants)
	factory := NewMCPOAuthHandlerFactory("http://obot.example", manager, storageClient, gateway, tokens, "", true)
	factory.SetPreconfiguredOAuth(grants)
	proxy, err := mcpgateway.NewHandler(t.Context(), manager, tokens, nil, nil, "http://obot.example", "", "", nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proxy.Close() })

	return &agentFlowFixture{
		provider:       provider,
		storage:        storageClient,
		db:             services.DB.DB,
		gateway:        gateway,
		grants:         grants,
		manager:        manager,
		factory:        factory,
		handler:        &handler{baseURL: "http://obot.example", oauthChecker: factory},
		proxy:          proxy,
		key:            kclient.ObjectKeyFromObject(vmcp),
		connections:    connections,
		instances:      instances,
		userConnection: userConnection.Name,
		userInstance:   userInstance.Name,
	}
}

// agentFlowUserCaller returns the principal of agentFlowUser, as the API authenticates them.
func agentFlowUserCaller() kuser.Info {
	return &kuser.DefaultInfo{Name: agentFlowUser, UID: agentFlowUser, Groups: []string{types.GroupAuthenticated}}
}

// agentFlowKeyCaller returns the principal of an agent's API key, as the outer vMCP gateway authenticates it.
func agentFlowKeyCaller(id uint) kuser.Info {
	return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: id}, []string{types.GroupMCP, types.GroupAuthenticated}, map[string][]string{
		"authorized_mcp_ids": {"*"},
	})
}

// agentFlowLoopbackCaller returns the principal of the composite loopback token that the outer gateway issues for an
// agent's vMCP connection, with which the composite reaches each of the agent's component connections.
func agentFlowLoopbackCaller(id uint, instance string) kuser.Info {
	return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: id}, []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, map[string][]string{
		"oauth_connection_id": {instance},
	})
}

// authorize runs an administrator's authorization of the vMCP's grant for the remote component end to end: the start
// handler returns the provider's authorization URL, the administrator's browser follows it to the provider, which
// redirects back to Obot's MCP OAuth callback with a code, and the callback, as the same administrator, exchanges it.
// It returns the access and refresh tokens the provider issued, as stored for the vMCP.
func (f *agentFlowFixture) authorize(t *testing.T) (string, string) {
	t.Helper()
	admin := administrator("1")
	start := api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps/"+f.key.Name+"/components/"+agentFlowComponent+"/preconfigured-oauth", nil),
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		User:           admin,
	}
	authorization, err := f.factory.StartPreconfiguredOAuth(start, f.key.Name, agentFlowComponent)
	require.NoError(t, err)
	require.False(t, authorization.LocalhostCallback)

	// The administrator's browser, at the provider.
	browser := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := browser.Get(authorization.AuthURL)
	require.NoError(t, err)
	_ = response.Body.Close()
	require.Equal(t, http.StatusFound, response.StatusCode)
	callbackURL, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, system.MCPOAuthCallbackURL("http://obot.example"), callbackURL.Scheme+"://"+callbackURL.Host+callbackURL.Path)

	// The provider's redirect, back at Obot, in the same administrator's session.
	recorder := httptest.NewRecorder()
	callback := api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?"+callbackURL.RawQuery, nil),
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		User:           admin,
	}
	require.NoError(t, f.handler.oauthCallback(callback))
	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oauth/complete", recorder.Header().Get("Location"))

	stored := f.stored(t)
	require.Equal(t, "1", stored.AuthorizedBy)
	require.Nil(t, stored.InvalidatedAt)
	return stored.AccessToken, stored.RefreshToken
}

// stored returns the vMCP's grant for the remote component, as stored.
func (f *agentFlowFixture) stored(t *testing.T) *gatewaytypes.MCPOAuthToken {
	t.Helper()
	stored, err := f.gateway.GetMCPOAuthToken(t.Context(), principal.PreconfiguredOAuthOwner(f.key.Name), vmcpconfig.PreconfiguredOAuthMCPID(f.key.Name, agentFlowComponent), f.provider.mcpURL())
	require.NoError(t, err)
	return stored
}

// recordProbe records in the vMCP's status that the controller's probe of the remote component found it asks for
// OAuth, as the controller does in the background.
func (f *agentFlowFixture) recordProbe(t *testing.T) {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	component := vmcpconfig.ComponentsForInstance(vmcp, v1.VMCPInstance{})[0]
	vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{agentFlowComponent: {
		TargetHash: vmcpconfig.ComponentOAuthTargetHash(vmcp, component),
		Required:   v1.VMCPComponentOAuthRequired,
	}}
	require.NoError(t, f.storage.Status().Update(t.Context(), &vmcp))
}

// oauth returns the remote component's preconfigured OAuth state, as an administrator reads it.
func (f *agentFlowFixture) oauth(t *testing.T) types.VMCPPreconfiguredOAuth {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	status, err := f.grants.Status(t.Context(), vmcp, agentFlowComponent)
	require.NoError(t, err)
	return status
}

// requireConnectRefused requires that agent id's connect is refused with a 409 with reason code for the remote
// component, never a challenge, as its readiness says.
func (f *agentFlowFixture) requireConnectRefused(t *testing.T, id uint, code string) {
	t.Helper()
	want := vmcpconfig.AgentReadinessFailure{
		Error:   vmcpconfig.AgentOAuthRequiredError,
		Reasons: []vmcpconfig.ReadinessReason{{ComponentID: agentFlowComponent, Code: code}},
	}
	require.Equal(t, want.Reasons, f.ready(t, id).Reasons, "agent %d", id)
	recorder := f.connectVMCP(t, id)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	var failure vmcpconfig.AgentReadinessFailure
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &failure))
	require.Equal(t, want, failure)
	require.NotContains(t, recorder.Body.String(), "profileID")
}

// callComponent proxies one MCP request of agent id to its connection to the component, as the composite does for the
// agent through the agent's composite loopback token, with the JSON-RPC ID requestID.
func (f *agentFlowFixture) callComponent(t *testing.T, id uint, requestID string) *httptest.ResponseRecorder {
	t.Helper()
	connection := f.connections[id]
	request := httptest.NewRequest(http.MethodPost, "http://localhost:8080/mcp-connect/"+connection,
		strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"method":"tools/list","params":{}}`, requestID)))
	request.SetPathValue("mcp_id", connection)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	recorder := httptest.NewRecorder()
	require.NoError(t, f.proxy.Proxy(api.Context{
		Request:        request,
		ResponseWriter: recorder,
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		APIBaseURL:     "http://obot.example/api",
		User:           agentFlowLoopbackCaller(id, f.instances[id]),
	}))
	return recorder
}

// connectVMCP connects agent id to the vMCP through the outer gateway with its API key. It is only called where the
// agent is refused before any MCP exchange: an agent that is ready would go on to the composite loopback, which this
// fixture does not serve.
func (f *agentFlowFixture) connectVMCP(t *testing.T, id uint) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://obot.example/mcp-connect/"+f.key.Name,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	request.SetPathValue("mcp_id", f.key.Name)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	require.NoError(t, f.proxy.Proxy(api.Context{
		Request:        request,
		ResponseWriter: recorder,
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		APIBaseURL:     "http://obot.example/api",
		User:           agentFlowKeyCaller(id),
	}))
	return recorder
}

// ready reports whether agent id is ready to use the vMCP, as its connect decides.
func (f *agentFlowFixture) ready(t *testing.T, id uint) vmcpconfig.Readiness {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	report, err := f.manager.AgentReadiness(t.Context(), vmcp, nil, agentFlowKeyCaller(id))
	require.NoError(t, err)
	return report.Readiness
}

// expireStored ages the vMCP's stored access token past its expiry, as time does.
func (f *agentFlowFixture) expireStored(t *testing.T) {
	t.Helper()
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthToken{}).
		Where("user_id = ? AND mcp_id = ?", principal.PreconfiguredOAuthOwner(f.key.Name), vmcpconfig.PreconfiguredOAuthMCPID(f.key.Name, agentFlowComponent)).
		Update("expiry", time.Now().Add(-time.Hour)).Error)
}

func (f *agentFlowFixture) requireNoChallenges(t *testing.T) {
	t.Helper()
	var challenges, pending int64
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthChallenge{}).Count(&challenges).Error)
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Count(&pending).Error)
	require.Zero(t, challenges, "an MCP OAuth challenge was retained")
	require.Zero(t, pending, "an MCP OAuth authorization was left pending")
}

// requireUpstream requires that the upstream received exactly the requests with the given IDs, each with bearer.
func requireUpstream(t *testing.T, p *agentFlowProvider, prefix, bearer string, ids ...string) {
	t.Helper()
	want := map[string]string{}
	for _, id := range ids {
		want[id] = bearer
	}
	require.Equal(t, want, p.bearers(prefix))
}

// TestAgentsUseTheVMCPGrantAnAdministratorAuthorized runs the whole life of a vMCP's preconfigured OAuth grant against
// a real OAuth provider: an administrator authorizes the remote component once, through the provider's authorization
// endpoint and Obot's callback, and the MCP gateway then proxies the requests of every agent granted the component to
// it with the one grant. It checks, on the wire to the upstream:
//
//   - before the component is authorized, the agents granted it are not ready, their connect is refused with
//     oauth_required, and they send no token;
//   - once it is, agents 7, 8 and 9, whose profiles differ, all send the same token, and agent 10, whose profile does
//     not grant the component, never sends it, though it reaches the component's server;
//   - once the grant is revoked, every agent granted the component is oauth_required again and sends no token;
//   - once it is authorized again and its refresh is refused with invalid_grant, every agent is refused the component,
//     without a challenge and without a token reaching the upstream, and its connect is refused with oauth_invalid,
//     which the component's preconfigured OAuth state shows;
//   - an expired grant is refreshed once at the provider, however many agents use it at once, and every agent's
//     request carries the new token.
func TestAgentsUseTheVMCPGrantAnAdministratorAuthorized(t *testing.T) {
	f := newAgentFlowFixture(t)
	f.recordProbe(t)
	granted := []uint{7, 8, 9}

	// Before any authorization, every agent granted the component is not ready, and sends no token of any kind: the
	// upstream's challenge reaches the composite loopback as a bare 401, which the composite reports to the agent as
	// the component needing OAuth.
	for _, id := range granted {
		f.requireConnectRefused(t, id, types.AgentReadinessOAuthRequired)
	}
	require.True(t, f.ready(t, 10).Ready(), "agent 10 is not granted the component, so it does not need it authorized")
	recorder := f.callComponent(t, 7, "none-7-0")
	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	requireUpstream(t, f.provider, "none-", "")
	f.requireNoChallenges(t)

	// (a) One authorization, for the vMCP's component, which every agent granted it uses.
	access, _ := f.authorize(t)
	require.EqualValues(t, 1, f.provider.exchanges.Load())
	f.requireNoChallenges(t)
	for _, id := range granted {
		require.True(t, f.ready(t, id).Ready(), "agent %d", id)
	}
	for i := range 2 {
		for _, id := range granted {
			recorder = f.callComponent(t, id, fmt.Sprintf("a-%d-%d", id, i))
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		}
	}
	requireUpstream(t, f.provider, "a-", access, "a-7-0", "a-7-1", "a-8-0", "a-8-1", "a-9-0", "a-9-1")
	// An agent whose profiles do not grant the component is never given its grant.
	recorder = f.callComponent(t, 10, "a-10-0")
	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "a-10-", "")
	require.Equal(t, types.VMCPPreconfiguredOAuthAuthorized, f.oauth(t).State)

	// (b) An administrator revokes the grant: every agent granted the component needs it authorized again, and its
	// requests reach the upstream with no token, which refuses them.
	_, revoked, err := f.grants.Revoke(t.Context(), f.storage, f.key, agentFlowComponent)
	require.NoError(t, err)
	require.True(t, revoked)
	for _, id := range granted {
		f.requireConnectRefused(t, id, types.AgentReadinessOAuthRequired)
	}
	for _, id := range []uint{7, 8} {
		recorder = f.callComponent(t, id, fmt.Sprintf("b-%d-0", id))
		require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
		require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	}
	requireUpstream(t, f.provider, "b-", "")
	f.requireNoChallenges(t)

	// (c) The component is authorized again, and works.
	access, refresh := f.authorize(t)
	recorder = f.callComponent(t, 8, "c-8-0")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "c-8-", access, "c-8-0")

	// Its access token expires, and the provider no longer honors its refresh token. The next request tries to refresh
	// it once, is answered invalid_grant, and is refused: a bare 401 to the composite loopback, which the composite
	// reports to the agent as the grant being invalid. Nothing reaches the upstream.
	f.provider.expire(access)
	f.provider.revoke(refresh)
	f.expireStored(t)
	for i, id := range []uint{7, 9} {
		recorder = f.callComponent(t, id, fmt.Sprintf("c-broken-%d", i))
		require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
		require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	}
	requireUpstream(t, f.provider, "c-broken-", "")
	// The refresh was tried once: the grant is marked invalid for every agent, and is not refreshed again.
	require.EqualValues(t, 1, f.provider.failedRefreshes.Load())
	stored := f.stored(t)
	require.NotNil(t, stored.InvalidatedAt)
	require.Contains(t, stored.InvalidReason, "invalid_grant")

	// Every agent's connect is refused with a 409 that names the grant as invalid, never a challenge, and the
	// component's preconfigured OAuth state shows it to the administrator.
	for _, id := range granted {
		f.requireConnectRefused(t, id, types.AgentReadinessOAuthInvalid)
	}
	f.requireNoChallenges(t)
	oauth := f.oauth(t)
	require.Equal(t, types.VMCPPreconfiguredOAuthInvalid, oauth.State)
	require.Contains(t, oauth.InvalidReason, "invalid_grant")

	// (d) The component is authorized again, and its access token expires. Agents 7, 8 and 9 use it at once: the grant
	// is refreshed once at the provider, and every request reaches the upstream with the new token.
	access, _ = f.authorize(t)
	f.provider.expire(access)
	f.expireStored(t)
	var (
		wait  sync.WaitGroup
		start = make(chan struct{})
		lock  sync.Mutex
		codes []int
		ids   []string
	)
	for _, id := range granted {
		for i := range 6 {
			requestID := fmt.Sprintf("d-%d-%d", id, i)
			ids = append(ids, requestID)
			wait.Go(func() {
				<-start
				recorder := f.callComponent(t, id, requestID)
				lock.Lock()
				defer lock.Unlock()
				codes = append(codes, recorder.Code)
			})
		}
	}
	close(start)
	wait.Wait()
	for _, code := range codes {
		require.Equal(t, http.StatusOK, code)
	}
	require.EqualValues(t, 1, f.provider.refreshes.Load(), "the shared grant was refreshed more than once")
	stored = f.stored(t)
	require.NotEqual(t, access, stored.AccessToken)
	require.Nil(t, stored.InvalidatedAt)
	require.Equal(t, "1", stored.AuthorizedBy, "a refresh changed who authorized the grant")
	requireUpstream(t, f.provider, "d-", stored.AccessToken, ids...)

	// The refreshed token keeps working without another refresh, and agent 10 still never gets it.
	recorder = f.callComponent(t, 9, "e-9-0")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "e-9-", stored.AccessToken, "e-9-0")
	require.EqualValues(t, 1, f.provider.refreshes.Load())
	recorder = f.callComponent(t, 10, "e-10-0")
	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "e-10-", "")
	f.requireNoChallenges(t)
}

// TestAgentsUseTheVMCPGrantForALateChallenge runs the life of the grant for a component whose server lets anyone
// initialize, and only challenges once tools are listed, against a real OAuth provider:
//
//   - while the vMCP holds no grant, and no probe has found otherwise, nothing shows that the component needs OAuth, so
//     agents read as ready, and an agent learns otherwise only from the component, as a bare 401 that the composite
//     reports as needing OAuth;
//   - an administrator authorizes the component: the start learns the challenge from tools/list, with
//     unauthenticated probes that end their session, and asks for its scope; the callback stores the grant, and every
//     agent granted the component sends its token to the upstream through the component hop;
//   - the grant shows that the component needs OAuth: its preconfigured OAuth state says so, and once the grant is marked
//     invalid, every agent granted the component is not ready, and its connect is refused with oauth_invalid, as its
//     readiness says.
func TestAgentsUseTheVMCPGrantForALateChallenge(t *testing.T) {
	f := newAgentFlowFixture(t)
	f.provider.late.Store(true)

	for _, id := range []uint{7, 8} {
		require.True(t, f.ready(t, id).Ready(), "agent %d", id)
	}
	require.Equal(t, types.VMCPPreconfiguredOAuthNotRequired, f.oauth(t).State)
	recorder := f.callComponent(t, 8, "late-none-8")
	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	requireUpstream(t, f.provider, "late-none-", "")
	f.requireNoChallenges(t)

	access, _ := f.authorize(t)
	require.Equal(t, "tools:read", f.provider.lastScope())
	requireUnauthenticatedProbes(t, f.provider.takeProbes())
	for _, id := range []uint{7, 8} {
		require.True(t, f.ready(t, id).Ready(), "agent %d", id)
		recorder = f.callComponent(t, id, fmt.Sprintf("late-%d-0", id))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	}
	requireUpstream(t, f.provider, "late-", access, "late-7-0", "late-8-0")
	require.Equal(t, types.VMCPPreconfiguredOAuthAuthorized, f.oauth(t).State)

	// A grant that stopped working still shows that the component needs OAuth.
	grantID := f.stored(t).GrantID
	var marked bool
	require.NoError(t, f.gateway.WithMCPOAuthRefreshLock(t.Context(), principal.PreconfiguredOAuthOwner(f.key.Name), vmcpconfig.PreconfiguredOAuthMCPID(f.key.Name, agentFlowComponent), func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		var err error
		marked, err = refresh.Invalidate(grantID, "the provider refused the refresh token")
		return err
	}))
	require.True(t, marked)
	require.Equal(t, types.VMCPPreconfiguredOAuthInvalid, f.oauth(t).State)
	for _, id := range []uint{7, 8} {
		f.requireConnectRefused(t, id, types.AgentReadinessOAuthInvalid)
	}

	// Authorizing it again probes without a token too, though a grant is stored.
	access, _ = f.authorize(t)
	require.Equal(t, "tools:read", f.provider.lastScope())
	requireUnauthenticatedProbes(t, f.provider.takeProbes())
	require.True(t, f.ready(t, 8).Ready())
	recorder = f.callComponent(t, 8, "late-8-1")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "late-8-", access, "late-8-1")
	f.requireNoChallenges(t)
}

// callUserComponent proxies one MCP request of agentFlowUser to their connection to the component, as the composite
// does for the user through the user's composite loopback token, with the JSON-RPC ID requestID.
func (f *agentFlowFixture) callUserComponent(t *testing.T, requestID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://localhost:8080/mcp-connect/"+f.userConnection,
		strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"method":"tools/list","params":{}}`, requestID)))
	request.SetPathValue("mcp_id", f.userConnection)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	recorder := httptest.NewRecorder()
	require.NoError(t, f.proxy.Proxy(api.Context{
		Request:        request,
		ResponseWriter: recorder,
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		APIBaseURL:     "http://obot.example/api",
		User: &kuser.DefaultInfo{Name: agentFlowUser, UID: agentFlowUser, Groups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, Extra: map[string][]string{
			"oauth_connection_id": {f.userInstance},
		}},
	}))
	return recorder
}

// userOAuthURL returns the authorization URL that agentFlowUser is prompted with for the remote component, as the vMCP
// consent and the OAuth check ask for each component of the user's connection, or "" when the user is not prompted.
func (f *agentFlowFixture) userOAuthURL(t *testing.T) string {
	t.Helper()
	caller := agentFlowUserCaller()
	server, config, err := f.manager.ServerForAction(t.Context(), f.userConnection, caller)
	require.NoError(t, err)
	req := api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodGet, "/api/vmcps/"+f.key.Name+"/oauth-url", nil),
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		User:           caller,
	}
	u, err := f.factory.CheckForMCPAuth(req, server, config, caller.GetUID(), server.Name, "")
	require.NoError(t, err)
	return u
}

// pendingAuthorizations counts the MCP OAuth authorizations that were started and not completed.
func (f *agentFlowFixture) pendingAuthorizations(t *testing.T) int64 {
	t.Helper()
	var pending int64
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Count(&pending).Error)
	return pending
}

// TestUsersUseTheVMCPGrantAnAdministratorAuthorized runs a user's connection to the remote component through the life
// of the vMCP's preconfigured OAuth, against a real OAuth provider, beside the agents that use the same grant:
//
//   - without a grant, the user is prompted to authorize the component, and once they have, their requests carry their
//     own token;
//   - once an administrator authorizes the component, the user is not prompted, and their requests carry the grant's
//     token, the agents', not their own;
//   - once the grant is revoked, the user's requests carry their own token again, which was never deleted;
//   - once a new grant broke, the user is still not prompted and nothing is started for them, and their request is
//     refused before anything reaches the upstream, as the administrator's grant to fix.
func TestUsersUseTheVMCPGrantAnAdministratorAuthorized(t *testing.T) {
	f := newAgentFlowFixture(t)
	f.recordProbe(t)

	// Without a grant, the user is prompted, and a request without their own authorization carries no token.
	require.NotEmpty(t, f.userOAuthURL(t))
	recorder := f.callUserComponent(t, "u-none")
	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "u-none", "")

	// The user's own authorization, which the provider honors.
	f.provider.lock.Lock()
	f.provider.access["own-101"] = true
	f.provider.lock.Unlock()
	require.NoError(t, f.gateway.ReplaceMCPOAuthToken(t.Context(), agentFlowUser, f.userConnection, f.provider.mcpURL(), "", &oauth2.Config{ClientID: "user-client"},
		&oauth2.Token{AccessToken: "own-101", TokenType: "Bearer"}))
	recorder = f.callUserComponent(t, "u-own-0")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "u-own-", "own-101", "u-own-0")

	// An administrator authorizes the component: the user is not prompted, and uses the grant, as agent 7 does.
	access, _ := f.authorize(t)
	pending := f.pendingAuthorizations(t)
	require.Empty(t, f.userOAuthURL(t))
	require.Equal(t, pending, f.pendingAuthorizations(t), "the user's OAuth check started an authorization")
	recorder = f.callUserComponent(t, "u-grant-0")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	recorder = f.callComponent(t, 7, "u-grant-7")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "u-grant-", access, "u-grant-0", "u-grant-7")

	// Revoked: the user's own token is used again.
	_, revoked, err := f.grants.Revoke(t.Context(), f.storage, f.key, agentFlowComponent)
	require.NoError(t, err)
	require.True(t, revoked)
	recorder = f.callUserComponent(t, "u-own-1")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	requireUpstream(t, f.provider, "u-own-", "own-101", "u-own-1")

	// A new grant breaks: its access token expires and the provider refuses its refresh token. The user is not
	// prompted, nothing is started for them, and their request is refused without reaching the upstream or using
	// their own token.
	access, refresh := f.authorize(t)
	f.provider.expire(access)
	f.provider.revoke(refresh)
	f.expireStored(t)
	pending = f.pendingAuthorizations(t)
	recorder = f.callUserComponent(t, "u-broken-0")
	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	requireUpstream(t, f.provider, "u-broken-", "")
	require.NotNil(t, f.stored(t).InvalidatedAt)
	require.Empty(t, f.userOAuthURL(t))
	require.Equal(t, pending, f.pendingAuthorizations(t), "the user's OAuth check started an authorization")
	var challenges int64
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthChallenge{}).Count(&challenges).Error)
	require.Zero(t, challenges, "an MCP OAuth challenge was retained")
}
