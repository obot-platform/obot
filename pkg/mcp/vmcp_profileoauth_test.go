package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	grantComponentURL = "https://one.example.com/mcp"
	grantSeven        = "vp1seven"
	grantEight        = "vp1eight"
	grantUsers        = "vp1users"
)

// grantVMCP returns a shared vMCP with one shared remote component that may ask for OAuth, with a profile for agent 7,
// one for agent 8 and one for every user, and the component's shared server.
func grantVMCP(name string, staticOAuth bool) (*v1.VMCP, *v1.MCPServer) {
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	vmcp := &v1.VMCP{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Grants",
			Components: []types.VMCPComponent{{
				ID:   "one",
				Name: "one",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: grantComponentURL, StaticOAuthRequired: staticOAuth},
				}},
			}},
			Profiles: []types.VMCPProfile{
				profile(grantSeven, types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
				profile(grantEight, types.Subject{Type: types.SubjectTypeAgent, ID: "8"}),
				profile(grantUsers, types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
			},
		}},
	}
	shared := vmcpComponentServer("ms1shared-one", "", "", "one", "one", grantComponentURL)
	shared.Spec.VMCPID = vmcp.Name
	return vmcp, shared
}

// storeGrant stores the grant a profile holds for the component, authorized for url.
func storeGrant(t *testing.T, gateway *gatewayclient.Client, vmcpID, profileID, url, grantID string) {
	t.Helper()
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), principal.ProfileGrant(vmcpID, profileID), vmcpaccess.ProfileOAuthGrantMCPID(vmcpID, "one"), url, grantID, "1",
		&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token-" + profileID, Expiry: time.Now().Add(time.Hour)}))
}

// TestAgentComponentConnectionsUseProfileGrants checks the token every connection to a vMCP component uses: an
// agent's uses the grant of the profile that names it, and no other profile's, even the profile for every user; it is
// refused when that grant broke, and refused a grant authorized for another upstream; a user's uses the user's own
// token as before; and neither ever reads the other's.
func TestAgentComponentConnectionsUseProfileGrants(t *testing.T) {
	vmcp, shared := grantVMCP("vmcp1grants", false)
	callers := []struct {
		name   string
		caller kuser.Info
	}{
		{name: "agent seven", caller: asAgent(7, 11)},
		{name: "agent eight", caller: asAgent(8, 12)},
		{name: "user seven", caller: principaltest.AsUser(&kuser.DefaultInfo{UID: "7"})},
	}
	objects := []kclient.Object{vmcp, shared}
	connections := map[string]string{}
	for i, caller := range callers {
		owner, err := principal.CallerOwner(caller.caller)
		require.NoError(t, err)
		instance := syncedVMCPInstanceForUser(&v1.VMCPInstance{
			Name:      "vmcpi1grants" + string(rune('a'+i)),
			Namespace: system.DefaultNamespace,
			Spec:      v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
		}, vmcp, caller.caller)
		connection := syncedVMCPConnection(t, &v1.MCPServerInstance{
			Name:      "msi1grants" + string(rune('a'+i)),
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:          owner,
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: "one",
				MCPServerName:   shared.Name,
			},
		}, vmcp, instance)
		objects = append(objects, instance, connection)
		connections[caller.name] = connection.Name
	}
	storage := newVMCPTestStorage(objects...)
	gateway := newGrantTestGateway(t)
	sm := &SessionManager{
		backend:                   &kubernetesBackend{},
		baseURL:                   "https://obot.example.com",
		storageClient:             storage,
		gatewayClient:             gateway,
		remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true},
	}
	sm.SetProfileOAuthGrants(vmcpaccess.NewProfileOAuthGrantStore(gateway))
	t.Cleanup(sm.Close)

	// The user and agent seven each hold a token keyed by themselves for their own connection. Neither is a grant.
	for _, owner := range []struct {
		userID, connection string
	}{
		{userID: "7", connection: connections["user seven"]},
		{userID: "agent:7", connection: connections["agent seven"]},
	} {
		require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), owner.userID, owner.connection, grantComponentURL, "", &oauth2.Config{ClientID: "own"}, &oauth2.Token{AccessToken: "own-" + owner.userID}))
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	connect := func(name string, caller kuser.Info) (ServerConfig, error) {
		t.Helper()
		_, cfg, err := sm.ServerForAction(ctx, connections[name], caller)
		return cfg, err
	}
	requireGrant := func(name string, caller kuser.Info, profileID string) {
		t.Helper()
		cfg, err := connect(name, caller)
		require.NoError(t, err)
		owner, mcpID, ok := cfg.OAuthTokenKey()
		if profileID == "" {
			require.Nil(t, cfg.OAuthGrant)
			require.False(t, ok, "an agent without a grant read a token keyed by itself")
			return
		}
		require.NotNil(t, cfg.OAuthGrant)
		require.Equal(t, profileID, cfg.OAuthGrant.ProfileID)
		require.True(t, ok)
		require.Equal(t, principal.ProfileGrant(vmcp.Name, profileID), owner)
		require.Equal(t, vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one"), mcpID)
		// The connection is still the agent's own: its sessions, caches and audit are keyed by it.
		require.Equal(t, caller.GetUID(), cfg.UserID)
	}
	requireRefused := func(name string, caller kuser.Info, profileID string) {
		t.Helper()
		_, err := connect(name, caller)
		grantErr, ok := errors.AsType[*AgentOAuthGrantError](err)
		require.True(t, ok, "connect error = %v, want an AgentOAuthGrantError", err)
		require.Equal(t, profileID, grantErr.ProfileID)
		require.Equal(t, "one", grantErr.ComponentID)
	}

	// No grant: agents use no token at all, never one keyed by themselves.
	requireGrant("agent seven", callers[0].caller, "")
	requireGrant("agent eight", callers[1].caller, "")

	// The user is unaffected throughout: the user's own token, for the user's own connection.
	requireUser := func() {
		t.Helper()
		cfg, err := connect("user seven", callers[2].caller)
		require.NoError(t, err)
		require.Nil(t, cfg.OAuthGrant)
		owner, mcpID, ok := cfg.OAuthTokenKey()
		require.True(t, ok)
		require.Equal(t, "7", owner)
		require.Equal(t, connections["user seven"], mcpID)
	}
	requireUser()

	// The grant of the profile for every user is never an agent's.
	storeGrant(t, gateway, vmcp.Name, grantSeven, grantComponentURL, "grant-seven")
	storeGrant(t, gateway, vmcp.Name, grantUsers, grantComponentURL, "grant-users")
	requireGrant("agent seven", callers[0].caller, grantSeven)
	requireGrant("agent eight", callers[1].caller, "")
	requireUser()
	storeGrant(t, gateway, vmcp.Name, grantEight, grantComponentURL, "grant-eight")
	requireGrant("agent eight", callers[1].caller, grantEight)

	// A broken grant refuses the agent; it never uses another profile's working grant.
	var marked bool
	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(vmcp.Name, grantSeven), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one"), func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		var err error
		marked, err = refresh.Invalidate("grant-seven", "refused")
		return err
	}))
	require.True(t, marked)
	requireRefused("agent seven", callers[0].caller, grantSeven)
	requireGrant("agent eight", callers[1].caller, grantEight)

	// Without its grant, the agent uses none.
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(vmcp.Name, grantSeven)}))
	requireGrant("agent seven", callers[0].caller, "")

	// A grant authorized for another upstream is never sent to this one.
	storeGrant(t, gateway, vmcp.Name, grantEight, "https://elsewhere.example.com/mcp", "grant-elsewhere")
	requireRefused("agent eight", callers[1].caller, grantEight)
	requireUser()
}

// TestAgentConnectUsesProfileGrants checks connect-time readiness and the composite configuration with profile grants:
// an agent connects with the grant of the profile that names it; one whose profile holds no grant for a component that
// requires OAuth is refused with oauth_required, though the profile for every user holds a working grant; and one whose
// grant broke is refused with oauth_invalid.
func TestAgentConnectUsesProfileGrants(t *testing.T) {
	vmcp, shared := grantVMCP("vmcp1connectgrants", true)
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	grants := vmcpaccess.NewProfileOAuthGrantStore(gateway)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}
	manager.SetProfileOAuthGrants(grants)
	refused := func(caller kuser.Info) vmcpaccess.AgentReadinessFailure {
		t.Helper()
		_, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, caller)
		notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
		require.True(t, ok, "connect error = %v, want an AgentNotReadyError", err)
		return notReady.Failure()
	}
	required := func(profileID string) vmcpaccess.AgentReadinessFailure {
		return vmcpaccess.AgentReadinessFailure{
			Error:   vmcpaccess.AgentOAuthRequiredError,
			Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthRequired, ProfileID: profileID}},
		}
	}
	grantOf := func(profileID string) *OAuthGrantRef {
		return &OAuthGrantRef{ComponentID: "one", ProfileID: profileID, Owner: principal.ProfileGrant(vmcp.Name, profileID), MCPID: vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one")}
	}

	require.Equal(t, required(grantSeven), refused(asAgent(7, 11)))

	storeGrant(t, gateway, vmcp.Name, grantSeven, grantComponentURL, "grant-seven")
	storeGrant(t, gateway, vmcp.Name, grantUsers, grantComponentURL, "grant-users")
	cfg := connectVMCP(t, manager, vmcp.Name, asAgent(7, 11))
	require.Equal(t, grantOf(grantSeven), cfg.Components[0].OAuthGrant)
	require.Equal(t, grantSeven, cfg.AuditLogMetadata[AuditLogProfiles])
	require.Equal(t, required(grantEight), refused(asAgent(8, 12)))

	storeGrant(t, gateway, vmcp.Name, grantEight, grantComponentURL, "grant-eight")
	cfg = connectVMCP(t, manager, vmcp.Name, asAgent(8, 12))
	require.Equal(t, grantOf(grantEight), cfg.Components[0].OAuthGrant)
	require.Equal(t, grantEight, cfg.AuditLogMetadata[AuditLogProfiles])

	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(vmcp.Name, grantSeven), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one"), func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-seven", "refused")
		return err
	}))
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthInvalid, ProfileID: grantSeven}},
	}, refused(asAgent(7, 11)))

	// A user connects as before: no grant is ever selected for them.
	cfg = connectVMCP(t, manager, vmcp.Name, principaltest.AsUser(&kuser.DefaultInfo{UID: "7"}))
	require.Nil(t, cfg.Components[0].OAuthGrant)
}

// TestAgentReadinessReportAgreesWithConnect checks that what the readiness API reports, which is computed with the
// session manager (SessionManager.AgentReadiness) and with no instance, is what a connect decides for the
// agent through its own instance, through every state of the grants and of the vMCP's profiles. If they disagreed, the API
// would report an agent ready that a connect refuses.
func TestAgentReadinessReportAgreesWithConnect(t *testing.T) {
	vmcp, shared := grantVMCP("vmcp1agree", true)
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	grants := vmcpaccess.NewProfileOAuthGrantStore(gateway)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}
	manager.SetProfileOAuthGrants(grants)

	agree := func(label string, ready7, ready8 bool) {
		t.Helper()
		requireReadinessAgreesWithConnect(t, manager, vmcp, label, ready7, ready8)
	}

	agree("no grant", false, false)
	storeGrant(t, gateway, vmcp.Name, grantUsers, grantComponentURL, "grant-users")
	agree("only the grant of the profile for every user", false, false)
	storeGrant(t, gateway, vmcp.Name, grantSeven, grantComponentURL, "grant-seven")
	agree("agent 7's grant", true, false)
	storeGrant(t, gateway, vmcp.Name, grantEight, grantComponentURL, "grant-eight")
	agree("agent 8's grant too", true, true)
	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(vmcp.Name, grantSeven), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one"), func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-seven", "refused")
		return err
	}))
	agree("agent 7's grant broken", false, true)
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(vmcp.Name, grantSeven)}))
	agree("agent 7's grant reset", false, true)
	storeGrant(t, gateway, vmcp.Name, grantEight, "https://elsewhere.example.com/mcp", "grant-elsewhere")
	agree("a grant authorized for another upstream", false, false)
}

// requireReadinessAgreesWithConnect requires that what the readiness API reports for agents 7 and 8 on vmcp is what a
// connect decides for each through its own instance, and that each is ready as wanted. It returns the reasons reported
// for each, by UID.
func requireReadinessAgreesWithConnect(t *testing.T, manager *SessionManager, vmcp *v1.VMCP, label string, ready7, ready8 bool) map[string][]vmcpaccess.ReadinessReason {
	t.Helper()
	reasons := map[string][]vmcpaccess.ReadinessReason{}
	for _, caller := range []kuser.Info{asAgent(7, 11), asAgent(8, 12)} {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		_, connectErr := manager.ServerConfigForVMCP(ctx, vmcp.Name, caller)
		cancel()
		report, err := manager.AgentReadiness(t.Context(), *vmcp, nil, caller)
		require.NoError(t, err)

		var connectReasons []vmcpaccess.ReadinessReason
		if notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](connectErr); ok {
			connectReasons = notReady.Readiness.Reasons
		} else {
			require.NoError(t, connectErr, "%s: %s", label, caller.GetUID())
		}
		require.Equal(t, connectReasons, report.Readiness.Reasons, "%s: %s", label, caller.GetUID())
		wantReady := ready7
		if caller.GetUID() == "agent:8" {
			wantReady = ready8
		}
		require.Equal(t, wantReady, report.Readiness.Ready(), "%s: %s", label, caller.GetUID())
		reasons[caller.GetUID()] = report.Readiness.Reasons
	}
	return reasons
}

// TestAgentReadinessReportAgreesWithConnectForLateChallenges is TestAgentReadinessReportAgreesWithConnect for a
// component without static OAuth, which may only ask for OAuth once it is called. Readiness cannot see that until a
// profile holds a grant for it, so both agents are ready while none does. From the first grant on, an agent whose
// profile holds none is refused at connect with oauth_required, as readiness reports, even once the only grant broke.
func TestAgentReadinessReportAgreesWithConnectForLateChallenges(t *testing.T) {
	vmcp, shared := grantVMCP("vmcp1agreelate", false)
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	grants := vmcpaccess.NewProfileOAuthGrantStore(gateway)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}
	manager.SetProfileOAuthGrants(grants)
	requiredOf := func(profileID string) []vmcpaccess.ReadinessReason {
		return []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthRequired, ProfileID: profileID}}
	}
	sevenOwner, mcpID := principal.ProfileGrant(vmcp.Name, grantSeven), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one")

	requireReadinessAgreesWithConnect(t, manager, vmcp, "no grant", true, true)

	// Agent 7's profile authorizes the component: agent 8, whose profile holds no grant, is refused.
	storeGrant(t, gateway, vmcp.Name, grantSeven, grantComponentURL, "grant-seven")
	reasons := requireReadinessAgreesWithConnect(t, manager, vmcp, "only agent 7's grant", true, false)
	require.Equal(t, requiredOf(grantEight), reasons["agent:8"])

	storeGrant(t, gateway, vmcp.Name, grantEight, grantComponentURL, "grant-eight")
	requireReadinessAgreesWithConnect(t, manager, vmcp, "agent 8's grant too", true, true)

	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), sevenOwner, mcpID, func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-seven", "refused")
		return err
	}))
	requireReadinessAgreesWithConnect(t, manager, vmcp, "agent 7's grant broken", false, true)

	// A broken grant still shows that the component uses OAuth.
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(vmcp.Name, grantEight)}))
	reasons = requireReadinessAgreesWithConnect(t, manager, vmcp, "only agent 7's broken grant", false, false)
	require.Equal(t, requiredOf(grantEight), reasons["agent:8"])

	// A grant of the users' profile counts as well: every profile of the vMCP does, though it is no agent's.
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{sevenOwner}))
	storeGrant(t, gateway, vmcp.Name, grantUsers, grantComponentURL, "grant-users")
	reasons = requireReadinessAgreesWithConnect(t, manager, vmcp, "only the users' profile's grant", false, false)
	require.Equal(t, requiredOf(grantSeven), reasons["agent:7"])
	require.Equal(t, requiredOf(grantEight), reasons["agent:8"])

	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(vmcp.Name, grantUsers)}))
	requireReadinessAgreesWithConnect(t, manager, vmcp, "every grant reset", true, true)
}
