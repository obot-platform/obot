package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	readinessVMCP = "vmcp1readiness"
	// readinessSecretValue and readinessAccessToken are values that no readiness or agent setup response may contain.
	readinessSecretValue = "secret-region-value-8d1f"
	readinessAccessToken = "secret-access-token-3c2a"
	readinessRemoteURL   = "https://remote.example.com/mcp"
)

// readinessFixture serves the readiness routes of agents and the agent setup routes of vMCPs, as pkg/api/router
// registers them, over a gateway database and a vMCP store like the server's. The readiness checks are those of a
// session manager that reads the vMCPs' agent OAuth grants, as the server configures it.
type readinessFixture struct {
	t       *testing.T
	handler http.Handler
	gateway *gateway.Client
	storage *vmcpTestStorage
	grants  *vmcpconfig.AgentOAuthGrantStore
	manager *mcp.SessionManager
	// agent and otherAgent are the agents that profiles vp1agent and vp1other name, and stranger an agent that no
	// profile names.
	agent      *gatewaytypes.Agent
	otherAgent *gatewaytypes.Agent
	stranger   *gatewaytypes.Agent
}

// readinessTestVMCP returns a vMCP whose component "api" needs a region a user would enter, and takes an optional value
// a user would enter, and whose component "remote" needs administrator static OAuth. Its profiles are for one agent, for
// another agent, and for every user, which never applies to an agent. Each grants every component.
func readinessTestVMCP(agentID, otherAgentID uint) *v1.VMCP {
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	return &v1.VMCP{Name: readinessVMCP, Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		DisplayName: "Readiness",
		Components: []types.VMCPComponent{
			{
				ID:                      "api",
				Name:                    "api",
				MCPServerCatalogEntryID: "entry-api",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{
					{Key: "REGION", Required: true},
					{Key: "VERBOSE"},
				}}},
				Configuration: []types.VMCPConfigurationPolicy{
					{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
					{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed},
				},
			},
			{
				ID:                      "remote",
				Name:                    "remote",
				MCPServerCatalogEntryID: "entry-remote",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: readinessRemoteURL, StaticOAuthRequired: true},
				}},
			},
		},
		Profiles: []types.VMCPProfile{
			profile("vp1agent", agentSubject(agentID)),
			profile("vp1other", agentSubject(otherAgentID)),
			profile("vp1users", types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
		},
	}}}
}

func agentSubject(agentID uint) types.Subject {
	return types.Subject{Type: types.SubjectTypeAgent, ID: strconv.FormatUint(uint64(agentID), 10)}
}

func newReadinessFixture(t *testing.T) *readinessFixture {
	t.Helper()
	gatewayClient := newHandlerTestGateway(t)
	agent, err := gatewayClient.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "builder"})
	require.NoError(t, err)
	otherAgent, err := gatewayClient.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "runner"})
	require.NoError(t, err)
	stranger, err := gatewayClient.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "stranger"})
	require.NoError(t, err)

	vmcp := readinessTestVMCP(agent.ID, otherAgent.ID)
	storage := newVMCPStatusTestStorage(vmcp)
	grants := vmcpconfig.NewAgentOAuthGrantStore(gatewayClient)
	// The connect path's store of grants, as the server configures it.
	manager := &mcp.SessionManager{}
	manager.SetAgentOAuthGrants(grants)

	f := &readinessFixture{
		t:          t,
		gateway:    gatewayClient,
		storage:    storage,
		grants:     grants,
		manager:    manager,
		agent:      agent,
		otherAgent: otherAgent,
		stranger:   stranger,
	}
	f.serve(NewAgentIdentityHandler(manager), NewVMCPAgentOAuthHandler(&recordingAuthorizer{}, grants))
	return f
}

func (f *readinessFixture) serve(agents *AgentIdentityHandler, setup *VMCPAgentOAuthHandler) {
	mux := http.NewServeMux()
	register := func(pattern string, handle api.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			err := handle(api.Context{
				ResponseWriter: w,
				Request:        r,
				GatewayClient:  f.gateway,
				Storage:        f.storage,
				User:           vmcpAdminForTest(),
			})
			// The server writes the error of a handler like this.
			if httpErr, ok := errorsAsHTTP(err); ok {
				http.Error(w, httpErr.Message, httpErr.Code)
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	}
	agents.RegisterRoutes(register)
	setup.RegisterRoutes(register)
	f.handler = mux
}

func errorsAsHTTP(err error) (*types.ErrHTTP, bool) {
	httpErr, ok := err.(*types.ErrHTTP)
	return httpErr, ok
}

// do makes a request that must answer with status, and returns its body, decoded into out if given.
func (f *readinessFixture) do(method string, status int, path string, out any) string {
	f.t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	require.Equal(f.t, status, rec.Code, "%s %s: %s", method, path, rec.Body.String())
	// Nothing a readiness or agent setup response says is a value an administrator entered or a token.
	require.NotContains(f.t, rec.Body.String(), readinessSecretValue)
	require.NotContains(f.t, rec.Body.String(), readinessAccessToken)
	if out != nil {
		require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), out), rec.Body.String())
	}
	return rec.Body.String()
}

func (f *readinessFixture) get(status int, path string, out any) string {
	f.t.Helper()
	return f.do(http.MethodGet, status, path, out)
}

func (f *readinessFixture) readiness(agent *gatewaytypes.Agent) types.AgentVMCPReadiness {
	f.t.Helper()
	var readiness types.AgentVMCPReadiness
	f.get(http.StatusOK, agentPath(agent, "/vmcps/"+readinessVMCP+"/readiness"), &readiness)
	return readiness
}

func (f *readinessFixture) setup() types.VMCPAgentSetup {
	f.t.Helper()
	var setup types.VMCPAgentSetup
	f.get(http.StatusOK, "/api/vmcps/"+readinessVMCP+"/agent-setup", &setup)
	return setup
}

// revoke revokes the vMCP's grant for the component through the API, as an administrator does.
func (f *readinessFixture) revoke(componentID string) types.VMCPAgentSetup {
	f.t.Helper()
	var setup types.VMCPAgentSetup
	f.do(http.MethodDelete, http.StatusOK, "/api/vmcps/"+readinessVMCP+"/agent-oauth/"+componentID, &setup)
	return setup
}

// fixRegion fixes the region of component "api" on the vMCP, for everyone, agents included, as an administrator does
// when agents need it.
func (f *readinessFixture) fixRegion() {
	f.t.Helper()
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		manifest.Components[0].Configuration[0].Policy = types.VMCPConfigurationPolicyFixed
	})
	require.NoError(f.t, f.gateway.UpsertCredential(f.t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(readinessVMCP),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey("api", "REGION"): readinessSecretValue},
	}))
}

// storeGrant stores the vMCP's grant for component "remote", as the callback of an administrator's authorization does.
func (f *readinessFixture) storeGrant(url, grantID string) {
	f.t.Helper()
	require.NoError(f.t, f.gateway.ReplaceMCPOAuthGrant(f.t.Context(), principal.AgentOAuthGrantOwner(readinessVMCP), vmcpconfig.AgentOAuthGrantMCPID(readinessVMCP, "remote"), url, grantID, "42",
		&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: readinessAccessToken, Expiry: time.Now().Add(time.Hour)}))
}

func (f *readinessFixture) updateVMCP(change func(*types.VMCPManifest)) {
	f.t.Helper()
	var stored v1.VMCP
	require.NoError(f.t, f.storage.Get(f.t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: readinessVMCP}, &stored))
	change(&stored.Spec.Manifest)
	require.NoError(f.t, f.storage.Update(f.t.Context(), &stored))
}

func agentPath(agent *gatewaytypes.Agent, rest string) string {
	return "/api/agents/" + strconv.FormatUint(uint64(agent.ID), 10) + rest
}

// TestAgentReadinessAPI follows two agents, each named by its own profile, through setup on the vMCP: a value that a
// user would enter blocks both until the vMCP fixes it, the one grant the vMCP holds for a component serves both, a
// grant that is invalidated in the database is seen at once by both, revoking it makes both need OAuth again, and
// removing an agent from its profile takes the vMCP away from it. No reason names a profile: every fix is on the vMCP.
func TestAgentReadinessAPI(t *testing.T) {
	f := newReadinessFixture(t)
	otherAgent, agent := f.otherAgent, f.agent
	notSetUp := []types.AgentReadinessReason{
		{ComponentID: "api", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"},
		{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired},
	}

	// Nothing is set up, for either agent.
	for _, a := range []*gatewaytypes.Agent{agent, otherAgent} {
		require.Equal(t, types.AgentVMCPReadiness{
			VMCPID:          readinessVMCP,
			VMCPDisplayName: "Readiness",
			Reasons:         notSetUp,
		}, f.readiness(a))
	}

	// One grant on the vMCP serves both agents; the region still blocks them: nothing an agent can be given supplies it.
	f.storeGrant(readinessRemoteURL, "grant-1")
	for _, a := range []*gatewaytypes.Agent{agent, otherAgent} {
		require.Equal(t, notSetUp[:1], f.readiness(a).Reasons)
	}

	// Fixing the region on the vMCP makes both ready. The optional value stays unset, and blocks no one.
	f.fixRegion()
	for _, a := range []*gatewaytypes.Agent{agent, otherAgent} {
		require.Equal(t, types.AgentVMCPReadiness{
			VMCPID:          readinessVMCP,
			VMCPDisplayName: "Readiness",
			Ready:           true,
			Reasons:         []types.AgentReadinessReason{},
		}, f.readiness(a))
	}

	// Invalidating the grant changes only its database row, and the next request sees it for every agent that uses it:
	// nothing is cached.
	var marked bool
	require.NoError(t, f.gateway.WithMCPOAuthRefreshLock(t.Context(), principal.AgentOAuthGrantOwner(readinessVMCP), vmcpconfig.AgentOAuthGrantMCPID(readinessVMCP, "remote"), func(_ context.Context, refresh *gateway.MCPOAuthRefresh) error {
		var err error
		marked, err = refresh.Invalidate("grant-1", "the provider refused the refresh token")
		return err
	}))
	require.True(t, marked)
	for _, a := range []*gatewaytypes.Agent{agent, otherAgent} {
		require.Equal(t, []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthInvalid}}, f.readiness(a).Reasons)
	}

	// Authorizing again replaces it, and revoking it makes every agent granted the component need OAuth again.
	f.storeGrant(readinessRemoteURL, "grant-2")
	require.True(t, f.readiness(agent).Ready)
	f.revoke("remote")
	for _, a := range []*gatewaytypes.Agent{agent, otherAgent} {
		require.Equal(t, []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired}}, f.readiness(a).Reasons)
	}

	// Removing the agent from its profile takes the vMCP away from it: the profile for every user never applies to an
	// agent.
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		manifest.Profiles[0].Subjects = []types.Subject{{Type: types.SubjectTypeAgent, ID: "999"}}
	})
	f.get(http.StatusNotFound, agentPath(agent, "/vmcps/"+readinessVMCP+"/readiness"), nil)
	var list types.AgentVMCPReadinessList
	f.get(http.StatusOK, agentPath(agent, "/vmcps"), &list)
	require.Empty(t, list.Items)
}

// TestAgentReadinessIsTheUnionOfProfiles checks an agent named by two profiles of a vMCP, each granting one component:
// its readiness covers both components, as a user's grants would, and only those; a component that only another
// agent's profile grants does not count for it; and removing it from one profile takes away only that profile's
// component.
func TestAgentReadinessIsTheUnionOfProfiles(t *testing.T) {
	f := newReadinessFixture(t)
	only := func(componentID string) types.VMCPProfilePermissions {
		return types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{componentID: {}}}
	}
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		manifest.Profiles = []types.VMCPProfile{
			{ID: "vp1api", Name: "api", Subjects: []types.Subject{agentSubject(f.agent.ID)}, Permissions: only("api")},
			{ID: "vp1remote", Name: "remote", Subjects: []types.Subject{agentSubject(f.agent.ID), agentSubject(f.otherAgent.ID)}, Permissions: only("remote")},
		}
	})
	apiReason := types.AgentReadinessReason{ComponentID: "api", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"}
	remoteReason := types.AgentReadinessReason{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired}

	require.Equal(t, []types.AgentReadinessReason{apiReason, remoteReason}, f.readiness(f.agent).Reasons)
	// The other agent is granted only "remote", so the region "api" needs keeps nothing from it.
	require.Equal(t, []types.AgentReadinessReason{remoteReason}, f.readiness(f.otherAgent).Reasons)
	f.storeGrant(readinessRemoteURL, "grant-1")
	require.True(t, f.readiness(f.otherAgent).Ready)
	require.Equal(t, []types.AgentReadinessReason{apiReason}, f.readiness(f.agent).Reasons)

	// The access summary lists both profiles that name the agent.
	var access types.AgentIdentityAccess
	f.get(http.StatusOK, agentPath(f.agent, "/access"), &access)
	var profileIDs []string
	for _, profile := range access.VMCPProfiles {
		profileIDs = append(profileIDs, profile.ProfileID)
	}
	require.Equal(t, []string{"vp1api", "vp1remote"}, profileIDs)

	// Removing the agent from the profile that grants "remote" leaves it "api" alone.
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		manifest.Profiles[1].Subjects = []types.Subject{agentSubject(f.otherAgent.ID)}
	})
	require.Equal(t, []types.AgentReadinessReason{apiReason}, f.readiness(f.agent).Reasons)
	f.fixRegion()
	require.True(t, f.readiness(f.agent).Ready)
	require.True(t, f.readiness(f.otherAgent).Ready)
}

// TestAgentReadinessUsesTheConnectPathChecks checks that the API is computed as the connect path computes it, with the
// store of grants the session manager is configured with: a stored grant makes the agent ready, and a session manager
// without the store reports no readiness at all rather than one computed without grants.
func TestAgentReadinessUsesTheConnectPathChecks(t *testing.T) {
	f := newReadinessFixture(t)
	f.fixRegion()
	f.storeGrant(readinessRemoteURL, "grant-1")
	require.True(t, f.readiness(f.agent).Ready)

	f.serve(NewAgentIdentityHandler(&mcp.SessionManager{}), NewVMCPAgentOAuthHandler(&recordingAuthorizer{}, f.grants))
	f.get(http.StatusInternalServerError, agentPath(f.agent, "/vmcps/"+readinessVMCP+"/readiness"), nil)

	// The report is exactly the readiness the session manager computes for a connect.
	f.serve(NewAgentIdentityHandler(f.manager), NewVMCPAgentOAuthHandler(&recordingAuthorizer{}, f.grants))
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: readinessVMCP}, &vmcp))
	want, err := f.manager.AgentReadiness(t.Context(), vmcp, nil, gateway.AgentUserInfo(f.agent, nil, nil))
	require.NoError(t, err)
	require.Equal(t, want.Readiness.Ready(), f.readiness(f.agent).Ready)
}

// TestAgentReadinessListAndLifecycle checks the list of the vMCPs an agent can access, and that it agrees with the
// agent's access summary, that a disabled agent reports its readiness, and that a deleted one is 404.
func TestAgentReadinessListAndLifecycle(t *testing.T) {
	f := newReadinessFixture(t)
	f.fixRegion()
	everyone := types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}
	named := agentSubject(f.agent.ID)
	someoneElse := types.Subject{Type: types.SubjectTypeAgent, ID: "999"}
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	for _, other := range []*v1.VMCP{
		// Shared, naming the agent, with nothing to set up.
		{Name: "vmcp1named", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Named", Profiles: []types.VMCPProfile{profile("vp1named", named)}}}},
		// Shared and open to every user, which grants no agent.
		{Name: "vmcp1open", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Open", Profiles: []types.VMCPProfile{profile("vp1open", everyone)}}}},
		// No profile names the agent.
		{Name: "vmcp1closed", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Closed", Profiles: []types.VMCPProfile{profile("vp1closed", someoneElse)}}}},
		// Personal vMCPs grant only their owner.
		{Name: "vmcp1personal", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{UserID: "1", Manifest: types.VMCPManifest{DisplayName: "Personal", Profiles: []types.VMCPProfile{profile("vp1personal", named)}}}},
	} {
		require.NoError(t, f.storage.Create(t.Context(), other))
	}

	var list types.AgentVMCPReadinessList
	f.get(http.StatusOK, agentPath(f.agent, "/vmcps"), &list)
	require.Len(t, list.Items, 2)
	require.Equal(t, "vmcp1named", list.Items[0].VMCPID)
	require.True(t, list.Items[0].Ready)
	require.Equal(t, readinessVMCP, list.Items[1].VMCPID)
	require.False(t, list.Items[1].Ready, "the remote component has no grant")
	require.Equal(t, []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired}}, list.Items[1].Reasons)

	// It lists the vMCPs whose profiles the access summary lists.
	var access types.AgentIdentityAccess
	f.get(http.StatusOK, agentPath(f.agent, "/access"), &access)
	var accessible []string
	for _, profile := range access.VMCPProfiles {
		if len(accessible) == 0 || accessible[len(accessible)-1] != profile.VMCPID {
			accessible = append(accessible, profile.VMCPID)
		}
	}
	require.Equal(t, []string{list.Items[0].VMCPID, list.Items[1].VMCPID}, accessible)

	// A vMCP open to every user, one the agent has no access to, a personal one, and an unknown one are not found.
	f.get(http.StatusNotFound, agentPath(f.agent, "/vmcps/vmcp1open/readiness"), nil)
	f.get(http.StatusNotFound, agentPath(f.agent, "/vmcps/vmcp1closed/readiness"), nil)
	f.get(http.StatusNotFound, agentPath(f.agent, "/vmcps/vmcp1personal/readiness"), nil)
	f.get(http.StatusNotFound, agentPath(f.agent, "/vmcps/vmcp1unknown/readiness"), nil)

	// A key that can use the LLM gateway and skills is a problem the access summary reports while no policy names the
	// agent, even when a policy and a rule name every user: those grant no agent.
	for _, policy := range []kclient.Object{
		&v1.ModelAccessPolicy{
			Name:      "map1everyone",
			Namespace: system.DefaultNamespace,
			Spec:      v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{DisplayName: "Everyone", Subjects: []types.Subject{everyone}}},
		},
		&v1.SkillAccessRule{
			Name:      "sar1everyone",
			Namespace: system.DefaultNamespace,
			Spec:      v1.SkillAccessRuleSpec{Manifest: types.SkillAccessRuleManifest{DisplayName: "Everyone", Subjects: []types.Subject{everyone}}},
		},
	} {
		require.NoError(t, f.storage.Create(t.Context(), policy))
	}
	_, err := f.gateway.CreateAgentAPIKey(t.Context(), f.agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "key", CanAccessLLMProxy: true, CanAccessSkills: true})
	require.NoError(t, err)
	f.get(http.StatusOK, agentPath(f.agent, "/access"), &access)
	require.Empty(t, access.ModelAccessPolicies)
	require.Empty(t, access.SkillAccessRules)
	require.Equal(t, []types.AgentIdentityAccessProblem{
		{Code: types.AgentProblemNoModelAccessPolicy},
		{Code: types.AgentProblemNoSkillAccessRule},
	}, access.Problems)

	// A disabled agent reports its readiness, and that it is disabled.
	_, err = f.gateway.DisableAgent(t.Context(), f.agent.ID, "paused")
	require.NoError(t, err)
	disabled := f.readiness(f.agent)
	require.True(t, disabled.Disabled)
	require.Len(t, disabled.Reasons, 1)
	f.get(http.StatusOK, agentPath(f.agent, "/vmcps"), &list)
	require.True(t, list.Items[0].Disabled)

	// A deleted agent, and one that does not exist, are not found.
	require.NoError(t, f.gateway.DeleteAgent(t.Context(), f.otherAgent.ID))
	f.get(http.StatusNotFound, agentPath(f.otherAgent, "/vmcps"), nil)
	f.get(http.StatusNotFound, agentPath(f.otherAgent, "/vmcps/"+readinessVMCP+"/readiness"), nil)
	f.get(http.StatusNotFound, "/api/agents/999/vmcps", nil)
	f.get(http.StatusNotFound, "/api/agents/nope/vmcps/"+readinessVMCP+"/readiness", nil)
}

// TestVMCPAgentSetupAPI checks a vMCP's agent setup: it covers the components that profiles naming agents grant, names
// the values agents need and cannot be given and the OAuth each component needs, reports the vMCP's grant with its
// metadata and no value or token, and agrees with every agent's readiness: an agent's reasons are those of the
// components it is granted. An unknown vMCP is 404.
func TestVMCPAgentSetupAPI(t *testing.T) {
	f := newReadinessFixture(t)
	requireAgrees := func() {
		t.Helper()
		setup := f.setup()
		for _, agent := range []*gatewaytypes.Agent{f.agent, f.otherAgent} {
			var vmcp v1.VMCP
			require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: readinessVMCP}, &vmcp))
			granted := vmcpconfig.EnabledComponents(gateway.AgentUserInfo(agent, nil, nil), vmcp, vmcpconfig.ComponentsForInstance(vmcp, v1.VMCPInstance{}))
			want := []types.AgentReadinessReason{}
			for _, component := range setup.Components {
				if slices.ContainsFunc(granted, func(c types.VMCPComponent) bool { return c.ID == component.ComponentID }) {
					want = append(want, component.Reasons...)
				}
			}
			readiness := f.readiness(agent)
			require.Equal(t, want, readiness.Reasons, "agent %d", agent.ID)
			require.Equal(t, len(want) == 0, readiness.Ready)
		}
	}

	require.Equal(t, types.VMCPAgentSetup{
		VMCPID: readinessVMCP,
		Components: []types.VMCPAgentSetupComponent{
			{
				ComponentID: "api",
				Reasons:     []types.AgentReadinessReason{{ComponentID: "api", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"}},
			},
			{
				ComponentID: "remote",
				Reasons:     []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired}},
				OAuth:       &types.VMCPAgentOAuthStatus{Required: true, State: types.VMCPAgentOAuthMissing},
			},
		},
	}, f.setup())
	requireAgrees()

	// Once the vMCP fixes the region and holds a grant, both components are ready.
	f.fixRegion()
	f.storeGrant(readinessRemoteURL, "grant-1")
	setup := f.setup()
	require.True(t, setup.Components[0].Ready)
	require.True(t, setup.Components[1].Ready)
	oauth := setup.Components[1].OAuth
	require.Equal(t, types.VMCPAgentOAuthAuthorized, oauth.State)
	require.Equal(t, readinessRemoteURL, oauth.Target)
	require.Equal(t, "42", oauth.AuthorizedBy)
	require.NotNil(t, oauth.AuthorizedAt)
	requireAgrees()

	// A grant that stopped working is reported as invalid as soon as it does.
	require.NoError(t, f.gateway.WithMCPOAuthRefreshLock(t.Context(), principal.AgentOAuthGrantOwner(readinessVMCP), vmcpconfig.AgentOAuthGrantMCPID(readinessVMCP, "remote"), func(_ context.Context, refresh *gateway.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-1", "refused")
		return err
	}))
	oauth = f.setup().Components[1].OAuth
	require.Equal(t, types.VMCPAgentOAuthInvalid, oauth.State)
	require.Equal(t, "refused", oauth.InvalidReason)
	requireAgrees()

	// A component that no profile naming an agent grants is not part of the setup, even if a profile for users grants it.
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		for i := range manifest.Profiles[:2] {
			manifest.Profiles[i].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"api": {}}}
		}
	})
	setup = f.setup()
	require.Len(t, setup.Components, 1)
	require.Equal(t, "api", setup.Components[0].ComponentID)
	requireAgrees()

	f.get(http.StatusNotFound, "/api/vmcps/vmcp1missing/agent-setup", nil)
}

// TestAgentAccessProblems checks that the access summary reports a key scope that no policy grants anything for.
func TestAgentAccessProblems(t *testing.T) {
	now := time.Now()
	expired := now.Add(-time.Hour)
	revoked := now.Add(-time.Minute)
	withPolicies := types.AgentIdentityAccess{
		ModelAccessPolicies: []types.AgentIdentityPolicyAccess{{ID: "map1"}},
		SkillAccessRules:    []types.AgentIdentityPolicyAccess{{ID: "sar1"}},
	}
	for _, tt := range []struct {
		name   string
		keys   []gatewaytypes.APIKey
		access types.AgentIdentityAccess
		want   []types.AgentIdentityAccessProblem
	}{
		{
			name:   "no keys",
			access: types.AgentIdentityAccess{},
			want:   []types.AgentIdentityAccessProblem{},
		},
		{
			name:   "a key for the LLM gateway and skills with no policy",
			keys:   []gatewaytypes.APIKey{{CanAccessLLMProxy: true, CanAccessSkills: true}},
			access: types.AgentIdentityAccess{},
			want: []types.AgentIdentityAccessProblem{
				{Code: types.AgentProblemNoModelAccessPolicy},
				{Code: types.AgentProblemNoSkillAccessRule},
			},
		},
		{
			name:   "a key for skills only",
			keys:   []gatewaytypes.APIKey{{CanAccessSkills: true}},
			access: types.AgentIdentityAccess{},
			want:   []types.AgentIdentityAccessProblem{{Code: types.AgentProblemNoSkillAccessRule}},
		},
		{
			name:   "policies that name the agent",
			keys:   []gatewaytypes.APIKey{{CanAccessLLMProxy: true, CanAccessSkills: true}},
			access: withPolicies,
			want:   []types.AgentIdentityAccessProblem{},
		},
		{
			name: "keys that are expired or revoked",
			keys: []gatewaytypes.APIKey{
				{CanAccessLLMProxy: true, ExpiresAt: &expired},
				{CanAccessSkills: true, RevokedAt: &revoked},
			},
			access: types.AgentIdentityAccess{},
			want:   []types.AgentIdentityAccessProblem{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, accessProblems(tt.keys, tt.access, now))
		})
	}
}

// TestAgentSetupNeverReturnsConfiguredURLs checks what a vMCP's agent setup says of the URL a grant was authorized for.
// A component whose catalog entry fixes the URL returns it as target: it is catalog data. A component whose URL comes
// from configuration, a hostname-constrained URL or a URL template, returns only the hostname as targetHostname, and the
// URL, with its port, userinfo, path and query, is nowhere in the raw response: not in the setup, not in the response of
// a revocation, and not in what readiness reports.
func TestAgentSetupNeverReturnsConfiguredURLs(t *testing.T) {
	const (
		hostnameURL = "https://user:pw-secret-userinfo@tenant.example.com:8443/secret-path-77/mcp?token=secret-query-token"
		templateURL = "https://acme-secret-tenant.example.org/secret-template-path?key=secret-template-key"
	)
	secrets := []string{"secret-path-77", "secret-query-token", "pw-secret-userinfo", "8443", "secret-template-path", "secret-template-key", "https://tenant.example.com", "https://acme-secret-tenant.example.org"}
	remote := func(id string, config types.RemoteCatalogConfig) types.VMCPComponent {
		config.StaticOAuthRequired = true
		return types.VMCPComponent{
			ID:                      id,
			Name:                    id,
			MCPServerCatalogEntryID: "entry-" + id,
			CatalogEntry:            types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeRemote, RemoteConfig: &config}},
		}
	}
	f := newReadinessFixture(t)
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		manifest.Components = []types.VMCPComponent{
			remote("fixed", types.RemoteCatalogConfig{FixedURL: readinessRemoteURL}),
			remote("hostname", types.RemoteCatalogConfig{Hostname: "tenant.example.com"}),
			remote("template", types.RemoteCatalogConfig{URLTemplate: "https://${TENANT}.example.org/secret-template-path"}),
		}
	})
	for component, url := range map[string]string{"fixed": readinessRemoteURL, "hostname": hostnameURL, "template": templateURL} {
		require.NoError(t, f.gateway.ReplaceMCPOAuthGrant(t.Context(), principal.AgentOAuthGrantOwner(readinessVMCP), vmcpconfig.AgentOAuthGrantMCPID(readinessVMCP, component), url, "grant-"+component, "42",
			&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: readinessAccessToken, Expiry: time.Now().Add(time.Hour)}))
	}

	check := func(label, body string) {
		t.Helper()
		for _, secret := range secrets {
			require.NotContains(t, body, secret, "%s: %s", label, body)
		}
	}
	byComponent := func(setup types.VMCPAgentSetup) map[string]*types.VMCPAgentOAuthStatus {
		result := map[string]*types.VMCPAgentOAuthStatus{}
		for _, component := range setup.Components {
			result[component.ComponentID] = component.OAuth
		}
		return result
	}

	var setup types.VMCPAgentSetup
	check("setup", f.get(http.StatusOK, "/api/vmcps/"+readinessVMCP+"/agent-setup", &setup))
	oauth := byComponent(setup)
	require.Equal(t, readinessRemoteURL, oauth["fixed"].Target)
	require.Empty(t, oauth["fixed"].TargetHostname)
	require.Empty(t, oauth["hostname"].Target)
	require.Equal(t, "tenant.example.com", oauth["hostname"].TargetHostname)
	require.Empty(t, oauth["template"].Target)
	require.Equal(t, "acme-secret-tenant.example.org", oauth["template"].TargetHostname)

	// The response of a revocation reports the grants of the vMCP's other components.
	setup = types.VMCPAgentSetup{}
	check("revoke", f.do(http.MethodDelete, http.StatusOK, "/api/vmcps/"+readinessVMCP+"/agent-oauth/fixed", &setup))
	oauth = byComponent(setup)
	require.Equal(t, types.VMCPAgentOAuthMissing, oauth["fixed"].State)
	require.Equal(t, "tenant.example.com", oauth["hostname"].TargetHostname)
	require.Empty(t, oauth["hostname"].Target)

	// Readiness, with its reasons, names components, never a URL.
	check("readiness", f.get(http.StatusOK, agentPath(f.agent, "/vmcps/"+readinessVMCP+"/readiness"), nil))
	check("list", f.get(http.StatusOK, agentPath(f.agent, "/vmcps"), nil))
}
