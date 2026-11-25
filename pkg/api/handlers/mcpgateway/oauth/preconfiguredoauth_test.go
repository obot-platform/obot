package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	ssservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	agentGrantVMCP      = "vmcp1agentgrants"
	agentGrantRemote    = "remote"
	agentGrantLocal     = "local"
	agentGrantAccess    = "provider-access-token"
	agentGrantRefresh   = "provider-refresh-token"
	agentGrantCodeSent  = "authorization-code"
	agentGrantRefreshed = "provider-refreshed-token"

	// lateChallengeToolsCall is a server that lets anyone initialize and list its tools, asks for OAuth only when a tool
	// is called, and publishes its protected resource metadata at the well-known location for its URL.
	lateChallengeToolsCall = "tools/call"
	// lateChallengeNone is a server that asks for OAuth nowhere and publishes no protected resource metadata.
	lateChallengeNone = "none"
	// lateChallengeHang is a server that lets anyone initialize, never answers tools/list, and publishes its protected
	// resource metadata.
	lateChallengeHang = "hang"
)

// agentGrantProvider is a remote MCP server that requires OAuth, with its own authorization server that registers
// clients dynamically and exchanges codes.
type agentGrantProvider struct {
	server     *httptest.Server
	operations atomic.Int32
	exchanges  atomic.Int32
	refreshes  atomic.Int32
	// redirectURI is the redirect_uri of the last code exchange.
	redirectURI atomic.Value
	// late, when set, is where the server asks for OAuth instead of at initialize (lateChallengeToolsCall,
	// lateChallengeNone, lateChallengeHang), and the server then answers the probes of an authorization's start
	// (serveLate).
	late atomic.Value

	lock sync.Mutex
	// probes are the requests a late-challenge server received at its MCP endpoint.
	probes []lateProbe
}

// lateProbe is one request a late-challenge server received.
type lateProbe struct {
	// method is the JSON-RPC method, or the HTTP method of a request without one.
	method string
	header http.Header
}

// agentGrantFixture is a shared vMCP with a remote component that requires OAuth and a local one, a profile for agent
// 7, one for agent 8 and one for every user, each granting every component, and the handlers that authorize the
// vMCP's preconfigured OAuth grants.
type agentGrantFixture struct {
	provider *agentGrantProvider
	storage  kclient.WithWatch
	// db is the gateway's database, for a test to age a pending authorization.
	db      *gorm.DB
	gateway *gatewayclient.Client
	grants  *vmcpconfig.PreconfiguredOAuthStore
	factory *MCPOAuthHandlerFactory
	handler *handler
	key     kclient.ObjectKey
}

// serveLate answers a request to the MCP endpoint of a server that asks for OAuth late, as mode says, after recording
// it. It hands out session "late-session".
func (p *agentGrantProvider) serveLate(w http.ResponseWriter, r *http.Request, mode string) {
	var body struct {
		Method string `json:"method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	method := body.Method
	if method == "" {
		method = r.Method
	}
	p.lock.Lock()
	p.probes = append(p.probes, lateProbe{method: method, header: r.Header.Clone()})
	p.lock.Unlock()
	switch method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "late-session")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "late", "version": "test"},
		}})
	case "tools/list":
		if mode == lateChallengeHang {
			<-r.Context().Done()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"tools": []any{}}})
	default:
		// The initialized notification, and the DELETE that ends the session.
		w.WriteHeader(http.StatusAccepted)
	}
}

// lateProbes returns the requests a late-challenge server received.
func (p *agentGrantProvider) lateProbes() []lateProbe {
	p.lock.Lock()
	defer p.lock.Unlock()
	return append([]lateProbe(nil), p.probes...)
}

// requireUnauthenticatedProbes requires that every probe of an authorization's start carried no header but those of
// the transport and the MCP session, so neither a configured header nor any token, and that the session it started was
// ended.
func requireUnauthenticatedProbes(t *testing.T, probes []lateProbe) {
	t.Helper()
	allowed := []string{"Accept", "Accept-Encoding", "Content-Length", "Content-Type", "Mcp-Protocol-Version", "Mcp-Session-Id", "User-Agent"}
	var methods []string
	for _, probe := range probes {
		methods = append(methods, probe.method)
		for name := range probe.header {
			require.Contains(t, allowed, name, "%s carried header %s", probe.method, name)
		}
		if probe.method != "initialize" {
			require.Equal(t, "late-session", probe.header.Get("Mcp-Session-Id"), probe.method)
			require.Equal(t, "2025-06-18", probe.header.Get("Mcp-Protocol-Version"), probe.method)
		}
	}
	require.ElementsMatch(t, []string{"initialize", "notifications/initialized", "tools/list", http.MethodDelete}, methods)
}

func newAgentGrantProvider(t *testing.T) *agentGrantProvider {
	t.Helper()
	p := &agentGrantProvider{}
	var base string
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp":
			if mode, _ := p.late.Load().(string); mode != "" {
				p.serveLate(w, r, mode)
				return
			}
			// Only the unauthenticated initialize probe reaches the server; no MCP operation is ever run for an
			// authorization.
			var body struct {
				Method string `json:"method"`
			}
			if r.Header.Get("Authorization") != "" || r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || body.Method != "initialize" {
				p.operations.Add(1)
			}
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base+`/metadata"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/metadata":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"resource":              base + "/mcp",
				"authorization_servers": []string{base},
				"scopes_supported":      []string{"read"},
			}))
		case "/.well-known/oauth-protected-resource/mcp":
			// Only a late-challenge server that has protected resource metadata publishes it here.
			if mode, _ := p.late.Load().(string); mode != lateChallengeToolsCall && mode != lateChallengeHang {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"resource":              base + "/mcp",
				"authorization_servers": []string{base},
				"scopes_supported":      []string{"published"},
			}))
		case "/.well-known/oauth-authorization-server":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                base,
				"response_types_supported":              []string{"code"},
				"authorization_endpoint":                base + "/authorize",
				"token_endpoint":                        base + "/token",
				"registration_endpoint":                 base + "/register",
				"token_endpoint_auth_methods_supported": []string{"none"},
			}))
		case "/register":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"client_id":                  "registered-client",
				"token_endpoint_auth_method": "none",
			}))
		case "/token":
			require.NoError(t, r.ParseForm())
			if r.Form.Get("grant_type") == "refresh_token" {
				// A refresh needs no redirect, so none is sent, and no callback is ever reached.
				require.Equal(t, agentGrantRefresh, r.Form.Get("refresh_token"))
				require.Empty(t, r.Form.Get("redirect_uri"))
				p.refreshes.Add(1)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"access_token":  agentGrantRefreshed,
					"refresh_token": agentGrantRefresh,
					"token_type":    "Bearer",
					"expires_in":    3600,
				}))
				return
			}
			require.Equal(t, agentGrantCodeSent, r.Form.Get("code"))
			p.redirectURI.Store(r.Form.Get("redirect_uri"))
			p.exchanges.Add(1)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":  agentGrantAccess,
				"refresh_token": agentGrantRefresh,
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "read",
			}))
		default:
			// Discovery tries other well-known locations first.
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	base = p.server.URL
	t.Cleanup(p.server.Close)
	return p
}

func (p *agentGrantProvider) mcpURL() string {
	return p.server.URL + "/mcp"
}

func newAgentGrantFixture(t *testing.T) *agentGrantFixture {
	t.Helper()
	provider := newAgentGrantProvider(t)
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	vmcp := &v1.VMCP{Name: agentGrantVMCP, Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{
			{
				ID:                      agentGrantRemote,
				Name:                    "Remote",
				MCPServerCatalogEntryID: "entry",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: provider.mcpURL()},
				}},
			},
			{ID: agentGrantLocal, Name: "Local"},
		},
		Profiles: []types.VMCPProfile{
			profile("vp1seven", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			profile("vp1eight", types.Subject{Type: types.SubjectTypeAgent, ID: "8"}),
			profile("vp1users", types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
		},
	}}}
	storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1.VMCP{}).WithObjects(vmcp).Build()
	services, err := ssservices.New(ssservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })
	tokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "http://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	grants := vmcpconfig.NewPreconfiguredOAuthStore(gateway)
	grants.SetComponentTargets(manager)
	factory := NewMCPOAuthHandlerFactory("http://obot.example", manager, storage, gateway, tokens, "", true)
	factory.SetPreconfiguredOAuth(grants)
	return &agentGrantFixture{
		provider: provider,
		storage:  storage,
		db:       services.DB.DB,
		gateway:  gateway,
		grants:   grants,
		factory:  factory,
		handler:  &handler{baseURL: "http://obot.example", oauthChecker: factory},
		key:      kclient.ObjectKeyFromObject(vmcp),
	}
}

// administrator returns the principal of user id, an administrator unless groups say otherwise.
func administrator(id string, groups ...string) kuser.Info {
	if len(groups) == 0 {
		groups = []string{types.GroupAdmin, types.GroupAuthenticated}
	}
	return principaltest.AsUser(&kuser.DefaultInfo{Name: "admin-" + id, UID: id, Groups: groups})
}

func (f *agentGrantFixture) request(caller kuser.Info, method, target string) (api.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	return api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(method, target, nil),
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		User:           caller,
	}, recorder
}

// update changes the stored vMCP with change.
func (f *agentGrantFixture) update(t *testing.T, change func(*v1.VMCP)) {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	change(&vmcp)
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
}

// deleteVMCP starts the deletion of the vMCP, which its finalizer holds back, as the vMCP controller's does.
func (f *agentGrantFixture) deleteVMCP(t *testing.T) {
	t.Helper()
	f.update(t, func(vmcp *v1.VMCP) { vmcp.Finalizers = append(vmcp.Finalizers, v1.VMCPFinalizer) })
	require.NoError(t, f.storage.Delete(t.Context(), &v1.VMCP{Name: f.key.Name, Namespace: f.key.Namespace}))
}

// start starts an authorization of the vMCP's grant for a component as caller, and returns the state of its pending
// attempt.
func (f *agentGrantFixture) start(t *testing.T, caller kuser.Info, componentID string) (string, error) {
	t.Helper()
	req, _ := f.request(caller, http.MethodPost, "/api/vmcps/"+f.key.Name+"/components/"+componentID+"/preconfigured-oauth")
	authorization, err := f.factory.StartPreconfiguredOAuth(req, f.key.Name, componentID)
	if err != nil {
		return "", err
	}
	require.False(t, authorization.LocalhostCallback)
	parsed, err := url.Parse(authorization.AuthURL)
	require.NoError(t, err)
	require.Equal(t, f.provider.server.URL+"/authorize", parsed.Scheme+"://"+parsed.Host+parsed.Path)
	require.Equal(t, system.MCPOAuthCallbackURL("http://obot.example"), parsed.Query().Get("redirect_uri"))
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)
	return state, nil
}

// callback calls the MCP OAuth callback with state as caller.
func (f *agentGrantFixture) callback(caller kuser.Info, state string) (*httptest.ResponseRecorder, error) {
	query := url.Values{"state": {state}, "code": {agentGrantCodeSent}}
	req, recorder := f.request(caller, http.MethodGet, "/oauth/mcp/callback?"+query.Encode())
	return recorder, f.handler.oauthCallback(req)
}

// storedGrant returns the access token, the administrator and the URL of the vMCP's grant for the remote component,
// and whether it holds one.
func (f *agentGrantFixture) storedGrant(t *testing.T) (string, string, string, bool) {
	t.Helper()
	token, err := f.gateway.GetMCPOAuthToken(t.Context(), principal.PreconfiguredOAuthOwner(f.key.Name), vmcpconfig.PreconfiguredOAuthMCPID(f.key.Name, agentGrantRemote), f.provider.mcpURL())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", "", false
	}
	require.NoError(t, err)
	return token.AccessToken, token.AuthorizedBy, token.URL, true
}

// oauth returns the remote component's preconfigured OAuth state, as its route reports it, and its JSON.
func (f *agentGrantFixture) oauth(t *testing.T) (types.VMCPPreconfiguredOAuth, string) {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	status, err := f.grants.Status(t.Context(), vmcp, agentGrantRemote)
	require.NoError(t, err)
	raw, err := json.Marshal(status)
	require.NoError(t, err)
	return status, string(raw)
}

// readiness reports whether agent id can use the vMCP, as its connect decides, for the vMCP as it is stored, without
// its local component.
func (f *agentGrantFixture) readiness(t *testing.T, id uint) []types.AgentReadinessReason {
	t.Helper()
	var stored v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &stored))
	stored.Spec.Manifest.Components = stored.Spec.Manifest.Components[:1]
	report, err := vmcpconfig.AgentReadiness(t.Context(), f.grants, stored, nil, agentGrantAgent(id))
	require.NoError(t, err)
	return report.Readiness.Reasons
}

func requireErrHTTP(t *testing.T, err error, code int) {
	t.Helper()
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, code, httpErr.Code, httpErr.Message)
}

// requireNoPending requires that no authorization was left pending.
func (f *agentGrantFixture) requireNoPending(t *testing.T) {
	t.Helper()
	var pending int64
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Count(&pending).Error)
	require.Zero(t, pending, "a refused start left a pending authorization")
}

// TestPreconfiguredOAuthAuthorization checks the whole authorization of a vMCP's preconfigured OAuth grant: it is started
// for the vMCP's component, never for the administrator; its callback is refused to anyone but the administrator who
// started it, who must still be one; it stores one grant for the vMCP's component, with who authorized it, which every
// agent granted the component uses; its attempt cannot be used again; and authorizing again replaces the grant.
func TestPreconfiguredOAuthAuthorization(t *testing.T) {
	f := newAgentGrantFixture(t)
	owner, mcpID := principal.PreconfiguredOAuthOwner(f.key.Name), vmcpconfig.PreconfiguredOAuthMCPID(f.key.Name, agentGrantRemote)
	state, err := f.start(t, administrator("1"), agentGrantRemote)
	require.NoError(t, err)
	require.Zero(t, f.provider.operations.Load(), "an authorization ran an MCP operation")

	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, owner, pending.UserID)
	require.Equal(t, mcpID, pending.MCPID)
	require.Equal(t, f.provider.mcpURL(), pending.URL)
	require.Equal(t, "1", pending.GrantInitiatorUserID)
	require.Equal(t, vmcpconfig.PreconfiguredOAuthDefinitionHash(vmcp.Spec.Manifest.Components[0]), pending.GrantDefinitionHash)

	// Another administrator, the initiating user without the administrator role, and an agent, even one the vMCP's
	// profiles name, cannot complete it, and it stays available to its administrator.
	for _, caller := range []kuser.Info{
		administrator("2"),
		administrator("1", types.GroupBasic, types.GroupAuthenticated),
		agentGrantAgent(1),
		agentGrantAgent(7),
	} {
		_, err := f.callback(caller, state)
		requireErrHTTP(t, err, http.StatusForbidden)
	}
	_, err = f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err, "a refused callback consumed the attempt")
	require.Zero(t, f.provider.exchanges.Load())

	recorder, err := f.callback(administrator("1"), state)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oauth/complete", recorder.Header().Get("Location"))
	require.NotContains(t, recorder.Body.String(), agentGrantAccess)
	access, authorizedBy, target, ok := f.storedGrant(t)
	require.True(t, ok)
	require.Equal(t, agentGrantAccess, access)
	require.Equal(t, "1", authorizedBy)
	require.Equal(t, f.provider.mcpURL(), target)
	// The administrator holds no token of their own for it.
	_, err = f.gateway.GetMCPOAuthToken(t.Context(), "1", mcpID, f.provider.mcpURL())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// It is the vMCP's one grant, recorded for the component, and every agent the vMCP's profiles grant the component
	// is ready with it.
	requireOneGrant := func() {
		t.Helper()
		var rows int64
		require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthToken{}).Where("user_id = ?", owner).Count(&rows).Error)
		require.EqualValues(t, 1, rows)
		token, err := f.gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, f.provider.mcpURL())
		require.NoError(t, err)
		var stored v1.VMCP
		require.NoError(t, f.storage.Get(t.Context(), f.key, &stored))
		require.Equal(t, []v1.VMCPPreconfiguredOAuthGrant{{ComponentID: agentGrantRemote, GrantIDs: []string{token.GrantID}}}, stored.Status.PreconfiguredOAuth)
	}
	requireOneGrant()
	for _, id := range []uint{7, 8} {
		require.Empty(t, f.readiness(t, id), "agent %d", id)
	}

	// The component's preconfigured OAuth state names who authorized it and for which target, never its tokens.
	oauth, raw := f.oauth(t)
	require.Equal(t, types.VMCPPreconfiguredOAuthAuthorized, oauth.State)
	require.Equal(t, "1", oauth.AuthorizedBy)
	require.Equal(t, f.provider.mcpURL(), oauth.Target)
	require.NotContains(t, raw, agentGrantAccess)
	require.NotContains(t, raw, agentGrantRefresh)

	// The authorization is recorded once, as the administrator's, against the vMCP, with the component, scopes and
	// hostname of the target, and never the token or the URL. The refused callbacks recorded nothing.
	events, _, err := f.gateway.ListAdminAuditEvents(t.Context(), gatewayclient.AdminAuditEventOptions{})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "1", events[0].ActorUserID)
	require.Equal(t, gatewaytypes.AdminAuditActionVMCPPreconfiguredOAuthAuthorize, events[0].Action)
	require.Equal(t, gatewaytypes.AdminAuditResourceVMCP, events[0].ResourceKind)
	require.Equal(t, f.key.Name, events[0].ResourceID)
	require.Empty(t, events[0].ParentID)
	var details map[string]any
	require.NoError(t, json.Unmarshal(events[0].Details, &details))
	require.Equal(t, agentGrantRemote, details["componentID"])
	require.Equal(t, vmcpconfig.URLHostname(f.provider.mcpURL()), details["hostname"])
	require.Contains(t, details, "scopes")
	require.NotContains(t, string(events[0].Details), agentGrantAccess)
	require.NotContains(t, string(events[0].Details), agentGrantRefresh)
	require.NotContains(t, string(events[0].Details), f.provider.mcpURL())

	// The attempt is used up.
	_, err = f.callback(administrator("1"), state)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.EqualValues(t, 1, f.provider.exchanges.Load())

	// Another administrator authorizes the component again: the new grant replaces the old one, and is still the only
	// one, recorded alone.
	state, err = f.start(t, administrator("2"), agentGrantRemote)
	require.NoError(t, err)
	_, err = f.callback(administrator("2"), state)
	require.NoError(t, err)
	_, authorizedBy, _, ok = f.storedGrant(t)
	require.True(t, ok)
	require.Equal(t, "2", authorizedBy)
	requireOneGrant()
}

// TestPreconfiguredOAuthAuthorizationByTheNoAuthOwner checks that without authentication, the "nobody" owner that every
// request is (authn.NoAuth) can authorize a vMCP's preconfigured OAuth grant: the attempt is bound to that owner's ID,
// and its callback, which is that owner's request too, completes it and stores the grant as authorized by that owner.
func TestPreconfiguredOAuthAuthorizationByTheNoAuthOwner(t *testing.T) {
	f := newAgentGrantFixture(t)
	// The no auth authenticator reads the default role setting, which the fixture's gateway client has no storage for,
	// so the owner is created through another client of the same database.
	sqlDB, err := f.db.DB()
	require.NoError(t, err)
	database, err := gatewaydb.New(f.db, sqlDB, true)
	require.NoError(t, err)
	roleStorage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.UserDefaultRoleSetting{
		Namespace: system.DefaultNamespace,
		Name:      system.DefaultRoleSettingName,
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: types.RoleBasic,
		},
	}).Build()
	noAuthGateway := gatewayclient.New(t.Context(), database, roleStorage, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = noAuthGateway.Close() })
	response, ok, err := authn.NewNoAuth(noAuthGateway).AuthenticateRequest(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	require.NoError(t, err)
	require.True(t, ok)
	nobody := response.User

	state, err := f.start(t, nobody, agentGrantRemote)
	require.NoError(t, err)
	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, nobody.GetUID(), pending.GrantInitiatorUserID)

	recorder, err := f.callback(nobody, state)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	access, authorizedBy, _, stored := f.storedGrant(t)
	require.True(t, stored)
	require.Equal(t, agentGrantAccess, access)
	require.Equal(t, nobody.GetUID(), authorizedBy)
}

// TestPreconfiguredOAuthCallbackRevalidates checks that a callback stores nothing when what it was started for changed in
// between: the component was deleted, the vMCP is being deleted, or the component's upstream URL or definition
// changed. Which profiles grant the component never matters: any remote component can be authorized.
func TestPreconfiguredOAuthCallbackRevalidates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, f *agentGrantFixture)
		code   int
	}{
		{
			name: "component deleted",
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				f.update(t, func(vmcp *v1.VMCP) { vmcp.Spec.Manifest.Components = vmcp.Spec.Manifest.Components[1:] })
			},
			code: http.StatusNotFound,
		},
		{
			name: "vMCP being deleted",
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				f.deleteVMCP(t)
			},
			code: http.StatusConflict,
		},
		{
			name: "upstream URL changed",
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				f.update(t, func(vmcp *v1.VMCP) {
					vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.FixedURL += "/moved"
				})
			},
			code: http.StatusConflict,
		},
		{
			name: "static OAuth credential changed",
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				f.update(t, func(vmcp *v1.VMCP) { vmcp.Spec.Manifest.Components[0].OAuthCredentialID = "another-credential" })
			},
			code: http.StatusConflict,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAgentGrantFixture(t)
			state, err := f.start(t, administrator("1"), agentGrantRemote)
			require.NoError(t, err)
			tt.change(t, f)

			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, tt.code)
			_, _, _, ok := f.storedGrant(t)
			require.False(t, ok)
			require.Zero(t, f.provider.exchanges.Load(), "a stale callback exchanged its code")
			// The attempt can never succeed, so it is gone.
			_, err = f.gateway.GetMCPOAuthPendingState(t.Context(), state)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}
}

// TestPreconfiguredOAuthStartRefusals checks which authorizations are refused before anything is stored.
func TestPreconfiguredOAuthStartRefusals(t *testing.T) {
	for _, tt := range []struct {
		name      string
		caller    kuser.Info
		change    func(t *testing.T, f *agentGrantFixture)
		component string
		code      int
		wantErr   string
	}{
		{
			name:   "personal vMCP",
			caller: administrator("1"),
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				f.update(t, func(vmcp *v1.VMCP) { vmcp.Spec.UserID = "5" })
			},
			component: agentGrantRemote,
			code:      http.StatusBadRequest,
			wantErr:   "personal",
		},
		{
			name:   "vMCP being deleted",
			caller: administrator("1"),
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				f.deleteVMCP(t)
			},
			component: agentGrantRemote,
			code:      http.StatusConflict,
		},
		{
			name:      "component without OAuth",
			caller:    administrator("1"),
			component: agentGrantLocal,
			code:      http.StatusBadRequest,
			wantErr:   "not a remote server",
		},
		{
			name:      "unknown component",
			caller:    administrator("1"),
			component: "missing",
			code:      http.StatusNotFound,
		},
		{
			name:      "unknown vMCP",
			caller:    administrator("1"),
			change:    func(t *testing.T, f *agentGrantFixture) { t.Helper(); f.key.Name = "vmcp1missing" },
			component: agentGrantRemote,
			code:      http.StatusNotFound,
		},
		// Which users reach the start is pkg/api/authz's to decide (TestVMCPPreconfiguredOAuthAuthorization): only
		// administrators and owners. The start still refuses any caller that is not a user, since a grant records the
		// user who authorized it.
		{
			name:      "agent a profile names",
			caller:    agentGrantAgent(7),
			component: agentGrantRemote,
			code:      http.StatusForbidden,
		},
		{
			name:      "agent with administrator groups",
			caller:    agentWithGroups(7, types.GroupAdmin, types.GroupAuthenticated),
			component: agentGrantRemote,
			code:      http.StatusForbidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAgentGrantFixture(t)
			if tt.change != nil {
				tt.change(t, f)
			}
			_, err := f.start(t, tt.caller, tt.component)
			requireErrHTTP(t, err, tt.code)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			}
			f.requireNoPending(t)
		})
	}

	// A component whose URL comes from configuration that the vMCP does not fix, so agents could not reach it either.
	f := newAgentGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1"})
	_, err := f.start(t, administrator("1"), agentGrantRemote)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.ErrorContains(t, err, "set it as fixed on the vMCP or on the catalog entry first")
	require.ErrorContains(t, err, "__url")
	require.NotContains(t, strings.ToLower(err.Error()), "profile")
	// The keys to set come back as structured reasons, with no value and no profile, for the administrator to be pointed
	// at the vMCP.
	var withReasons *api.ErrHTTPWithReasons
	require.ErrorAs(t, err, &withReasons)
	require.Equal(t, []types.AgentReadinessReason{{
		ComponentID: agentGrantRemote,
		Code:        types.AgentReadinessConfigurationRequired,
		Field:       "__url",
	}}, withReasons.Reasons)
	reasons, err := json.Marshal(withReasons.Reasons)
	require.NoError(t, err)
	require.NotContains(t, string(reasons), "profile")
	f.requireNoPending(t)
}

// TestPreconfiguredOAuthStartWithoutAnInitializeChallenge starts the vMCP's grant for a component whose server lets anyone
// initialize and list its tools, at a URL the catalog fixes and at one the vMCP fixes:
//
//   - one that only challenges when a tool is called starts from the protected resource metadata it publishes, bound
//     to the vMCP's component, its URL and its administrator as any grant is, and the component is known to need OAuth
//     once the grant is stored;
//   - one that publishes none is refused as not requiring OAuth, as before;
//   - one that never answers tools/list fails the start once the request's time runs out, without falling back to the
//     metadata it publishes.
//
// Every start's probes are unauthenticated and end their session, a refused start leaves no pending authorization, and
// no error carries a URL that comes from the vMCP's fixed configuration.
func TestPreconfiguredOAuthStartWithoutAnInitializeChallenge(t *testing.T) {
	for _, configured := range []bool{false, true} {
		for _, tt := range []struct {
			name string
			mode string
			// wantScope is the scope the authorization asks for, when it starts.
			wantScope string
			// wantCode is the status of the error that refuses the start, if it is an HTTP error, and wantErr is part of
			// its message.
			wantCode int
			wantErr  string
		}{
			{
				name:      "tools/call challenge with published metadata",
				mode:      lateChallengeToolsCall,
				wantScope: "published",
			},
			{
				name:     "no challenge and no metadata",
				mode:     lateChallengeNone,
				wantCode: http.StatusBadRequest,
				wantErr:  `component "remote" does not require OAuth`,
			},
			{
				name:    "hangs on tools/list",
				mode:    lateChallengeHang,
				wantErr: "context deadline exceeded",
			},
		} {
			name := tt.name
			if configured {
				name += " at a URL the vMCP fixes"
			}
			t.Run(name, func(t *testing.T) {
				f := newAgentGrantFixture(t)
				if configured {
					f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1"})
					f.fix(t, map[string]string{"__url": f.provider.mcpURL()})
				}
				f.provider.late.Store(tt.mode)
				requireRequired := func(want bool) {
					t.Helper()
					oauth, _ := f.oauth(t)
					require.Equal(t, want, oauth.State != types.VMCPPreconfiguredOAuthNotRequired, oauth.State)
					for _, id := range []uint{7, 8} {
						if want {
							require.Empty(t, f.readiness(t, id), "agent %d", id)
						}
					}
				}
				requireRequired(false)

				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				req, _ := f.request(administrator("1"), http.MethodPost, "/api/vmcps/"+f.key.Name+"/components/"+agentGrantRemote+"/preconfigured-oauth")
				req.Request = req.WithContext(ctx)
				started := time.Now()
				authorization, err := f.factory.StartPreconfiguredOAuth(req, f.key.Name, agentGrantRemote)
				require.Less(t, time.Since(started), 10*time.Second)
				requireUnauthenticatedProbes(t, f.provider.lateProbes())

				if tt.wantErr != "" {
					require.ErrorContains(t, err, tt.wantErr)
					if tt.wantCode != 0 {
						requireErrHTTP(t, err, tt.wantCode)
					}
					if configured {
						require.NotContains(t, err.Error(), f.provider.mcpURL())
					}
					f.requireNoPending(t)
					requireRequired(false)
					return
				}

				require.NoError(t, err)
				parsed, err := url.Parse(authorization.AuthURL)
				require.NoError(t, err)
				require.Equal(t, f.provider.server.URL+"/authorize", parsed.Scheme+"://"+parsed.Host+parsed.Path)
				require.Equal(t, tt.wantScope, parsed.Query().Get("scope"))
				state := parsed.Query().Get("state")
				pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
				require.NoError(t, err)
				require.Equal(t, principal.PreconfiguredOAuthOwner(f.key.Name), pending.UserID)
				require.Equal(t, f.provider.mcpURL(), pending.URL)
				require.Equal(t, "1", pending.GrantInitiatorUserID)

				// Only its administrator completes it, once.
				_, err = f.callback(administrator("2"), state)
				requireErrHTTP(t, err, http.StatusForbidden)
				_, err = f.callback(administrator("1"), state)
				require.NoError(t, err)
				_, _, target, ok := f.storedGrant(t)
				require.True(t, ok)
				require.Equal(t, f.provider.mcpURL(), target)
				_, err = f.callback(administrator("1"), state)
				requireErrHTTP(t, err, http.StatusBadRequest)

				// The grant shows that the component needs OAuth, and every agent granted it is ready with it.
				requireRequired(true)
			})
		}
	}
}

// setRemote replaces how the fixture's remote component reaches its upstream.
func (f *agentGrantFixture) setRemote(t *testing.T, remote *types.RemoteCatalogConfig) {
	t.Helper()
	f.update(t, func(vmcp *v1.VMCP) { vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig = remote })
}

// fix fixes values of the remote component on the vMCP, for everyone, agents included, as an administrator does: the
// fixed policy for each key, and the values in the vMCP's fixed configuration.
func (f *agentGrantFixture) fix(t *testing.T, values map[string]string) {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	component := &vmcp.Spec.Manifest.Components[0]
	secrets := map[string]string{}
	for key, value := range values {
		component.Configuration = slices.DeleteFunc(component.Configuration, func(policy types.VMCPConfigurationPolicy) bool { return policy.Key == key })
		component.Configuration = append(component.Configuration, types.VMCPConfigurationPolicy{Key: key, Policy: types.VMCPConfigurationPolicyFixed})
		secrets[vmcpconfig.ConfigurationKey(component.ID, key)] = value
	}
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	require.NoError(t, f.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.StaticConfigurationCredentialName(&vmcp),
		Secrets: secrets,
	}))
}

// TestPreconfiguredOAuthsForFixedURLs authorizes the vMCP's grant for a remote component whose upstream URL comes from the
// vMCP's fixed configuration, a hostname-constrained one and a URL template: the grant is bound to the URL that agents
// reach, and every agent granted the component is ready with it. A URL nothing fixes is refused, and a grant for a URL
// that changed since, before or after the callback, is refused.
func TestPreconfiguredOAuthsForFixedURLs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		remote func(base string) *types.RemoteCatalogConfig
		config []types.MCPConfig
		policy []types.VMCPConfigurationPolicy
		// value is the fixed configuration that makes the component reach path on the provider.
		value func(base, path string) map[string]string
	}{
		{
			name:   "hostname constraint",
			remote: func(string) *types.RemoteCatalogConfig { return &types.RemoteCatalogConfig{Hostname: "127.0.0.1"} },
			value: func(base, path string) map[string]string {
				return map[string]string{"__url": base + path}
			},
		},
		{
			name: "URL template",
			remote: func(base string) *types.RemoteCatalogConfig {
				return &types.RemoteCatalogConfig{URLTemplate: base + "/${PATH}"}
			},
			config: []types.MCPConfig{{Key: "PATH", Required: true}},
			policy: []types.VMCPConfigurationPolicy{{Key: "PATH", Policy: types.VMCPConfigurationPolicyUserAllowed}},
			value: func(_, path string) map[string]string {
				return map[string]string{"PATH": strings.TrimPrefix(path, "/")}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAgentGrantFixture(t)
			base := f.provider.server.URL
			f.update(t, func(vmcp *v1.VMCP) {
				vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig = tt.remote(base)
				vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config = tt.config
				vmcp.Spec.Manifest.Components[0].Configuration = tt.policy
			})

			// Nothing fixes the URL yet.
			_, err := f.start(t, administrator("1"), agentGrantRemote)
			requireErrHTTP(t, err, http.StatusBadRequest)

			// The vMCP fixes the URL, and its grant is bound to it.
			f.fix(t, tt.value(base, "/fixed"))
			state, err := f.start(t, administrator("1"), agentGrantRemote)
			require.NoError(t, err)
			pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
			require.NoError(t, err)
			require.Equal(t, base+"/fixed", pending.URL)
			_, err = f.callback(administrator("1"), state)
			require.NoError(t, err)
			_, _, target, ok := f.storedGrant(t)
			require.True(t, ok)
			require.Equal(t, base+"/fixed", target)

			// Every agent granted the component reaches the URL the grant is for, and is ready. The setup names only the
			// hostname of a URL the vMCP's configuration sets.
			for _, id := range []uint{7, 8} {
				require.Empty(t, f.readiness(t, id), "agent %d", id)
			}
			oauth, raw := f.oauth(t)
			require.Equal(t, types.VMCPPreconfiguredOAuthAuthorized, oauth.State)
			require.Empty(t, oauth.Target)
			require.Equal(t, vmcpconfig.URLHostname(base), oauth.TargetHostname)
			require.NotContains(t, raw, base+"/fixed")

			// The URL the vMCP fixes changes while another authorization waits for its callback: the callback is refused,
			// and its attempt consumed.
			state, err = f.start(t, administrator("1"), agentGrantRemote)
			require.NoError(t, err)
			f.fix(t, tt.value(base, "/moved"))
			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, http.StatusConflict)
			_, err = f.gateway.GetMCPOAuthPendingState(t.Context(), state)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)

			// The grant was authorized for the old URL, which agents reach no more: it is invalid, for every agent.
			oauth, _ = f.oauth(t)
			require.Equal(t, types.VMCPPreconfiguredOAuthInvalid, oauth.State)
			for _, id := range []uint{7, 8} {
				require.Equal(t, []types.AgentReadinessReason{{ComponentID: agentGrantRemote, Code: types.AgentReadinessOAuthInvalid}}, f.readiness(t, id), "agent %d", id)
			}
		})
	}
}

// agentGrantAgent returns the principal of the agent with id.
func agentGrantAgent(id uint) kuser.Info {
	return agentWithGroups(id, types.GroupMCP, types.GroupAuthenticated)
}

// agentWithGroups returns the principal of the agent with id, in groups.
func agentWithGroups(id uint, groups ...string) kuser.Info {
	extra := map[string][]string{}
	principal.RecordKind(extra, principal.KindAgent)
	uid := principal.Agent(id).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Groups: groups, Extra: extra}
}

// startRelay starts, as caller, the authorization of the vMCP's grant for the fixture's remote component, whose
// provider only redirects to a localhost callback. It returns the response and the state of the pending attempt.
func (f *agentGrantFixture) startRelay(t *testing.T, caller kuser.Info) (types.VMCPPreconfiguredOAuthAuthorization, string, error) {
	t.Helper()
	req, _ := f.request(caller, http.MethodPost, "/api/vmcps/"+f.key.Name+"/components/"+agentGrantRemote+"/preconfigured-oauth")
	authorization, err := f.factory.StartPreconfiguredOAuth(req, f.key.Name, agentGrantRemote)
	if err != nil {
		return authorization, "", err
	}
	login, err := url.Parse(authorization.AuthURL)
	require.NoError(t, err)
	state, ok := strings.CutPrefix(login.Path, "/oauth/mcp/login/")
	require.True(t, ok, "authorization URL %s is not the local login URL", authorization.AuthURL)
	return authorization, state, nil
}

// readRelay reads the attempt as `obot mcp login` does, without any credential.
func (f *agentGrantFixture) readRelay(t *testing.T, state string) (types.MCPLocalLogin, string) {
	t.Helper()
	req, recorder := f.request(&kuser.DefaultInfo{UID: "anonymous", Name: "anonymous"}, http.MethodGet, "/oauth/mcp/login/"+state)
	req.SetPathValue("state", state)
	require.NoError(t, f.handler.localLogin(req))
	var login types.MCPLocalLogin
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &login))
	return login, recorder.Body.String()
}

// TestPreconfiguredOAuthThroughTheLocalhostRelay authorizes the vMCP's grant for a component whose provider only redirects
// to a localhost callback, through the same relay as a user's own connection: the start returns the local login URL,
// `obot mcp login` reads the attempt and relays the provider's callback to Obot in the administrator's browser, and the
// grant is stored for the vMCP's component. Only the initiating administrator completes it, once, and the grant is then
// refreshed like any other, with no redirect.
func TestPreconfiguredOAuthThroughTheLocalhostRelay(t *testing.T) {
	f := newAgentGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{FixedURL: f.provider.mcpURL(), LocalhostCallbackEnabled: true, LocalhostCallbackPath: "/relay/callback"})

	authorization, state, err := f.startRelay(t, administrator("1"))
	require.NoError(t, err)
	require.True(t, authorization.LocalhostCallback)
	require.True(t, strings.HasPrefix(authorization.AuthURL, "http://obot.example/oauth/mcp/login/"), authorization.AuthURL)
	require.True(t, strings.HasPrefix(state, localLoginStatePrefix), state)
	require.Zero(t, f.provider.operations.Load(), "an authorization ran an MCP operation")

	// The attempt is the vMCP's, bound to its administrator, with the relay's loopback-only redirect.
	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	owner, mcpID := principal.PreconfiguredOAuthOwner(f.key.Name), vmcpconfig.PreconfiguredOAuthMCPID(f.key.Name, agentGrantRemote)
	require.Equal(t, owner, pending.UserID)
	require.Equal(t, "1", pending.GrantInitiatorUserID)
	require.True(t, localLoginState(pending))
	redirect, err := url.Parse(pending.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, "localhost", redirect.Hostname())
	require.Equal(t, "/relay/callback", redirect.Path)

	// The relay reads the attempt without a credential, and learns nothing but where to send the administrator.
	login, body := f.readRelay(t, state)
	require.Equal(t, pending.RedirectURL, login.RedirectURL)
	require.Equal(t, state, login.State)
	authURL, err := url.Parse(login.AuthorizationURL)
	require.NoError(t, err)
	require.Equal(t, f.provider.server.URL+"/authorize", authURL.Scheme+"://"+authURL.Host+authURL.Path)
	require.Equal(t, pending.RedirectURL, authURL.Query().Get("redirect_uri"))
	require.NotEmpty(t, authURL.Query().Get("code_challenge"))
	require.NotContains(t, body, pending.Verifier)
	require.NotContains(t, body, owner)

	// The relayed callback is refused to another administrator and to the administrator without the role, and the
	// attempt stays available.
	for _, caller := range []kuser.Info{administrator("2"), administrator("1", types.GroupBasic, types.GroupAuthenticated)} {
		_, err := f.callback(caller, state)
		requireErrHTTP(t, err, http.StatusForbidden)
	}
	_, err = f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err, "a refused relay consumed the attempt")

	// The initiating administrator completes it: the grant is stored for the vMCP's component, and the relay is told.
	recorder, err := f.callback(administrator("1"), state)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	completion, err := url.Parse(recorder.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, redirect.Host, completion.Host)
	require.Equal(t, "/oauth/obot/callback", completion.Path)
	require.Equal(t, "complete", completion.Query().Get("status"))
	require.Equal(t, state, completion.Query().Get("state"))
	require.Equal(t, pending.RedirectURL, f.provider.redirectURI.Load())
	access, authorizedBy, target, ok := f.storedGrant(t)
	require.True(t, ok)
	require.Equal(t, agentGrantAccess, access)
	require.Equal(t, "1", authorizedBy)
	require.Equal(t, f.provider.mcpURL(), target)

	// It is used up.
	_, err = f.callback(administrator("1"), state)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.EqualValues(t, 1, f.provider.exchanges.Load())

	// Once the access token expires, an agent's use refreshes the grant with no redirect and no relay.
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthToken{}).Where("user_id = ? AND mcp_id = ?", owner, mcpID).Update("expiry", time.Now().Add(-time.Hour)).Error)
	source, err := mcp.NewGlobalTokenStore(f.gateway).ForUserAndMCP(owner, mcpID, f.provider.mcpURL()).TokenSource(context.WithValue(t.Context(), oauth2.HTTPClient, http.DefaultClient))
	require.NoError(t, err)
	token, err := source.Token()
	require.NoError(t, err)
	require.Equal(t, agentGrantRefreshed, token.AccessToken)
	require.EqualValues(t, 1, f.provider.refreshes.Load())
	refreshed, err := f.gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, f.provider.mcpURL())
	require.NoError(t, err)
	require.Equal(t, agentGrantRefreshed, refreshed.AccessToken)
	require.Equal(t, "1", refreshed.AuthorizedBy)
}

// TestPreconfiguredOAuthRelayRefusals checks that a relayed attempt that expired, whose redirect is not the relay's
// loopback callback, or whose component changed since it started is refused, and stores nothing.
func TestPreconfiguredOAuthRelayRefusals(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, f *agentGrantFixture)
		code   int
	}{
		{
			// The relay waits ten minutes; so does its attempt.
			name: "expired",
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("state <> ''").Update("created_at", time.Now().Add(-localLoginTTL-time.Minute)).Error)
			},
			code: http.StatusBadRequest,
		},
		{
			name: "component changed",
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				f.setRemote(t, &types.RemoteCatalogConfig{FixedURL: f.provider.mcpURL() + "/moved", LocalhostCallbackEnabled: true})
			},
			code: http.StatusConflict,
		},
		{
			name: "redirect is not a loopback callback",
			change: func(t *testing.T, f *agentGrantFixture) {
				t.Helper()
				require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("state <> ''").Update("redirect_url", "http://attacker.example:8080/callback").Error)
			},
			code: http.StatusForbidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAgentGrantFixture(t)
			f.setRemote(t, &types.RemoteCatalogConfig{FixedURL: f.provider.mcpURL(), LocalhostCallbackEnabled: true})
			_, state, err := f.startRelay(t, administrator("1"))
			require.NoError(t, err)
			tt.change(t, f)

			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, tt.code)
			_, _, _, ok := f.storedGrant(t)
			require.False(t, ok)
			require.Zero(t, f.provider.exchanges.Load())
		})
	}

	// URLs the vMCP fixes go through the relay too.
	f := newAgentGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1", LocalhostCallbackEnabled: true})
	f.fix(t, map[string]string{"__url": f.provider.mcpURL()})
	authorization, state, err := f.startRelay(t, administrator("1"))
	require.NoError(t, err)
	require.True(t, authorization.LocalhostCallback)
	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, f.provider.mcpURL(), pending.URL)
	recorder, err := f.callback(administrator("1"), state)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	_, _, target, ok := f.storedGrant(t)
	require.True(t, ok)
	require.Equal(t, f.provider.mcpURL(), target)
}
