package oauth

import (
	"context"
	"encoding/json"
	"errors"
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
	profileGrantVMCP      = "vmcp1profilegrants"
	profileGrantSeven     = "vp1seven"
	profileGrantEight     = "vp1eight"
	profileGrantUsers     = "vp1users"
	profileGrantRemote    = "remote"
	profileGrantLocal     = "local"
	profileGrantAccess    = "provider-access-token"
	profileGrantRefresh   = "provider-refresh-token"
	profileGrantCodeSent  = "authorization-code"
	profileGrantRefreshed = "provider-refreshed-token"

	// lateChallengeToolsCall is a server that lets anyone initialize and list its tools, asks for OAuth only when a tool
	// is called, and publishes its protected resource metadata at the well-known location for its URL.
	lateChallengeToolsCall = "tools/call"
	// lateChallengeNone is a server that asks for OAuth nowhere and publishes no protected resource metadata.
	lateChallengeNone = "none"
	// lateChallengeHang is a server that lets anyone initialize, never answers tools/list, and publishes its protected
	// resource metadata.
	lateChallengeHang = "hang"
)

// profileGrantProvider is a remote MCP server that requires OAuth, with its own authorization server that registers
// clients dynamically and exchanges codes.
type profileGrantProvider struct {
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

// profileGrantFixture is a shared vMCP with a remote component that requires OAuth and a local one, with a profile for
// agent 7, one for agent 8 and one for every user, and the handlers that authorize their grants.
type profileGrantFixture struct {
	provider *profileGrantProvider
	storage  kclient.WithWatch
	// db is the gateway's database, for a test to age a pending authorization.
	db      *gorm.DB
	gateway *gatewayclient.Client
	grants  *vmcpconfig.ProfileOAuthGrantStore
	factory *MCPOAuthHandlerFactory
	handler *handler
	key     kclient.ObjectKey
}

// serveLate answers a request to the MCP endpoint of a server that asks for OAuth late, as mode says, after recording
// it. It hands out session "late-session".
func (p *profileGrantProvider) serveLate(w http.ResponseWriter, r *http.Request, mode string) {
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
func (p *profileGrantProvider) lateProbes() []lateProbe {
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

func newProfileGrantProvider(t *testing.T) *profileGrantProvider {
	t.Helper()
	p := &profileGrantProvider{}
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
				require.Equal(t, profileGrantRefresh, r.Form.Get("refresh_token"))
				require.Empty(t, r.Form.Get("redirect_uri"))
				p.refreshes.Add(1)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"access_token":  profileGrantRefreshed,
					"refresh_token": profileGrantRefresh,
					"token_type":    "Bearer",
					"expires_in":    3600,
				}))
				return
			}
			require.Equal(t, profileGrantCodeSent, r.Form.Get("code"))
			p.redirectURI.Store(r.Form.Get("redirect_uri"))
			p.exchanges.Add(1)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":  profileGrantAccess,
				"refresh_token": profileGrantRefresh,
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

func (p *profileGrantProvider) mcpURL() string {
	return p.server.URL + "/mcp"
}

func newProfileGrantFixture(t *testing.T) *profileGrantFixture {
	t.Helper()
	provider := newProfileGrantProvider(t)
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	vmcp := &v1.VMCP{Name: profileGrantVMCP, Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{
			{
				ID:                      profileGrantRemote,
				Name:                    "Remote",
				MCPServerCatalogEntryID: "entry",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: provider.mcpURL()},
				}},
			},
			{ID: profileGrantLocal, Name: "Local"},
		},
		Profiles: []types.VMCPProfile{
			profile(profileGrantSeven, types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			profile(profileGrantEight, types.Subject{Type: types.SubjectTypeAgent, ID: "8"}),
			profile(profileGrantUsers, types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
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
	grants := vmcpconfig.NewProfileOAuthGrantStore(gateway)
	grants.SetComponentTargets(manager)
	factory := NewMCPOAuthHandlerFactory("http://obot.example", manager, storage, gateway, tokens, "", true)
	factory.SetProfileOAuthGrants(grants)
	return &profileGrantFixture{
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

func (f *profileGrantFixture) request(caller kuser.Info, method, target string) (api.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	return api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(method, target, nil),
		Storage:        f.storage,
		GatewayClient:  f.gateway,
		User:           caller,
	}, recorder
}

// start starts an authorization of the profile's grant as caller, and returns the state of its pending attempt.
func (f *profileGrantFixture) start(t *testing.T, caller kuser.Info, profileID, componentID string) (string, error) {
	t.Helper()
	req, _ := f.request(caller, http.MethodPost, "/api/vmcps/"+f.key.Name+"/profiles/"+profileID+"/components/"+componentID+"/oauth")
	authorization, err := f.factory.StartProfileOAuthGrant(req, f.key.Name, profileID, componentID)
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
func (f *profileGrantFixture) callback(caller kuser.Info, state string) (*httptest.ResponseRecorder, error) {
	query := url.Values{"state": {state}, "code": {profileGrantCodeSent}}
	req, recorder := f.request(caller, http.MethodGet, "/oauth/mcp/callback?"+query.Encode())
	return recorder, f.handler.oauthCallback(req)
}

func (f *profileGrantFixture) storedGrant(t *testing.T, profileID string) (string, string, string, bool) {
	t.Helper()
	token, err := f.gateway.GetMCPOAuthToken(t.Context(), principal.ProfileGrant(f.key.Name, profileID), vmcpconfig.ProfileOAuthGrantMCPID(f.key.Name, profileGrantRemote), f.provider.mcpURL())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", "", false
	}
	require.NoError(t, err)
	return token.AccessToken, token.AuthorizedBy, token.URL, true
}

func requireErrHTTP(t *testing.T, err error, code int) {
	t.Helper()
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, code, httpErr.Code, httpErr.Message)
}

// TestProfileOAuthGrantAuthorization checks the whole authorization of a profile's grant: it is started for the
// profile, never for the administrator; its callback is refused to anyone but the administrator who started it, who
// must still be one; it stores the grant for the profile, with who authorized it; and its attempt cannot be used
// again.
func TestProfileOAuthGrantAuthorization(t *testing.T) {
	f := newProfileGrantFixture(t)
	state, err := f.start(t, administrator("1"), profileGrantEight, profileGrantRemote)
	require.NoError(t, err)
	require.Zero(t, f.provider.operations.Load(), "an authorization ran an MCP operation")

	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, principal.ProfileGrant(f.key.Name, profileGrantEight), pending.UserID)
	require.Equal(t, vmcpconfig.ProfileOAuthGrantMCPID(f.key.Name, profileGrantRemote), pending.MCPID)
	require.Equal(t, f.provider.mcpURL(), pending.URL)
	require.Equal(t, "1", pending.GrantInitiatorUserID)
	require.NotEmpty(t, pending.GrantDefinitionHash)

	// Another administrator, the initiating user without the administrator role, and an agent cannot complete it, and
	// it stays available to its administrator.
	agentExtra := map[string][]string{}
	principal.RecordKind(agentExtra, principal.KindAgent)
	for _, caller := range []kuser.Info{
		administrator("2"),
		administrator("1", types.GroupBasic, types.GroupAuthenticated),
		&kuser.DefaultInfo{UID: "agent:1", Groups: []string{types.GroupAdmin, types.GroupAuthenticated}, Extra: agentExtra},
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
	access, authorizedBy, target, ok := f.storedGrant(t, profileGrantEight)
	require.True(t, ok)
	require.Equal(t, profileGrantAccess, access)
	require.Equal(t, "1", authorizedBy)
	require.Equal(t, f.provider.mcpURL(), target)
	// The administrator holds no token of their own for it.
	_, err = f.gateway.GetMCPOAuthToken(t.Context(), "1", vmcpconfig.ProfileOAuthGrantMCPID(f.key.Name, profileGrantRemote), f.provider.mcpURL())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// The grant is recorded, and its status names who authorized it and for which target, never its tokens.
	var stored v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &stored))
	require.Len(t, stored.Status.ProfileOAuthGrants, 1)
	status, err := f.grants.ProfileOAuthStatus(t.Context(), stored, profileGrantEight)
	require.NoError(t, err)
	require.Equal(t, types.VMCPProfileOAuthAuthorized, status[profileGrantRemote].State)
	require.Equal(t, "1", status[profileGrantRemote].AuthorizedBy)
	require.Equal(t, f.provider.mcpURL(), status[profileGrantRemote].Target)
	statusJSON, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(statusJSON), profileGrantAccess)
	require.NotContains(t, string(statusJSON), profileGrantRefresh)

	// The authorization is recorded once, as the administrator's, with the component, scopes and hostname of the target,
	// and never the token or the URL. The refused callbacks recorded nothing.
	events, _, err := f.gateway.ListAdminAuditEvents(t.Context(), gatewayclient.AdminAuditEventOptions{})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "1", events[0].ActorUserID)
	require.Equal(t, "vmcp.profile.oauth.authorize", events[0].Action)
	require.Equal(t, "vmcpProfile", events[0].ResourceKind)
	require.Equal(t, profileGrantEight, events[0].ResourceID)
	require.Equal(t, f.key.Name, events[0].ParentID)
	var details map[string]any
	require.NoError(t, json.Unmarshal(events[0].Details, &details))
	require.Equal(t, profileGrantRemote, details["componentID"])
	require.Equal(t, vmcpconfig.URLHostname(f.provider.mcpURL()), details["hostname"])
	require.Contains(t, details, "scopes")
	require.NotContains(t, string(events[0].Details), profileGrantAccess)
	require.NotContains(t, string(events[0].Details), profileGrantRefresh)
	require.NotContains(t, string(events[0].Details), f.provider.mcpURL())

	// The attempt is used up.
	_, err = f.callback(administrator("1"), state)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.EqualValues(t, 1, f.provider.exchanges.Load())
}

// TestProfileOAuthGrantAuthorizationByTheNoAuthOwner checks that without authentication, the "nobody" owner that every
// request is (authn.NoAuth) can authorize a profile's grant: the attempt is bound to that owner's ID, and its callback,
// which is that owner's request too, completes it and stores the grant as authorized by that owner.
func TestProfileOAuthGrantAuthorizationByTheNoAuthOwner(t *testing.T) {
	f := newProfileGrantFixture(t)
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

	state, err := f.start(t, nobody, profileGrantEight, profileGrantRemote)
	require.NoError(t, err)
	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, nobody.GetUID(), pending.GrantInitiatorUserID)

	recorder, err := f.callback(nobody, state)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	access, authorizedBy, _, stored := f.storedGrant(t, profileGrantEight)
	require.True(t, stored)
	require.Equal(t, profileGrantAccess, access)
	require.Equal(t, nobody.GetUID(), authorizedBy)
}

// TestProfileOAuthGrantCallbackRevalidates checks that a callback stores nothing when what it was started for changed
// in between: the profile was deleted, or the component's upstream URL or definition changed.
func TestProfileOAuthGrantCallbackRevalidates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*v1.VMCP)
		code   int
	}{
		{
			name:   "profile deleted",
			change: func(vmcp *v1.VMCP) { vmcp.Spec.Manifest.Profiles = vmcp.Spec.Manifest.Profiles[:1] },
			code:   http.StatusNotFound,
		},
		{
			name: "profile no longer names agents",
			change: func(vmcp *v1.VMCP) {
				vmcp.Spec.Manifest.Profiles[1].Subjects = []types.Subject{{Type: types.SubjectTypeUser, ID: "3"}}
			},
			code: http.StatusBadRequest,
		},
		{
			name:   "component deleted",
			change: func(vmcp *v1.VMCP) { vmcp.Spec.Manifest.Components = vmcp.Spec.Manifest.Components[1:] },
			code:   http.StatusNotFound,
		},
		{
			name: "component no longer granted",
			change: func(vmcp *v1.VMCP) {
				vmcp.Spec.Manifest.Profiles[1].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{profileGrantLocal: {}}}
			},
			code: http.StatusBadRequest,
		},
		{
			name: "upstream URL changed",
			change: func(vmcp *v1.VMCP) {
				vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.FixedURL += "/moved"
			},
			code: http.StatusConflict,
		},
		{
			name: "static OAuth credential changed",
			change: func(vmcp *v1.VMCP) {
				vmcp.Spec.Manifest.Components[0].OAuthCredentialID = "another-credential"
			},
			code: http.StatusConflict,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newProfileGrantFixture(t)
			state, err := f.start(t, administrator("1"), profileGrantEight, profileGrantRemote)
			require.NoError(t, err)
			var vmcp v1.VMCP
			require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
			tt.change(&vmcp)
			require.NoError(t, f.storage.Update(t.Context(), &vmcp))

			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, tt.code)
			_, _, _, ok := f.storedGrant(t, profileGrantEight)
			require.False(t, ok)
			require.Zero(t, f.provider.exchanges.Load(), "a stale callback exchanged its code")
			// The attempt can never succeed, so it is gone.
			_, err = f.gateway.GetMCPOAuthPendingState(t.Context(), state)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}
}

// TestProfileOAuthGrantStartRefusals checks which authorizations are refused before anything is stored.
func TestProfileOAuthGrantStartRefusals(t *testing.T) {
	agentExtra := map[string][]string{}
	principal.RecordKind(agentExtra, principal.KindAgent)
	agent := &kuser.DefaultInfo{UID: "agent:7", Groups: []string{types.GroupAdmin, types.GroupAuthenticated}, Extra: agentExtra}
	for _, tt := range []struct {
		name      string
		caller    kuser.Info
		profile   string
		component string
		code      int
	}{
		{name: "profile for every user", caller: administrator("1"), profile: profileGrantUsers, component: profileGrantRemote, code: http.StatusBadRequest},
		{name: "unknown profile", caller: administrator("1"), profile: "vp1missing", component: profileGrantRemote, code: http.StatusNotFound},
		{name: "component without OAuth", caller: administrator("1"), profile: profileGrantEight, component: profileGrantLocal, code: http.StatusBadRequest},
		{name: "unknown component", caller: administrator("1"), profile: profileGrantEight, component: "missing", code: http.StatusNotFound},
		{name: "not an administrator", caller: administrator("1", types.GroupBasic, types.GroupAuthenticated), profile: profileGrantEight, component: profileGrantRemote, code: http.StatusForbidden},
		{name: "agent", caller: agent, profile: profileGrantEight, component: profileGrantRemote, code: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newProfileGrantFixture(t)
			_, err := f.start(t, tt.caller, tt.profile, tt.component)
			requireErrHTTP(t, err, tt.code)
		})
	}

	// A component the profile does not grant: the profile's grant would never be used, so it is refused, naming it.
	f := newProfileGrantFixture(t)
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	vmcp.Spec.Manifest.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{profileGrantLocal: {}}}
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	_, err := f.start(t, administrator("1"), profileGrantSeven, profileGrantRemote)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.Contains(t, err.Error(), `does not grant component "`+profileGrantRemote+`"`)

	// A component whose URL comes from configuration that no profile of an agent using the grant sets.
	f = newProfileGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1"})
	_, err = f.start(t, administrator("1"), profileGrantEight, profileGrantRemote)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.True(t, strings.Contains(err.Error(), "configure the profile first"), err.Error())
	require.True(t, strings.Contains(err.Error(), "__url"), err.Error())
	// The keys to set come back as structured reasons, with no value, for the administrator to be pointed at.
	var withReasons *api.ErrHTTPWithReasons
	require.ErrorAs(t, err, &withReasons)
	require.Equal(t, []types.AgentReadinessReason{{
		ComponentID: profileGrantRemote,
		Code:        types.AgentReadinessConfigurationRequired,
		Field:       "__url",
		ProfileID:   profileGrantEight,
	}}, withReasons.Reasons)
}

// TestProfileOAuthGrantStartWithoutAnInitializeChallenge starts the grant of a component whose server lets anyone
// initialize and list its tools, at a URL the catalog fixes and at one the profile configures:
//
//   - one that only challenges when a tool is called starts from the protected resource metadata it publishes, bound
//     to the profile, its URL and its administrator as any grant is, and the component is known to need OAuth once the
//     grant is stored;
//   - one that publishes none is refused as not requiring OAuth, as before;
//   - one that never answers tools/list fails the start once the request's time runs out, without falling back to the
//     metadata it publishes.
//
// Every start's probes are unauthenticated and end their session, a refused start leaves no pending authorization, and
// no error carries a URL that comes from configuration.
func TestProfileOAuthGrantStartWithoutAnInitializeChallenge(t *testing.T) {
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
				name += " at a configured URL"
			}
			t.Run(name, func(t *testing.T) {
				f := newProfileGrantFixture(t)
				if configured {
					f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1"})
					f.configure(t, profileGrantEight, map[string]*string{"__url": new(f.provider.mcpURL())})
				}
				f.provider.late.Store(tt.mode)
				requireRequired := func(want bool) {
					t.Helper()
					var vmcp v1.VMCP
					require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
					for _, profileID := range []string{profileGrantSeven, profileGrantEight} {
						status, err := f.grants.ProfileOAuthStatus(t.Context(), vmcp, profileID)
						require.NoError(t, err)
						require.Equal(t, want, status[profileGrantRemote].Required, profileID)
					}
				}
				requireRequired(false)

				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				req, _ := f.request(administrator("1"), http.MethodPost, "/api/vmcps/"+f.key.Name+"/profiles/"+profileGrantEight+"/components/"+profileGrantRemote+"/oauth")
				req.Request = req.WithContext(ctx)
				started := time.Now()
				authorization, err := f.factory.StartProfileOAuthGrant(req, f.key.Name, profileGrantEight, profileGrantRemote)
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
					var pending int64
					require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Count(&pending).Error)
					require.Zero(t, pending, "a refused start left a pending authorization")
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
				require.Equal(t, principal.ProfileGrant(f.key.Name, profileGrantEight), pending.UserID)
				require.Equal(t, f.provider.mcpURL(), pending.URL)
				require.Equal(t, "1", pending.GrantInitiatorUserID)

				// Only its administrator completes it, once.
				_, err = f.callback(administrator("2"), state)
				requireErrHTTP(t, err, http.StatusForbidden)
				_, err = f.callback(administrator("1"), state)
				require.NoError(t, err)
				_, _, target, ok := f.storedGrant(t, profileGrantEight)
				require.True(t, ok)
				require.Equal(t, f.provider.mcpURL(), target)
				_, err = f.callback(administrator("1"), state)
				requireErrHTTP(t, err, http.StatusBadRequest)

				// The grant shows that the component needs OAuth, to every profile, including the one that holds none.
				requireRequired(true)
			})
		}
	}
}

// setRemote replaces how the fixture's remote component reaches its upstream.
func (f *profileGrantFixture) setRemote(t *testing.T, remote *types.RemoteCatalogConfig) {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig = remote
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
}

// configure sets a profile's configuration of the remote component, as an administrator does through spec 12's API.
func (f *profileGrantFixture) configure(t *testing.T, profileID string, values map[string]*string) {
	t.Helper()
	_, err := vmcpconfig.NewProfileConfigurationStore(f.gateway).Update(t.Context(), f.storage, f.key, profileID, map[string]map[string]*string{profileGrantRemote: values})
	require.NoError(t, err)
}

// TestProfileOAuthGrantsForConfiguredURLs authorizes grants for remote components whose upstream URL comes from
// configuration, a hostname-constrained one and a URL template: each grant is bound to the URL that its own profile's
// agents reach, and an agent that reaches it is ready. A grant for a URL that changed since, before or after the
// callback, is refused.
func TestProfileOAuthGrantsForConfiguredURLs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		remote func(base string) *types.RemoteCatalogConfig
		config []types.MCPConfig
		policy []types.VMCPConfigurationPolicy
		// value sets the configuration that makes the component reach path on the provider.
		value func(base, path string) map[string]*string
	}{
		{
			name:   "hostname constraint",
			remote: func(string) *types.RemoteCatalogConfig { return &types.RemoteCatalogConfig{Hostname: "127.0.0.1"} },
			value: func(base, path string) map[string]*string {
				return map[string]*string{"__url": new(base + path)}
			},
		},
		{
			name: "URL template",
			remote: func(base string) *types.RemoteCatalogConfig {
				return &types.RemoteCatalogConfig{URLTemplate: base + "/${PATH}"}
			},
			config: []types.MCPConfig{{Key: "PATH", Required: true}},
			policy: []types.VMCPConfigurationPolicy{{Key: "PATH", Policy: types.VMCPConfigurationPolicyUserAllowed}},
			value: func(_, path string) map[string]*string {
				return map[string]*string{"PATH": new(strings.TrimPrefix(path, "/"))}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newProfileGrantFixture(t)
			base := f.provider.server.URL
			var vmcp v1.VMCP
			require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
			vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig = tt.remote(base)
			vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config = tt.config
			vmcp.Spec.Manifest.Components[0].Configuration = tt.policy
			require.NoError(t, f.storage.Update(t.Context(), &vmcp))

			// Nothing configures the URL yet.
			_, err := f.start(t, administrator("1"), profileGrantSeven, profileGrantRemote)
			requireErrHTTP(t, err, http.StatusBadRequest)

			// Each profile configures its own agents' URL, and its grant is bound to it.
			for _, profile := range []struct{ id, path string }{{id: profileGrantSeven, path: "/seven"}, {id: profileGrantEight, path: "/eight"}} {
				f.configure(t, profile.id, tt.value(base, profile.path))
				state, err := f.start(t, administrator("1"), profile.id, profileGrantRemote)
				require.NoError(t, err)
				pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
				require.NoError(t, err)
				require.Equal(t, base+profile.path, pending.URL)
				_, err = f.callback(administrator("1"), state)
				require.NoError(t, err)
				_, _, target, ok := f.storedGrant(t, profile.id)
				require.True(t, ok)
				require.Equal(t, base+profile.path, target)
			}

			// Each agent reaches the URL its profile's grant is for, and is ready.
			var stored v1.VMCP
			require.NoError(t, f.storage.Get(t.Context(), f.key, &stored))
			stored.Spec.Manifest.Components = stored.Spec.Manifest.Components[:1]
			for _, agentID := range []uint{7, 8} {
				report, err := vmcpconfig.AgentReadiness(t.Context(), f.grants, stored, nil, profileGrantAgent(agentID))
				require.NoError(t, err)
				require.Empty(t, report.Readiness.Reasons, "agent %d", agentID)
			}

			// Agent 8's URL changes while an authorization of its profile's grant waits for its callback: the callback is
			// refused, and its attempt consumed.
			state, err := f.start(t, administrator("1"), profileGrantEight, profileGrantRemote)
			require.NoError(t, err)
			f.configure(t, profileGrantEight, tt.value(base, "/moved"))
			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, http.StatusConflict)
			_, err = f.gateway.GetMCPOAuthPendingState(t.Context(), state)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)

			// Agent 8's grant was authorized for the old URL, which its agents reach no more; agent 7's still works.
			require.NoError(t, f.storage.Get(t.Context(), f.key, &stored))
			for profileID, want := range map[string]string{profileGrantSeven: types.VMCPProfileOAuthAuthorized, profileGrantEight: types.VMCPProfileOAuthInvalid} {
				status, err := f.grants.ProfileOAuthStatus(t.Context(), stored, profileID)
				require.NoError(t, err)
				require.Equal(t, want, status[profileGrantRemote].State, profileID)
			}
		})
	}
}

// profileGrantAgent returns the principal of the agent with id.
func profileGrantAgent(id uint) kuser.Info {
	extra := map[string][]string{}
	principal.RecordKind(extra, principal.KindAgent)
	uid := principal.Agent(id).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Groups: []string{types.GroupMCP, types.GroupAuthenticated}, Extra: extra}
}

// startRelay starts, as caller, the authorization of a profile's grant for the fixture's remote component, whose
// provider only redirects to a localhost callback. It returns the response and the state of the pending attempt.
func (f *profileGrantFixture) startRelay(t *testing.T, caller kuser.Info, profileID string) (types.VMCPProfileOAuthAuthorization, string, error) {
	t.Helper()
	req, _ := f.request(caller, http.MethodPost, "/api/vmcps/"+f.key.Name+"/profiles/"+profileID+"/components/"+profileGrantRemote+"/oauth")
	authorization, err := f.factory.StartProfileOAuthGrant(req, f.key.Name, profileID, profileGrantRemote)
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
func (f *profileGrantFixture) readRelay(t *testing.T, state string) (types.MCPLocalLogin, string) {
	t.Helper()
	req, recorder := f.request(&kuser.DefaultInfo{UID: "anonymous", Name: "anonymous"}, http.MethodGet, "/oauth/mcp/login/"+state)
	req.SetPathValue("state", state)
	require.NoError(t, f.handler.localLogin(req))
	var login types.MCPLocalLogin
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &login))
	return login, recorder.Body.String()
}

// TestProfileOAuthGrantThroughTheLocalhostRelay authorizes the grant of a component whose provider only redirects to a
// localhost callback, through the same relay as a user's own connection: the start returns the local login URL, `obot
// mcp login` reads the attempt and relays the provider's callback to Obot in the administrator's browser, and the
// grant is stored for the profile. Only the initiating administrator completes it, once, and the grant is then
// refreshed like any other, with no redirect.
func TestProfileOAuthGrantThroughTheLocalhostRelay(t *testing.T) {
	f := newProfileGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{FixedURL: f.provider.mcpURL(), LocalhostCallbackEnabled: true, LocalhostCallbackPath: "/relay/callback"})

	authorization, state, err := f.startRelay(t, administrator("1"), profileGrantEight)
	require.NoError(t, err)
	require.True(t, authorization.LocalhostCallback)
	require.True(t, strings.HasPrefix(authorization.AuthURL, "http://obot.example/oauth/mcp/login/"), authorization.AuthURL)
	require.True(t, strings.HasPrefix(state, localLoginStatePrefix), state)
	require.Zero(t, f.provider.operations.Load(), "an authorization ran an MCP operation")

	// The attempt is the profile's, bound to its administrator, with the relay's loopback-only redirect.
	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	owner, mcpID := principal.ProfileGrant(f.key.Name, profileGrantEight), vmcpconfig.ProfileOAuthGrantMCPID(f.key.Name, profileGrantRemote)
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

	// The initiating administrator completes it: the grant is stored for the profile, and the relay is told.
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
	access, authorizedBy, target, ok := f.storedGrant(t, profileGrantEight)
	require.True(t, ok)
	require.Equal(t, profileGrantAccess, access)
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
	require.Equal(t, profileGrantRefreshed, token.AccessToken)
	require.EqualValues(t, 1, f.provider.refreshes.Load())
	refreshed, err := f.gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, f.provider.mcpURL())
	require.NoError(t, err)
	require.Equal(t, profileGrantRefreshed, refreshed.AccessToken)
	require.Equal(t, "1", refreshed.AuthorizedBy)
}

// TestProfileOAuthGrantRelayRefusals checks that a relayed attempt that expired, whose redirect is not the relay's
// loopback callback, or whose component changed since it started is refused, and stores nothing.
func TestProfileOAuthGrantRelayRefusals(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, f *profileGrantFixture)
		code   int
	}{
		{
			// The relay waits ten minutes; so does its attempt.
			name: "expired",
			change: func(t *testing.T, f *profileGrantFixture) {
				t.Helper()
				require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("state <> ''").Update("created_at", time.Now().Add(-localLoginTTL-time.Minute)).Error)
			},
			code: http.StatusBadRequest,
		},
		{
			name: "component changed",
			change: func(t *testing.T, f *profileGrantFixture) {
				t.Helper()
				f.setRemote(t, &types.RemoteCatalogConfig{FixedURL: f.provider.mcpURL() + "/moved", LocalhostCallbackEnabled: true})
			},
			code: http.StatusConflict,
		},
		{
			name: "redirect is not a loopback callback",
			change: func(t *testing.T, f *profileGrantFixture) {
				t.Helper()
				require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("state <> ''").Update("redirect_url", "http://attacker.example:8080/callback").Error)
			},
			code: http.StatusForbidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newProfileGrantFixture(t)
			f.setRemote(t, &types.RemoteCatalogConfig{FixedURL: f.provider.mcpURL(), LocalhostCallbackEnabled: true})
			_, state, err := f.startRelay(t, administrator("1"), profileGrantEight)
			require.NoError(t, err)
			tt.change(t, f)

			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, tt.code)
			_, _, _, ok := f.storedGrant(t, profileGrantEight)
			require.False(t, ok)
			require.Zero(t, f.provider.exchanges.Load())
		})
	}

	// Profile-configured URLs go through the relay too.
	f := newProfileGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1", LocalhostCallbackEnabled: true})
	f.configure(t, profileGrantSeven, map[string]*string{"__url": new(f.provider.mcpURL())})
	authorization, state, err := f.startRelay(t, administrator("1"), profileGrantSeven)
	require.NoError(t, err)
	require.True(t, authorization.LocalhostCallback)
	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, f.provider.mcpURL(), pending.URL)
	recorder, err := f.callback(administrator("1"), state)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	_, _, target, ok := f.storedGrant(t, profileGrantSeven)
	require.True(t, ok)
	require.Equal(t, f.provider.mcpURL(), target)
}
