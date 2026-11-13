package oauth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	profileGrantVMCP     = "vmcp1profilegrants"
	profileGrantAgents   = "vp1agents"
	profileGrantSeven    = "vp1seven"
	profileGrantUsers    = "vp1users"
	profileGrantRemote   = "remote"
	profileGrantLocal    = "local"
	profileGrantAccess   = "provider-access-token"
	profileGrantRefresh  = "provider-refresh-token"
	profileGrantCodeSent = "authorization-code"
)

// profileGrantProvider is a remote MCP server that requires OAuth, with its own authorization server that registers
// clients dynamically and exchanges codes.
type profileGrantProvider struct {
	server     *httptest.Server
	operations atomic.Int32
	exchanges  atomic.Int32
}

// profileGrantFixture is a shared vMCP with a remote component that requires OAuth and a local one, with a profile for
// agent 7, one for every agent and one for every user, and the handlers that authorize their grants.
type profileGrantFixture struct {
	provider *profileGrantProvider
	storage  kclient.WithWatch
	gateway  *gatewayclient.Client
	grants   *vmcpconfig.ProfileOAuthGrantStore
	factory  *MCPOAuthHandlerFactory
	handler  *handler
	key      kclient.ObjectKey
}

func newProfileGrantProvider(t *testing.T) *profileGrantProvider {
	t.Helper()
	p := &profileGrantProvider{}
	var base string
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp":
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
			require.Equal(t, profileGrantCodeSent, r.Form.Get("code"))
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
			profile(profileGrantAgents, types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
			profile(profileGrantUsers, types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}),
		},
	}}}
	storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1.VMCP{}).WithObjects(vmcp).Build()
	gateway := vmcpConsentGateway(t)
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
	authURL, err := f.factory.StartProfileOAuthGrant(req, f.key.Name, profileID, componentID)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(authURL)
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
	state, err := f.start(t, administrator("1"), profileGrantAgents, profileGrantRemote)
	require.NoError(t, err)
	require.Zero(t, f.provider.operations.Load(), "an authorization ran an MCP operation")

	pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, principal.ProfileGrant(f.key.Name, profileGrantAgents), pending.UserID)
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
	access, authorizedBy, target, ok := f.storedGrant(t, profileGrantAgents)
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
	status, err := f.grants.ProfileOAuthStatus(t.Context(), stored, profileGrantAgents)
	require.NoError(t, err)
	require.Equal(t, types.VMCPProfileOAuthAuthorized, status[profileGrantRemote].State)
	require.Equal(t, "1", status[profileGrantRemote].AuthorizedBy)
	require.Equal(t, f.provider.mcpURL(), status[profileGrantRemote].Target)
	statusJSON, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(statusJSON), profileGrantAccess)
	require.NotContains(t, string(statusJSON), profileGrantRefresh)

	// The attempt is used up.
	_, err = f.callback(administrator("1"), state)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.EqualValues(t, 1, f.provider.exchanges.Load())
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
			state, err := f.start(t, administrator("1"), profileGrantAgents, profileGrantRemote)
			require.NoError(t, err)
			var vmcp v1.VMCP
			require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
			tt.change(&vmcp)
			require.NoError(t, f.storage.Update(t.Context(), &vmcp))

			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, tt.code)
			_, _, _, ok := f.storedGrant(t, profileGrantAgents)
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
		{name: "profile for users", caller: administrator("1"), profile: profileGrantUsers, component: profileGrantRemote, code: http.StatusBadRequest},
		{name: "unknown profile", caller: administrator("1"), profile: "vp1missing", component: profileGrantRemote, code: http.StatusNotFound},
		{name: "component without OAuth", caller: administrator("1"), profile: profileGrantAgents, component: profileGrantLocal, code: http.StatusBadRequest},
		{name: "unknown component", caller: administrator("1"), profile: profileGrantAgents, component: "missing", code: http.StatusNotFound},
		{name: "not an administrator", caller: administrator("1", types.GroupBasic, types.GroupAuthenticated), profile: profileGrantAgents, component: profileGrantRemote, code: http.StatusForbidden},
		{name: "agent", caller: agent, profile: profileGrantAgents, component: profileGrantRemote, code: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newProfileGrantFixture(t)
			_, err := f.start(t, tt.caller, tt.profile, tt.component)
			requireErrHTTP(t, err, tt.code)
		})
	}

	// A component whose URL comes from configuration that no profile of an agent using the grant sets.
	f := newProfileGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1"})
	_, err := f.start(t, administrator("1"), profileGrantAgents, profileGrantRemote)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.True(t, strings.Contains(err.Error(), "configure the profile first"), err.Error())
	require.True(t, strings.Contains(err.Error(), "__url"), err.Error())

	// A component that authorizes through a localhost callback stays refused.
	f.setRemote(t, &types.RemoteCatalogConfig{FixedURL: f.provider.mcpURL(), LocalhostCallbackEnabled: true})
	_, err = f.start(t, administrator("1"), profileGrantAgents, profileGrantRemote)
	requireErrHTTP(t, err, http.StatusBadRequest)
	require.True(t, strings.Contains(err.Error(), "localhost callback"), err.Error())
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
// configuration, a hostname-constrained one and a URL template: each grant is bound to the URL that an agent using the
// profile's grants reaches, its own profile's or, for a value it does not set, the wildcard profile's, and an agent
// that reaches it is ready. A grant for a URL that changed since, before or after the callback, is refused.
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

			// The wildcard profile configures it: agent 7's explicit profile falls back to it.
			f.configure(t, profileGrantAgents, tt.value(base, "/mcp"))
			for _, profileID := range []string{profileGrantSeven, profileGrantAgents} {
				state, err := f.start(t, administrator("1"), profileID, profileGrantRemote)
				require.NoError(t, err)
				pending, err := f.gateway.GetMCPOAuthPendingState(t.Context(), state)
				require.NoError(t, err)
				require.Equal(t, base+"/mcp", pending.URL)
				_, err = f.callback(administrator("1"), state)
				require.NoError(t, err)
				_, _, target, ok := f.storedGrant(t, profileID)
				require.True(t, ok)
				require.Equal(t, base+"/mcp", target)
			}

			// An agent of either profile reaches the URL the grant is for, and is ready.
			var stored v1.VMCP
			require.NoError(t, f.storage.Get(t.Context(), f.key, &stored))
			stored.Spec.Manifest.Components = stored.Spec.Manifest.Components[:1]
			checks := vmcpconfig.AgentReadinessChecks{OAuth: vmcpconfig.ProfileOAuthGrantCheck{Grants: f.grants}}
			for _, agentID := range []uint{7, 8} {
				readiness, err := checks.AgentReadiness(t.Context(), stored, v1.VMCPInstance{}, profileGrantAgent(agentID))
				require.NoError(t, err)
				require.Empty(t, readiness.Reasons, "agent %d", agentID)
			}

			// The wildcard's URL changes while an authorization of its grant waits for its callback: the callback is
			// refused, and its attempt consumed.
			state, err := f.start(t, administrator("1"), profileGrantAgents, profileGrantRemote)
			require.NoError(t, err)
			f.configure(t, profileGrantAgents, tt.value(base, "/moved"))
			_, err = f.callback(administrator("1"), state)
			requireErrHTTP(t, err, http.StatusConflict)
			_, err = f.gateway.GetMCPOAuthPendingState(t.Context(), state)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)

			// Both grants were authorized for the old URL, which neither profile's agents reach any more.
			require.NoError(t, f.storage.Get(t.Context(), f.key, &stored))
			for _, profileID := range []string{profileGrantSeven, profileGrantAgents} {
				status, err := f.grants.ProfileOAuthStatus(t.Context(), stored, profileID)
				require.NoError(t, err)
				require.Equal(t, types.VMCPProfileOAuthInvalid, status[profileGrantRemote].State, profileID)
			}
		})
	}

	// An ambiguous fallback is a conflict, as for the vMCP's other ambiguous cases.
	f := newProfileGrantFixture(t)
	f.setRemote(t, &types.RemoteCatalogConfig{Hostname: "127.0.0.1"})
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, types.VMCPProfile{
		ID:          "vp1everyone",
		Name:        "everyone",
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	})
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	_, err := f.start(t, administrator("1"), profileGrantSeven, profileGrantRemote)
	requireErrHTTP(t, err, http.StatusConflict)
}

// profileGrantAgent returns the principal of the agent with id.
func profileGrantAgent(id uint) kuser.Info {
	extra := map[string][]string{}
	principal.RecordKind(extra, principal.KindAgent)
	uid := principal.Agent(id).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Groups: []string{types.GroupMCP, types.GroupAuthenticated}, Extra: extra}
}
