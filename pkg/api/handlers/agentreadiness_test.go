package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	readinessVMCP = "vmcp1readiness"
	// readinessSecretValue and readinessAccessToken are values that no readiness or status response may contain.
	readinessSecretValue = "secret-region-value-8d1f"
	readinessAccessToken = "secret-access-token-3c2a"
	readinessRemoteURL   = "https://remote.example.com/mcp"
)

// readinessFixture serves the readiness routes of agents and the status route of profiles, as pkg/api/router registers
// them, over a gateway database and a vMCP store like the server's. The readiness checks are those of a session manager
// that reads profile OAuth grants, as the server configures it.
type readinessFixture struct {
	t       *testing.T
	handler http.Handler
	gateway *gateway.Client
	storage *vmcpTestStorage
	grants  *vmcpconfig.ProfileOAuthGrantStore
	config  *VMCPProfileConfigurationHandler
	manager *mcp.SessionManager
	// agent is the agent with an explicit profile, and wildcardAgent the one with only the wildcard profile.
	agent         *gatewaytypes.Agent
	wildcardAgent *gatewaytypes.Agent
}

// readinessTestVMCP returns a vMCP whose component "api" needs a region and a token a user would enter, and whose
// component "remote" needs administrator static OAuth. Its profiles are for one agent, for every agent, and for every
// user.
func readinessTestVMCP(agentID uint) *v1.VMCP {
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
			profile("vp1agent", types.Subject{Type: types.SubjectTypeAgent, ID: strconv.FormatUint(uint64(agentID), 10)}),
			profile("vp1agents", types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
			profile("vp1users", types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}),
		},
	}}}
}

func newReadinessFixture(t *testing.T) *readinessFixture {
	t.Helper()
	gatewayClient := newHandlerTestGateway(t)
	agent, err := gatewayClient.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "builder"})
	require.NoError(t, err)
	wildcardAgent, err := gatewayClient.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "runner"})
	require.NoError(t, err)

	vmcp := readinessTestVMCP(agent.ID)
	storage := newProfileConfigurationTestStorage(vmcp)
	grants := vmcpconfig.NewProfileOAuthGrantStore(gatewayClient)
	// The connect path's store of grants, as the server configures it.
	manager := &mcp.SessionManager{}
	manager.SetProfileOAuthGrants(grants)

	f := &readinessFixture{
		t:             t,
		gateway:       gatewayClient,
		storage:       storage,
		grants:        grants,
		config:        NewVMCPProfileConfigurationHandler(true, true, vmcpconfig.NewProfileConfigurationStore(gatewayClient)),
		manager:       manager,
		agent:         agent,
		wildcardAgent: wildcardAgent,
	}
	f.serve(NewAgentIdentityHandler(true, true, manager), NewVMCPProfileOAuthHandler(true, true, &recordingAuthorizer{}, grants))
	return f
}

func (f *readinessFixture) serve(agents *AgentIdentityHandler, profiles *VMCPProfileOAuthHandler) {
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
	profiles.RegisterRoutes(register)
	f.handler = mux
}

func errorsAsHTTP(err error) (*types.ErrHTTP, bool) {
	httpErr, ok := err.(*types.ErrHTTP)
	return httpErr, ok
}

// get makes a request that must answer with status, and returns its body, decoded into out if given.
func (f *readinessFixture) get(status int, path string, out any) string {
	f.t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(f.t, status, rec.Code, "GET %s: %s", path, rec.Body.String())
	// Nothing a readiness or status response says is a value an administrator entered or a token.
	require.NotContains(f.t, rec.Body.String(), readinessSecretValue)
	require.NotContains(f.t, rec.Body.String(), readinessAccessToken)
	if out != nil {
		require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), out), rec.Body.String())
	}
	return rec.Body.String()
}

func (f *readinessFixture) readiness(agent *gatewaytypes.Agent) types.AgentVMCPReadiness {
	f.t.Helper()
	var readiness types.AgentVMCPReadiness
	f.get(http.StatusOK, "/api/agents/"+strconv.FormatUint(uint64(agent.ID), 10)+"/vmcps/"+readinessVMCP+"/readiness", &readiness)
	return readiness
}

func (f *readinessFixture) profileStatus(profileID string) types.VMCPProfileStatus {
	f.t.Helper()
	var status types.VMCPProfileStatus
	f.get(http.StatusOK, "/api/vmcps/"+readinessVMCP+"/profiles/"+profileID+"/status", &status)
	return status
}

// configure sets the configuration of a profile through the API that administrators use.
func (f *readinessFixture) configure(profileID string, values map[string]any) {
	f.t.Helper()
	_, err := callProfileConfiguration(f.t, f.config, f.storage, f.gateway, http.MethodPut, readinessVMCP, profileID, map[string]any{"components": map[string]any{"api": values}})
	require.NoError(f.t, err)
}

func (f *readinessFixture) storeGrant(profileID, url, grantID string) {
	f.t.Helper()
	require.NoError(f.t, f.gateway.ReplaceMCPOAuthGrant(f.t.Context(), principal.ProfileGrant(readinessVMCP, profileID), vmcpconfig.ProfileOAuthGrantMCPID(readinessVMCP, "remote"), url, grantID, "42",
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

// TestAgentReadinessAPI follows an agent and an agent with only the wildcard profile through setup: the reasons name
// the profile to fix, fallback is applied and reported as sources, a grant that is invalidated in the database is seen
// at once, and removing an explicit assignment changes where things come from instead of making the agent unready.
func TestAgentReadinessAPI(t *testing.T) {
	f := newReadinessFixture(t)
	wildcardAgent, agent := f.wildcardAgent, f.agent

	// Nothing is set up: each agent is told to fix the profile that applies to it.
	require.Equal(t, types.AgentVMCPReadiness{
		VMCPID:            readinessVMCP,
		VMCPDisplayName:   "Readiness",
		ExplicitProfileID: "vp1agent",
		WildcardProfileID: "vp1agents",
		Reasons: []types.AgentReadinessReason{
			{ComponentID: "api", Code: types.AgentReadinessConfigurationRequired, Field: "REGION", ProfileID: "vp1agent"},
			{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired, ProfileID: "vp1agent"},
		},
	}, f.readiness(agent))
	require.Equal(t, types.AgentVMCPReadiness{
		VMCPID:            readinessVMCP,
		VMCPDisplayName:   "Readiness",
		WildcardProfileID: "vp1agents",
		Reasons: []types.AgentReadinessReason{
			{ComponentID: "api", Code: types.AgentReadinessConfigurationRequired, Field: "REGION", ProfileID: "vp1agents"},
			{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired, ProfileID: "vp1agents"},
		},
	}, f.readiness(wildcardAgent))

	// The wildcard profile's setup makes the wildcard agent ready, and the other agent falls back to it.
	f.configure("vp1agents", map[string]any{"REGION": readinessSecretValue})
	f.storeGrant("vp1agents", readinessRemoteURL, "grant-agents")
	require.Equal(t, types.AgentVMCPReadiness{
		VMCPID:            readinessVMCP,
		VMCPDisplayName:   "Readiness",
		Ready:             true,
		WildcardProfileID: "vp1agents",
		Reasons:           []types.AgentReadinessReason{},
		// An agent with no explicit profile gets everything from the wildcard profile, which is its fallback.
		Sources: []types.AgentReadinessSource{
			{Kind: types.AgentSourceConfiguration, ComponentID: "api", Field: "REGION", ProfileID: "vp1agents", Fallback: true},
			{Kind: types.AgentSourceOAuth, ComponentID: "remote", ProfileID: "vp1agents", Fallback: true},
		},
	}, f.readiness(wildcardAgent))
	require.Equal(t, types.AgentVMCPReadiness{
		VMCPID:            readinessVMCP,
		VMCPDisplayName:   "Readiness",
		Ready:             true,
		ExplicitProfileID: "vp1agent",
		WildcardProfileID: "vp1agents",
		Reasons:           []types.AgentReadinessReason{},
		Sources: []types.AgentReadinessSource{
			{Kind: types.AgentSourceConfiguration, ComponentID: "api", Field: "REGION", ProfileID: "vp1agents", Fallback: true},
			{Kind: types.AgentSourceOAuth, ComponentID: "remote", ProfileID: "vp1agents", Fallback: true},
		},
	}, f.readiness(agent))

	// The explicit profile's own value and grant take over from the fallback.
	f.configure("vp1agent", map[string]any{"REGION": readinessSecretValue})
	f.storeGrant("vp1agent", readinessRemoteURL, "grant-agent")
	ready := f.readiness(agent)
	require.True(t, ready.Ready)
	require.Equal(t, []types.AgentReadinessSource{
		{Kind: types.AgentSourceConfiguration, ComponentID: "api", Field: "REGION", ProfileID: "vp1agent"},
		{Kind: types.AgentSourceOAuth, ComponentID: "remote", ProfileID: "vp1agent"},
	}, ready.Sources)

	// Invalidating a grant changes only its database row, and the next request sees it: nothing is cached. The agent is
	// not sent to the wildcard profile's working grant.
	var marked bool
	require.NoError(t, f.gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(readinessVMCP, "vp1agent"), vmcpconfig.ProfileOAuthGrantMCPID(readinessVMCP, "remote"), func(_ context.Context, refresh *gateway.MCPOAuthRefresh) error {
		var err error
		marked, err = refresh.Invalidate("grant-agent", "the provider refused the refresh token")
		return err
	}))
	require.True(t, marked)
	invalid := f.readiness(agent)
	require.False(t, invalid.Ready)
	require.Equal(t, []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthInvalid, ProfileID: "vp1agent"}}, invalid.Reasons)
	require.True(t, f.readiness(wildcardAgent).Ready)

	// Removing the explicit assignment leaves the agent on the wildcard profile: its sources change, it is not unready.
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		manifest.Profiles[0].Subjects = []types.Subject{{Type: types.SubjectTypeAgent, ID: "999"}}
	})
	removed := f.readiness(agent)
	require.True(t, removed.Ready, "%+v", removed.Reasons)
	require.Empty(t, removed.ExplicitProfileID)
	require.Equal(t, []types.AgentReadinessSource{
		{Kind: types.AgentSourceConfiguration, ComponentID: "api", Field: "REGION", ProfileID: "vp1agents", Fallback: true},
		{Kind: types.AgentSourceOAuth, ComponentID: "remote", ProfileID: "vp1agents", Fallback: true},
	}, removed.Sources)
}

// TestAgentReadinessAmbiguousSelection checks that more than one profile naming every agent blocks an agent that has
// only the wildcard on the whole vMCP, and an agent with an explicit profile only for the components that need the
// fallback.
func TestAgentReadinessAmbiguousSelection(t *testing.T) {
	f := newReadinessFixture(t)
	f.configure("vp1agent", map[string]any{"REGION": readinessSecretValue})
	f.storeGrant("vp1agent", readinessRemoteURL, "grant-agent")
	f.updateVMCP(func(manifest *types.VMCPManifest) {
		manifest.Profiles = append(manifest.Profiles, types.VMCPProfile{
			ID:          "vp1others",
			Name:        "others",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		})
	})

	blocked := f.readiness(f.wildcardAgent)
	require.False(t, blocked.Ready)
	require.Equal(t, []types.AgentReadinessReason{{Code: types.AgentReadinessSelectionAmbiguous}}, blocked.Reasons)
	require.Empty(t, blocked.WildcardProfileID, "the fallback is unknown")

	// The explicit profile supplies everything, so nothing needs the fallback.
	require.True(t, f.readiness(f.agent).Ready)
	// Without its grant, the grant would come from the ambiguous fallback.
	require.NoError(t, f.gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(readinessVMCP, "vp1agent")}))
	partial := f.readiness(f.agent)
	require.Equal(t, []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessSelectionAmbiguous}}, partial.Reasons)
}

// TestAgentReadinessUsesTheConnectPathChecks checks that the API is computed as the connect path computes it, with the
// store of grants the session manager is configured with: a stored grant makes the agent ready, and a session manager
// without the store reports no readiness at all rather than one computed without grants.
func TestAgentReadinessUsesTheConnectPathChecks(t *testing.T) {
	f := newReadinessFixture(t)
	f.configure("vp1agent", map[string]any{"REGION": readinessSecretValue})
	f.storeGrant("vp1agent", readinessRemoteURL, "grant-agent")
	require.True(t, f.readiness(f.agent).Ready)

	f.serve(NewAgentIdentityHandler(true, true, &mcp.SessionManager{}), NewVMCPProfileOAuthHandler(true, true, &recordingAuthorizer{}, f.grants))
	f.get(http.StatusInternalServerError, "/api/agents/"+strconv.FormatUint(uint64(f.agent.ID), 10)+"/vmcps/"+readinessVMCP+"/readiness", nil)

	// The report is exactly the readiness the session manager computes for a connect.
	f.serve(NewAgentIdentityHandler(true, true, f.manager), NewVMCPProfileOAuthHandler(true, true, &recordingAuthorizer{}, f.grants))
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: readinessVMCP}, &vmcp))
	want, err := f.manager.AgentReadiness(t.Context(), vmcp, nil, gateway.AgentUserInfo(f.agent, nil, nil))
	require.NoError(t, err)
	require.Equal(t, want.Readiness.Ready(), f.readiness(f.agent).Ready)
}

func TestAgentReadinessPendingSync(t *testing.T) {
	f := newReadinessFixture(t)
	f.configure("vp1agent", map[string]any{"REGION": readinessSecretValue})
	f.storeGrant("vp1agent", readinessRemoteURL, "grant-agent")
	require.False(t, f.readiness(f.agent).PendingSync, "an agent that has not connected has no connection to sync")

	instance := &v1.VMCPInstance{
		Name:      "vi1agent",
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPInstanceSpec{UserID: principal.Agent(f.agent.ID).String(), Manifest: types.VMCPInstanceManifest{VMCPID: readinessVMCP}},
	}
	require.NoError(t, f.storage.Create(t.Context(), instance))
	require.True(t, f.readiness(f.agent).PendingSync)

	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: readinessVMCP}, &vmcp))
	instance.Status.AgentConfigurationRevision = vmcpconfig.ResolveAgentConfiguration(vmcp, *instance, gateway.AgentUserInfo(f.agent, nil, nil)).Revision
	require.NoError(t, f.storage.Status().Update(t.Context(), instance))
	require.False(t, f.readiness(f.agent).PendingSync)
}

// TestAgentReadinessListAndLifecycle checks the list of the vMCPs an agent can access, and that it agrees with the
// agent's access summary, that a disabled agent reports its readiness, and that a deleted one is 404.
func TestAgentReadinessListAndLifecycle(t *testing.T) {
	f := newReadinessFixture(t)
	f.configure("vp1agents", map[string]any{"REGION": readinessSecretValue})
	everyone := types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}
	someoneElse := types.Subject{Type: types.SubjectTypeAgent, ID: "999"}
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	for _, other := range []*v1.VMCP{
		// Shared and open to everyone, with nothing to set up.
		{Name: "vmcp1open", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Open", Profiles: []types.VMCPProfile{profile("vp1open", everyone)}}}},
		// No profile names the agent.
		{Name: "vmcp1closed", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Closed", Profiles: []types.VMCPProfile{profile("vp1closed", someoneElse)}}}},
		// Personal vMCPs grant only their owner.
		{Name: "vmcp1personal", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{UserID: "1", Manifest: types.VMCPManifest{DisplayName: "Personal", Profiles: []types.VMCPProfile{profile("vp1personal", everyone)}}}},
	} {
		require.NoError(t, f.storage.Create(t.Context(), other))
	}

	var list types.AgentVMCPReadinessList
	f.get(http.StatusOK, agentPath(f.agent, "/vmcps"), &list)
	require.Len(t, list.Items, 2)
	require.Equal(t, "vmcp1open", list.Items[0].VMCPID)
	require.True(t, list.Items[0].Ready)
	require.Equal(t, readinessVMCP, list.Items[1].VMCPID)
	require.False(t, list.Items[1].Ready, "the remote component has no grant")
	require.Equal(t, []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired, ProfileID: "vp1agent"}}, list.Items[1].Reasons)
	require.Empty(t, list.Items[1].Sources, "the list leaves out sources")

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

	// A vMCP the agent has no access to, a personal one, and an unknown one are not found.
	f.get(http.StatusNotFound, agentPath(f.agent, "/vmcps/vmcp1closed/readiness"), nil)
	f.get(http.StatusNotFound, agentPath(f.agent, "/vmcps/vmcp1personal/readiness"), nil)
	f.get(http.StatusNotFound, agentPath(f.agent, "/vmcps/vmcp1unknown/readiness"), nil)

	// A key that can use the LLM gateway and skills, with no policy naming the agent, is a problem the access summary
	// reports.
	_, err := f.gateway.CreateAgentAPIKey(t.Context(), f.agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "key", CanAccessLLMProxy: true, CanAccessSkills: true})
	require.NoError(t, err)
	f.get(http.StatusOK, agentPath(f.agent, "/access"), &access)
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
	require.NoError(t, f.gateway.DeleteAgent(t.Context(), f.wildcardAgent.ID))
	f.get(http.StatusNotFound, agentPath(f.wildcardAgent, "/vmcps"), nil)
	f.get(http.StatusNotFound, agentPath(f.wildcardAgent, "/vmcps/"+readinessVMCP+"/readiness"), nil)
	f.get(http.StatusNotFound, "/api/agents/999/vmcps", nil)
	f.get(http.StatusNotFound, "/api/agents/nope/vmcps/"+readinessVMCP+"/readiness", nil)
}

// TestVMCPProfileStatusAPI checks the status of a profile: it covers every component, describes only that profile with
// no fallback to the wildcard profile, reports the grant's metadata and no value or token, and answers 404 for a vMCP or
// profile that is not there.
func TestVMCPProfileStatusAPI(t *testing.T) {
	f := newReadinessFixture(t)

	// Only the wildcard profile is set up.
	f.configure("vp1agents", map[string]any{"REGION": readinessSecretValue})
	f.storeGrant("vp1agents", readinessRemoteURL, "grant-agents")

	// The explicit profile is its own: it does not take the wildcard's setup.
	status := f.profileStatus("vp1agent")
	require.True(t, status.AgentCapable)
	require.False(t, status.PendingCollection)
	require.Equal(t, []types.VMCPProfileComponentStatus{
		{
			ComponentID:   "api",
			Configuration: types.VMCPProfileConfigurationStatus{Set: []string{}, MissingRequired: []string{"REGION"}, OptionalUnset: []string{"VERBOSE"}},
		},
		{
			ComponentID:   "remote",
			Configuration: types.VMCPProfileConfigurationStatus{Set: []string{}, MissingRequired: []string{}, OptionalUnset: []string{}},
			OAuth:         &types.VMCPProfileOAuthStatus{Required: true, State: types.VMCPProfileOAuthMissing},
		},
	}, status.Components)

	status = f.profileStatus("vp1agents")
	require.Equal(t, []string{"REGION"}, status.Components[0].Configuration.Set)
	require.Equal(t, types.VMCPProfileOAuthAuthorized, status.Components[1].OAuth.State)
	require.Equal(t, readinessRemoteURL, status.Components[1].OAuth.Target)
	require.Equal(t, "42", status.Components[1].OAuth.AuthorizedBy)
	require.NotNil(t, status.Components[1].OAuth.AuthorizedAt)

	// A profile that is not for agents says so.
	require.False(t, f.profileStatus("vp1users").AgentCapable)

	// A grant that stopped working is reported as invalid as soon as it does.
	require.NoError(t, f.gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(readinessVMCP, "vp1agents"), vmcpconfig.ProfileOAuthGrantMCPID(readinessVMCP, "remote"), func(_ context.Context, refresh *gateway.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-agents", "refused")
		return err
	}))
	oauth := f.profileStatus("vp1agents").Components[1].OAuth
	require.Equal(t, types.VMCPProfileOAuthInvalid, oauth.State)
	require.Equal(t, "refused", oauth.InvalidReason)

	f.get(http.StatusNotFound, "/api/vmcps/"+readinessVMCP+"/profiles/vp1missing/status", nil)
	f.get(http.StatusNotFound, "/api/vmcps/vmcp1missing/profiles/vp1agents/status", nil)
}

// TestAgentReadinessRoutesFollowAgentIdentities checks the new routes answer 404 while agent identities are off, and
// 501 without authentication, as the other routes of agents do.
func TestAgentReadinessRoutesFollowAgentIdentities(t *testing.T) {
	for _, tt := range []struct {
		name        string
		enabled     bool
		authEnabled bool
		want        int
	}{
		{
			name:        "agent identities disabled",
			authEnabled: true,
			want:        http.StatusNotFound,
		},
		{
			name:    "authentication disabled",
			enabled: true,
			want:    http.StatusNotImplemented,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newReadinessFixture(t)
			f.serve(NewAgentIdentityHandler(tt.enabled, tt.authEnabled, f.manager), NewVMCPProfileOAuthHandler(tt.enabled, tt.authEnabled, &recordingAuthorizer{}, f.grants))
			for _, path := range []string{
				agentPath(f.agent, "/vmcps"),
				agentPath(f.agent, "/vmcps/"+readinessVMCP+"/readiness"),
				"/api/vmcps/" + readinessVMCP + "/profiles/vp1agents/status",
			} {
				rec := httptest.NewRecorder()
				f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, bytes.NewReader(nil)))
				require.Equal(t, tt.want, rec.Code, path)
			}
		})
	}
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

// TestAgentReadinessRoutesAreNotForAgents checks the new routes against the server's authorizer, in the way the other
// routes of agents are checked: administrators and owners reach them, auditors read them, and no other user and no
// agent, not even the one the readiness is about, does.
func TestAgentReadinessRoutesAreNotForAgents(t *testing.T) {
	scopes := gatewaytypes.APIKeyScopes{CanAccessSkills: true, CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}}
	agent := gateway.AgentUserInfo(&gatewaytypes.Agent{ID: 7, DisplayName: "agent"}, scopes.Groups(nil), map[string][]string{"authorized_mcp_ids": scopes.MCPServerIDs})
	authorizer := authz.NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
	// Administrators and owners have the whole tree under /api/agents and /api/vmcps, whatever the method, so the mux
	// decides what exists; auditors read it; and nobody else reaches it.
	for _, tt := range []struct {
		name   string
		user   user.Info
		reads  bool
		writes bool
	}{
		{
			name:   "owner",
			user:   &user.DefaultInfo{UID: "1", Groups: types.RoleOwner.Groups()},
			reads:  true,
			writes: true,
		},
		{
			name:   "admin",
			user:   &user.DefaultInfo{UID: "1", Groups: types.RoleAdmin.Groups()},
			reads:  true,
			writes: true,
		},
		{
			name:  "auditor",
			user:  &user.DefaultInfo{UID: "1", Groups: (types.RoleBasic | types.RoleAuditor).Groups()},
			reads: true,
		},
		{
			name: "basic user",
			user: &user.DefaultInfo{UID: "2", Groups: types.RoleBasic.Groups()},
		},
		{
			name: "power user plus",
			user: &user.DefaultInfo{UID: "3", Groups: types.RolePowerUserPlus.Groups()},
		},
		{
			name: "the agent itself",
			user: agent,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, path := range []string{
				"/api/agents/7/vmcps",
				"/api/agents/7/vmcps/" + readinessVMCP + "/readiness",
				"/api/vmcps/" + readinessVMCP + "/profiles/vp1agents/status",
			} {
				require.Equal(t, tt.reads, authorizer.Authorize(httptest.NewRequest(http.MethodGet, path, nil), tt.user), "GET %s", path)
				for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
					require.Equal(t, tt.writes, authorizer.Authorize(httptest.NewRequest(method, path, nil), tt.user), "%s %s", method, path)
				}
			}
		})
	}
}

// TestProfileOAuthStatusNeverReturnsConfiguredURLs checks what a profile's status says of the URL a grant was authorized
// for. A component whose catalog entry fixes the URL returns it as target: it is catalog data. A component whose URL comes
// from configuration, a hostname-constrained URL or a URL template, returns only the hostname as targetHostname, and the
// URL, with its port, userinfo, path and query, is nowhere in the raw response: not in the status, not in the response of
// a reset, and not in what readiness reports.
func TestProfileOAuthStatusNeverReturnsConfiguredURLs(t *testing.T) {
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
		require.NoError(t, f.gateway.ReplaceMCPOAuthGrant(t.Context(), principal.ProfileGrant(readinessVMCP, "vp1agents"), vmcpconfig.ProfileOAuthGrantMCPID(readinessVMCP, component), url, "grant-"+component, "42",
			&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: readinessAccessToken, Expiry: time.Now().Add(time.Hour)}))
	}

	check := func(label, body string) {
		t.Helper()
		for _, secret := range secrets {
			require.NotContains(t, body, secret, "%s: %s", label, body)
		}
	}
	byComponent := func(status types.VMCPProfileStatus) map[string]*types.VMCPProfileOAuthStatus {
		result := map[string]*types.VMCPProfileOAuthStatus{}
		for _, component := range status.Components {
			result[component.ComponentID] = component.OAuth
		}
		return result
	}

	var status types.VMCPProfileStatus
	check("status", f.get(http.StatusOK, "/api/vmcps/"+readinessVMCP+"/profiles/vp1agents/status", &status))
	oauth := byComponent(status)
	require.Equal(t, readinessRemoteURL, oauth["fixed"].Target)
	require.Empty(t, oauth["fixed"].TargetHostname)
	require.Empty(t, oauth["hostname"].Target)
	require.Equal(t, "tenant.example.com", oauth["hostname"].TargetHostname)
	require.Empty(t, oauth["template"].Target)
	require.Equal(t, "acme-secret-tenant.example.org", oauth["template"].TargetHostname)

	// The response of a reset reports the grants of the profile's other components.
	body, err := callProfileOAuth(t, NewVMCPProfileOAuthHandler(true, true, &recordingAuthorizer{}, f.grants), f.storage, f.gateway, http.MethodDelete, readinessVMCP, "vp1agents", "fixed")
	require.NoError(t, err)
	check("reset", body)
	status = types.VMCPProfileStatus{}
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	oauth = byComponent(status)
	require.Equal(t, types.VMCPProfileOAuthMissing, oauth["fixed"].State)
	require.Equal(t, "tenant.example.com", oauth["hostname"].TargetHostname)
	require.Empty(t, oauth["hostname"].Target)

	// Readiness, with its reasons and sources, names profiles and components, never a URL.
	check("readiness", f.get(http.StatusOK, agentPath(f.agent, "/vmcps/"+readinessVMCP+"/readiness"), nil))
	check("list", f.get(http.StatusOK, agentPath(f.agent, "/vmcps"), nil))
}
