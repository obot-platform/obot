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
	grantAgents       = "vp1agents"
	grantUsers        = "vp1users"
)

// grantVMCP returns a shared vMCP with one shared remote component that may ask for OAuth, with a profile for agent 7,
// one for every agent and one for every user, and the component's shared server.
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
				profile(grantAgents, types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
				profile(grantUsers, types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}),
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
// agent's uses the grant of its explicit profile, falls back to the wildcard profile's only when its explicit profile
// holds none, is refused rather than falling back when its explicit grant broke, and is refused a grant authorized for
// another upstream; a user's uses the user's own token as before; and neither ever reads the other's.
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

	storeGrant(t, gateway, vmcp.Name, grantSeven, grantComponentURL, "grant-seven")
	storeGrant(t, gateway, vmcp.Name, grantAgents, grantComponentURL, "grant-agents")
	requireGrant("agent seven", callers[0].caller, grantSeven)
	requireGrant("agent eight", callers[1].caller, grantAgents)
	requireUser()

	// A broken explicit grant refuses the agent; it never falls back to the working wildcard grant.
	var marked bool
	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(vmcp.Name, grantSeven), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one"), func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		var err error
		marked, err = refresh.Invalidate("grant-seven", "refused")
		return err
	}))
	require.True(t, marked)
	requireRefused("agent seven", callers[0].caller, grantSeven)
	requireGrant("agent eight", callers[1].caller, grantAgents)

	// A missing explicit grant falls back.
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(vmcp.Name, grantSeven)}))
	requireGrant("agent seven", callers[0].caller, grantAgents)

	// A grant authorized for another upstream is never sent to this one.
	storeGrant(t, gateway, vmcp.Name, grantAgents, "https://elsewhere.example.com/mcp", "grant-elsewhere")
	requireRefused("agent seven", callers[0].caller, grantAgents)
	requireRefused("agent eight", callers[1].caller, grantAgents)
	requireUser()
}

// TestAgentConnectUsesProfileGrants checks connect-time readiness and the composite configuration with profile grants:
// an agent whose explicit grant broke is refused with oauth_invalid, though the wildcard profile holds a working grant;
// an agent whose explicit profile holds none is connected with the wildcard's, recorded as a fallback in the audit
// metadata; and an agent with no grant for a component that requires OAuth is refused with oauth_required.
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

	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthRequired, ProfileID: grantSeven}},
	}, refused(asAgent(7, 11)))

	storeGrant(t, gateway, vmcp.Name, grantSeven, grantComponentURL, "grant-seven")
	storeGrant(t, gateway, vmcp.Name, grantAgents, grantComponentURL, "grant-agents")
	cfg := connectVMCP(t, manager, vmcp.Name, asAgent(7, 11))
	require.Equal(t, &OAuthGrantRef{ComponentID: "one", ProfileID: grantSeven, Owner: principal.ProfileGrant(vmcp.Name, grantSeven), MCPID: vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one")}, cfg.Components[0].OAuthGrant)
	require.Empty(t, cfg.AuditLogMetadata[AuditLogFallbackProfiles])

	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(vmcp.Name, grantSeven), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one"), func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-seven", "refused")
		return err
	}))
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthInvalid, ProfileID: grantSeven}},
	}, refused(asAgent(7, 11)))

	// An agent of the wildcard profile alone, and agent seven once its explicit grant is gone, use the wildcard's.
	for _, caller := range []kuser.Info{asAgent(8, 12), asAgent(7, 11)} {
		if caller.GetUID() == "agent:7" {
			require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(vmcp.Name, grantSeven)}))
		}
		cfg := connectVMCP(t, manager, vmcp.Name, caller)
		require.NotNil(t, cfg.Components[0].OAuthGrant)
		require.Equal(t, grantAgents, cfg.Components[0].OAuthGrant.ProfileID)
		require.Equal(t, grantAgents, cfg.AuditLogMetadata[AuditLogFallbackProfiles])
	}

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
		}
	}

	agree("no grant", false, false)
	storeGrant(t, gateway, vmcp.Name, grantAgents, grantComponentURL, "grant-agents")
	agree("only the wildcard profile's grant", true, true)
	storeGrant(t, gateway, vmcp.Name, grantSeven, grantComponentURL, "grant-seven")
	agree("an explicit grant too", true, true)
	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), principal.ProfileGrant(vmcp.Name, grantSeven), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, "one"), func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		_, err := refresh.Invalidate("grant-seven", "refused")
		return err
	}))
	agree("a broken explicit grant", false, true)
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwners(t.Context(), []string{principal.ProfileGrant(vmcp.Name, grantSeven)}))
	agree("the explicit grant reset", true, true)
	storeGrant(t, gateway, vmcp.Name, grantAgents, "https://elsewhere.example.com/mcp", "grant-elsewhere")
	agree("a grant authorized for another upstream", false, false)
}

// TestAgentReadinessReportAgreesWithConnectWhenAmbiguous is TestAgentReadinessReportAgreesWithConnect for a vMCP that
// more than one profile names every agent for: the agent with only the wildcard is refused for the whole vMCP, and the
// agent with an explicit profile only for the grant that would come from the fallback.
func TestAgentReadinessReportAgreesWithConnectWhenAmbiguous(t *testing.T) {
	vmcp, shared := grantVMCP("vmcp1ambiguous", true)
	vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, types.VMCPProfile{
		ID:          "vp1others",
		Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	})
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	grants := vmcpaccess.NewProfileOAuthGrantStore(gateway)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}
	manager.SetProfileOAuthGrants(grants)

	for _, tt := range []struct {
		name   string
		caller kuser.Info
		want   []vmcpaccess.ReadinessReason
	}{
		{
			name:   "an agent with only the ambiguous wildcard",
			caller: asAgent(8, 12),
			want:   []vmcpaccess.ReadinessReason{{Code: types.AgentReadinessSelectionAmbiguous}},
		},
		{
			name:   "an agent with an explicit profile that holds no grant",
			caller: asAgent(7, 11),
			want:   []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessSelectionAmbiguous}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, tt.caller)
			notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
			require.True(t, ok, "connect error = %v", err)
			report, err := manager.AgentReadiness(t.Context(), *vmcp, nil, tt.caller)
			require.NoError(t, err)
			require.Equal(t, tt.want, report.Readiness.Reasons)
			require.Equal(t, tt.want, notReady.Readiness.Reasons)
		})
	}
}
