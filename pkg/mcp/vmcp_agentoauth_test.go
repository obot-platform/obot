package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	"github.com/obot-platform/obot/pkg/storage"
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
	grantSevenTwo     = "vp1seventwo"
	grantEight        = "vp1eight"
	grantNine         = "vp1nine"
	grantUsers        = "vp1users"
)

// grantVMCP returns a shared vMCP with a shared remote component, one, that may ask for OAuth, and a shared component,
// two, that needs nothing, and their shared servers. Agent 7 is named by two profiles, one granting each component, so
// it is granted both; agent 8 is named by a profile that grants only component one, and agent 9 by one that grants only
// component two. A profile grants every user both.
func grantVMCP(name string, staticOAuth bool) (*v1.VMCP, *v1.MCPServer, *v1.MCPServer) {
	profile := func(id string, subject types.Subject, componentIDs ...string) types.VMCPProfile {
		allowed := map[string]types.VMCPComponentSet{}
		for _, componentID := range componentIDs {
			allowed[componentID] = types.VMCPComponentSet{}
		}
		return types.VMCPProfile{
			ID:          id,
			Name:        id,
			Subjects:    []types.Subject{subject},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: allowed},
		}
	}
	agent := func(id string) types.Subject { return types.Subject{Type: types.SubjectTypeAgent, ID: id} }
	users := profile(grantUsers, types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID})
	users.Permissions = types.VMCPProfilePermissions{AllowAllComponents: true}
	vmcp := &v1.VMCP{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Grants",
			Components: []types.VMCPComponent{
				{
					ID:   "one",
					Name: "one",
					CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
						Runtime:      types.RuntimeRemote,
						RemoteConfig: &types.RemoteCatalogConfig{FixedURL: grantComponentURL, StaticOAuthRequired: staticOAuth},
					}},
				},
				{
					ID:   "two",
					Name: "two",
				},
			},
			Profiles: []types.VMCPProfile{
				profile(grantSeven, agent("7"), "one"),
				profile(grantSevenTwo, agent("7"), "two"),
				profile(grantEight, agent("8"), "one"),
				profile(grantNine, agent("9"), "two"),
				users,
			},
		}},
	}
	one := vmcpComponentServer("ms1shared-one", "", "", "one", "one", grantComponentURL)
	one.Spec.VMCPID = vmcp.Name
	two := vmcpComponentServer("ms1shared-two", "", "", "two", "two", "https://two.example.com/mcp")
	two.Spec.VMCPID = vmcp.Name
	return vmcp, one, two
}

// grantKey returns the key of the token of the vMCP's agent OAuth grant for component one.
func grantKey(vmcpID string) (owner, mcpID string) {
	return principal.AgentOAuthGrantOwner(vmcpID), vmcpaccess.AgentOAuthGrantMCPID(vmcpID, "one")
}

// storeGrant stores the vMCP's agent OAuth grant for component one, authorized once by an administrator for url.
func storeGrant(t *testing.T, gateway *gatewayclient.Client, vmcpID, url, grantID string) {
	t.Helper()
	owner, mcpID := grantKey(vmcpID)
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, url, grantID, "1",
		&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token-" + grantID, Expiry: time.Now().Add(time.Hour)}))
}

// invalidateGrant marks the vMCP's grant for component one invalid, as a refused refresh does.
func invalidateGrant(t *testing.T, gateway *gatewayclient.Client, vmcpID, grantID string) {
	t.Helper()
	owner, mcpID := grantKey(vmcpID)
	var marked bool
	require.NoError(t, gateway.WithMCPOAuthRefreshLock(t.Context(), owner, mcpID, func(_ context.Context, refresh *gatewayclient.MCPOAuthRefresh) error {
		var err error
		marked, err = refresh.Invalidate(grantID, "refused")
		return err
	}))
	require.True(t, marked)
}

// revokeGrant deletes the vMCP's grant for component one, as an administrator's revocation does.
func revokeGrant(t *testing.T, gateway *gatewayclient.Client, vmcpID string) {
	t.Helper()
	require.NoError(t, gateway.DeleteMCPOAuthGrantsForOwner(t.Context(), principal.AgentOAuthGrantOwner(vmcpID)))
}

// newGrantTestManager returns a session manager over storageClient whose agent OAuth grants are stored in gateway.
func newGrantTestManager(storageClient storage.Client, gateway *gatewayclient.Client) (*SessionManager, *vmcpaccess.AgentOAuthGrantStore) {
	grants := vmcpaccess.NewAgentOAuthGrantStore(gateway)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort, remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true}}
	manager.SetAgentOAuthGrants(grants)
	return manager, grants
}

// TestAgentComponentConnectionsUseTheVMCPGrant checks the token every connection to a vMCP component uses: an agent's
// uses the vMCP's one grant for the component when a profile naming the agent grants it the component, the same grant
// whichever profile does, and none for a component that no profile naming it grants; it is refused when the grant broke,
// and refused a grant authorized for another upstream; a user's uses the user's own token as before; and neither ever
// reads the other's.
func TestAgentComponentConnectionsUseTheVMCPGrant(t *testing.T) {
	vmcp, one, two := grantVMCP("vmcp1grants", false)
	callers := []struct {
		name   string
		caller kuser.Info
	}{
		{name: "agent seven", caller: asAgent(7, 11)},
		{name: "agent eight", caller: asAgent(8, 12)},
		{name: "agent nine", caller: asAgent(9, 13)},
		{name: "user seven", caller: principaltest.AsUser(&kuser.DefaultInfo{UID: "7"})},
	}
	objects := []kclient.Object{vmcp, one, two}
	connections := map[string]string{}
	callerOf := map[string]kuser.Info{}
	for i, caller := range callers {
		callerOf[caller.name] = caller.caller
		owner, err := principal.CallerOwner(caller.caller)
		require.NoError(t, err)
		instance := syncedVMCPInstanceForUser(&v1.VMCPInstance{
			Name:      "vmcpi1grants" + string(rune('a'+i)),
			Namespace: system.DefaultNamespace,
			Spec:      v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
		}, vmcp, caller.caller)
		// Every caller has a connection to component one, even agent nine, whom no profile grants it, so that the
		// grant is shown never to be given to it.
		connection := syncedVMCPConnection(t, &v1.MCPServerInstance{
			Name:      "msi1grants" + string(rune('a'+i)),
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:          owner,
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: "one",
				MCPServerName:   one.Name,
			},
		}, vmcp, instance)
		objects = append(objects, instance, connection)
		connections[caller.name] = connection.Name
	}
	storageClient := newVMCPTestStorage(objects...)
	gateway := newGrantTestGateway(t)
	sm := &SessionManager{
		backend:                   &kubernetesBackend{},
		baseURL:                   "https://obot.example.com",
		storageClient:             storageClient,
		gatewayClient:             gateway,
		remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true},
	}
	sm.SetAgentOAuthGrants(vmcpaccess.NewAgentOAuthGrantStore(gateway))
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
	connect := func(name string) (ServerConfig, error) {
		t.Helper()
		_, cfg, err := sm.ServerForAction(ctx, connections[name], callerOf[name])
		return cfg, err
	}
	grantOwner, grantMCPID := grantKey(vmcp.Name)
	requireGrant := func(name string, granted bool) {
		t.Helper()
		cfg, err := connect(name)
		require.NoError(t, err, name)
		owner, mcpID, ok := cfg.OAuthTokenKey()
		if !granted {
			require.Nil(t, cfg.OAuthGrant, name)
			require.False(t, ok, "%s without a grant read a token keyed by itself", name)
			return
		}
		require.Equal(t, &OAuthGrantRef{ComponentID: "one", Owner: grantOwner, MCPID: grantMCPID}, cfg.OAuthGrant, name)
		require.True(t, ok, name)
		require.Equal(t, grantOwner, owner, name)
		require.Equal(t, grantMCPID, mcpID, name)
		// The connection is still the agent's own: its sessions, caches and audit are keyed by it.
		require.Equal(t, callerOf[name].GetUID(), cfg.UserID, name)
	}
	requireRefused := func(name string) {
		t.Helper()
		_, err := connect(name)
		grantErr, ok := errors.AsType[*AgentOAuthGrantError](err)
		require.True(t, ok, "%s connect error = %v, want an AgentOAuthGrantError", name, err)
		require.Equal(t, vmcp.Name, grantErr.VMCPID, name)
		require.Equal(t, "one", grantErr.ComponentID, name)
	}
	// The user is unaffected throughout: the user's own token, for the user's own connection.
	requireUser := func() {
		t.Helper()
		cfg, err := connect("user seven")
		require.NoError(t, err)
		require.Nil(t, cfg.OAuthGrant)
		owner, mcpID, ok := cfg.OAuthTokenKey()
		require.True(t, ok)
		require.Equal(t, "7", owner)
		require.Equal(t, connections["user seven"], mcpID)
	}

	// No grant: agents use no token at all, never one keyed by themselves.
	requireGrant("agent seven", false)
	requireGrant("agent eight", false)
	requireGrant("agent nine", false)
	requireUser()

	// One grant, authorized once on the vMCP, is used by every agent that a profile grants the component, whichever
	// profile that is, and by no agent that no profile naming it grants the component.
	storeGrant(t, gateway, vmcp.Name, grantComponentURL, "grant-one")
	requireGrant("agent seven", true)
	requireGrant("agent eight", true)
	requireGrant("agent nine", false)
	requireUser()

	// A broken grant refuses every agent that uses it, and only them.
	invalidateGrant(t, gateway, vmcp.Name, "grant-one")
	requireRefused("agent seven")
	requireRefused("agent eight")
	requireGrant("agent nine", false)

	// Once revoked, no agent uses any.
	revokeGrant(t, gateway, vmcp.Name)
	requireGrant("agent seven", false)
	requireGrant("agent eight", false)

	// A grant authorized for another upstream is never sent to this one.
	storeGrant(t, gateway, vmcp.Name, "https://elsewhere.example.com/mcp", "grant-elsewhere")
	requireRefused("agent seven")
	requireRefused("agent eight")
	requireGrant("agent nine", false)
	requireUser()
}

// TestAgentConnectUsesTheVMCPGrant checks connect-time readiness and the composite configuration with the vMCP's agent
// OAuth grant: before any grant, every agent granted a component that requires OAuth is refused with oauth_required,
// and an agent whose profiles do not grant it is not; once authorized, every agent granted the component connects with
// the one grant, attributed to every profile that names it; a broken grant refuses them all with oauth_invalid, and a
// revoked one with oauth_required. No 409 names a profile.
func TestAgentConnectUsesTheVMCPGrant(t *testing.T) {
	vmcp, one, two := grantVMCP("vmcp1connectgrants", true)
	storageClient := newVMCPTestStorage(vmcp, one, two)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	manager, _ := newGrantTestManager(storageClient, gateway)
	refused := func(caller kuser.Info) vmcpaccess.AgentReadinessFailure {
		t.Helper()
		_, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, caller)
		notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
		require.True(t, ok, "connect error = %v, want an AgentNotReadyError", err)
		failure := notReady.Failure()
		encoded, err := json.Marshal(failure)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "profileID", "a 409 names a profile")
		return failure
	}
	failure := func(code string) vmcpaccess.AgentReadinessFailure {
		return vmcpaccess.AgentReadinessFailure{
			Error:   vmcpaccess.AgentOAuthRequiredError,
			Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: code}},
		}
	}
	owner, mcpID := grantKey(vmcp.Name)
	grant := &OAuthGrantRef{ComponentID: "one", Owner: owner, MCPID: mcpID}
	componentIDs := func(cfg ServerConfig) []string {
		var ids []string
		for _, component := range cfg.Components {
			ids = append(ids, component.ComponentID)
		}
		return ids
	}

	require.Equal(t, failure(types.AgentReadinessOAuthRequired), refused(asAgent(7, 11)))
	require.Equal(t, failure(types.AgentReadinessOAuthRequired), refused(asAgent(8, 12)))
	// Agent nine is granted only component two, so component one's OAuth never keeps it out.
	cfg := connectVMCP(t, manager, vmcp.Name, asAgent(9, 13))
	require.Equal(t, []string{"two"}, componentIDs(cfg))
	require.Nil(t, cfg.Components[0].OAuthGrant)
	require.Equal(t, grantNine, cfg.AuditLogMetadata[AuditLogProfiles])

	storeGrant(t, gateway, vmcp.Name, grantComponentURL, "grant-one")
	// Agent seven gets the union of its two profiles, and the vMCP's grant for component one.
	cfg = connectVMCP(t, manager, vmcp.Name, asAgent(7, 11))
	require.Equal(t, []string{"one", "two"}, componentIDs(cfg))
	require.Equal(t, grant, cfg.Components[0].OAuthGrant)
	require.Nil(t, cfg.Components[1].OAuthGrant)
	require.Equal(t, grantSeven+","+grantSevenTwo, cfg.AuditLogMetadata[AuditLogProfiles])
	// Agent eight, granted component one by another profile, uses the same grant.
	cfg = connectVMCP(t, manager, vmcp.Name, asAgent(8, 12))
	require.Equal(t, []string{"one"}, componentIDs(cfg))
	require.Equal(t, grant, cfg.Components[0].OAuthGrant)
	require.Equal(t, grantEight, cfg.AuditLogMetadata[AuditLogProfiles])

	invalidateGrant(t, gateway, vmcp.Name, "grant-one")
	require.Equal(t, failure(types.AgentReadinessOAuthInvalid), refused(asAgent(7, 11)))
	require.Equal(t, failure(types.AgentReadinessOAuthInvalid), refused(asAgent(8, 12)))

	revokeGrant(t, gateway, vmcp.Name)
	require.Equal(t, failure(types.AgentReadinessOAuthRequired), refused(asAgent(7, 11)))
	require.Equal(t, failure(types.AgentReadinessOAuthRequired), refused(asAgent(8, 12)))
	connectVMCP(t, manager, vmcp.Name, asAgent(9, 13))

	// A user connects as before: no grant is ever selected for them.
	cfg = connectVMCP(t, manager, vmcp.Name, principaltest.AsUser(&kuser.DefaultInfo{UID: "7"}))
	require.Equal(t, []string{"one", "two"}, componentIDs(cfg))
	require.Nil(t, cfg.Components[0].OAuthGrant)
	require.Equal(t, grantUsers, cfg.AuditLogMetadata[AuditLogProfiles])
}

// TestAgentReadinessReportAgreesWithConnect checks that what the readiness API reports, which is computed with the
// session manager (SessionManager.AgentReadiness) and with no instance, is what a connect decides for the agent through
// its own instance, through every state of the vMCP's grant, of its components and of its profiles. If they disagreed,
// the API would report an agent ready that a connect refuses.
func TestAgentReadinessReportAgreesWithConnect(t *testing.T) {
	vmcp, one, two := grantVMCP("vmcp1agree", true)
	storageClient := newVMCPTestStorage(vmcp, one, two)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	manager, _ := newGrantTestManager(storageClient, gateway)
	agree := func(label string, ready map[uint]bool) map[string][]vmcpaccess.ReadinessReason {
		t.Helper()
		return requireReadinessAgreesWithConnect(t, manager, currentVMCP(t, storageClient, vmcp.Name), label, ready)
	}
	reason := func(componentID, code, field string) []vmcpaccess.ReadinessReason {
		return []vmcpaccess.ReadinessReason{{ComponentID: componentID, Code: code, Field: field}}
	}

	reasons := agree("no grant", map[uint]bool{7: false, 8: false, 9: true})
	require.Equal(t, reason("one", types.AgentReadinessOAuthRequired, ""), reasons["agent:7"])
	require.Equal(t, reason("one", types.AgentReadinessOAuthRequired, ""), reasons["agent:8"])

	// One authorization on the vMCP makes every agent granted the component ready.
	storeGrant(t, gateway, vmcp.Name, grantComponentURL, "grant-one")
	agree("the vMCP's grant", map[uint]bool{7: true, 8: true, 9: true})

	// Component two now needs a value agents cannot be given: only the agents granted it are kept out.
	updateGrantVMCP(t, storageClient, vmcp.Name, func(current *v1.VMCP) {
		component := &current.Spec.Manifest.Components[1]
		component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Usage: types.Header, Required: true}}
		component.Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
	})
	reasons = agree("component two needs configuration", map[uint]bool{7: false, 8: true, 9: false})
	require.Equal(t, reason("two", types.AgentReadinessConfigurationRequired, "X-Token"), reasons["agent:7"])
	require.Equal(t, reason("two", types.AgentReadinessConfigurationRequired, "X-Token"), reasons["agent:9"])

	// Removing agent seven from the profile that grants it component two takes away only that profile's component.
	updateGrantVMCP(t, storageClient, vmcp.Name, func(current *v1.VMCP) {
		current.Spec.Manifest.Profiles = slices.DeleteFunc(current.Spec.Manifest.Profiles, func(profile types.VMCPProfile) bool {
			return profile.ID == grantSevenTwo
		})
	})
	agree("agent seven named by one profile", map[uint]bool{7: true, 8: true, 9: false})

	invalidateGrant(t, gateway, vmcp.Name, "grant-one")
	reasons = agree("the vMCP's grant broken", map[uint]bool{7: false, 8: false, 9: false})
	require.Equal(t, reason("one", types.AgentReadinessOAuthInvalid, ""), reasons["agent:7"])
	require.Equal(t, reason("one", types.AgentReadinessOAuthInvalid, ""), reasons["agent:8"])

	revokeGrant(t, gateway, vmcp.Name)
	reasons = agree("the vMCP's grant revoked", map[uint]bool{7: false, 8: false, 9: false})
	require.Equal(t, reason("one", types.AgentReadinessOAuthRequired, ""), reasons["agent:7"])
	require.Equal(t, reason("one", types.AgentReadinessOAuthRequired, ""), reasons["agent:8"])

	storeGrant(t, gateway, vmcp.Name, "https://elsewhere.example.com/mcp", "grant-elsewhere")
	reasons = agree("a grant authorized for another upstream", map[uint]bool{7: false, 8: false, 9: false})
	require.Equal(t, reason("one", types.AgentReadinessOAuthInvalid, ""), reasons["agent:8"])
}

// currentVMCP returns the vMCP with name as stored.
func currentVMCP(t *testing.T, storageClient storage.Client, name string) *v1.VMCP {
	t.Helper()
	current := &v1.VMCP{}
	require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, current))
	return current
}

// updateGrantVMCP changes the stored vMCP with name, and then does for every instance and connection what the
// VMCPInstance and MCPServerInstance controllers do for the change: it records each instance's configuration status
// for its owner, and synchronizes each connection's configuration with its component.
func updateGrantVMCP(t *testing.T, storageClient storage.Client, name string, change func(*v1.VMCP)) {
	t.Helper()
	current := currentVMCP(t, storageClient, name)
	change(current)
	require.NoError(t, storageClient.Update(t.Context(), current))

	var instances v1.VMCPInstanceList
	require.NoError(t, storageClient.List(t.Context(), &instances))
	for i := range instances.Items {
		instance := &instances.Items[i]
		syncedVMCPInstanceForUser(instance, current, ownerInfo(instance.Spec.UserID))
		require.NoError(t, storageClient.Update(t.Context(), instance))

		var connections v1.MCPServerInstanceList
		require.NoError(t, storageClient.List(t.Context(), &connections, kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name}))
		for j := range connections.Items {
			connection := &connections.Items[j]
			if !slices.ContainsFunc(current.Spec.Manifest.Components, func(component types.VMCPComponent) bool {
				return component.ID == connection.Spec.VMCPComponentID
			}) {
				continue
			}
			syncedVMCPConnection(t, connection, current, instance)
			require.NoError(t, storageClient.Update(t.Context(), connection))
		}
	}
}

// requireReadinessAgreesWithConnect requires that what the readiness API reports for each agent in ready, by ID, on
// vmcp is what a connect decides for it through its own instance, and that it is ready as wanted. It returns the
// reasons reported for each, by UID.
func requireReadinessAgreesWithConnect(t *testing.T, manager *SessionManager, vmcp *v1.VMCP, label string, ready map[uint]bool) map[string][]vmcpaccess.ReadinessReason {
	t.Helper()
	reasons := map[string][]vmcpaccess.ReadinessReason{}
	ids := make([]uint, 0, len(ready))
	for id := range ready {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		caller := asAgent(id, 10+id)
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
		require.Equal(t, ready[id], report.Readiness.Ready(), "%s: %s: %v", label, caller.GetUID(), report.Readiness.Reasons)
		reasons[caller.GetUID()] = report.Readiness.Reasons
	}
	return reasons
}

// TestAgentReadinessReportAgreesWithConnectForLateChallenges is TestAgentReadinessReportAgreesWithConnect for a
// component without static OAuth, which may only ask for OAuth once it is called. Readiness cannot see that until the
// vMCP holds a grant for it, so every agent is ready while it holds none. Once it holds one, the component is known to
// require OAuth, even once the grant broke, which keeps every agent granted the component out with oauth_invalid, as at
// connect; revoking it leaves nothing that shows the component uses OAuth.
func TestAgentReadinessReportAgreesWithConnectForLateChallenges(t *testing.T) {
	vmcp, one, two := grantVMCP("vmcp1agreelate", false)
	storageClient := newVMCPTestStorage(vmcp, one, two)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	manager, grants := newGrantTestManager(storageClient, gateway)
	agree := func(label string, ready bool) map[string][]vmcpaccess.ReadinessReason {
		t.Helper()
		return requireReadinessAgreesWithConnect(t, manager, vmcp, label, map[uint]bool{7: ready, 8: ready, 9: true})
	}
	// requiresOAuth reports whether the vMCP's agent setup, which agent readiness agrees with, counts component one as
	// requiring OAuth.
	requiresOAuth := func() bool {
		t.Helper()
		setup, err := vmcpaccess.AgentSetup(t.Context(), grants, *vmcp)
		require.NoError(t, err)
		require.Equal(t, "one", setup.Components[0].ComponentID)
		require.NotNil(t, setup.Components[0].OAuth)
		return setup.Components[0].OAuth.Required
	}

	agree("no grant", true)
	require.False(t, requiresOAuth())

	storeGrant(t, gateway, vmcp.Name, grantComponentURL, "grant-one")
	agree("the vMCP's grant", true)
	require.True(t, requiresOAuth(), "a component the vMCP holds a grant for does not count as requiring OAuth")

	invalidateGrant(t, gateway, vmcp.Name, "grant-one")
	reasons := agree("the vMCP's grant broken", false)
	require.Equal(t, []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthInvalid}}, reasons["agent:8"])
	require.True(t, requiresOAuth())

	revokeGrant(t, gateway, vmcp.Name)
	agree("the vMCP's grant revoked", true)
	require.False(t, requiresOAuth())
}
