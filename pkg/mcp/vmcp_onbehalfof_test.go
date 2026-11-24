package mcp

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// asAgentOnBehalfOf returns the principal of the agent with id acting on behalf of the user with userID, as the
// authenticator of delegated tokens builds it.
func asAgentOnBehalfOf(t *testing.T, id, keyID, userID uint) *kuser.DefaultInfo {
	t.Helper()

	info := asAgent(id, keyID)
	principal.RecordOnBehalfOf(info.Extra, userID)
	require.NoError(t, agentprofile.RecordDelegation(info.Extra, agentprofile.Delegation{
		ID:   "adl1test",
		User: subjects.Caller{Principal: principal.User(userID)},
	}))
	return info
}

// TestAgentConnectOnBehalfOfAUser checks that an agent acting on behalf of a user connects through the agent's own
// instance, with the tools of every vMCP profile that names the agent, and never of one that names the user, and that the
// call is attributed to the agent's profiles.
func TestAgentConnectOnBehalfOfAUser(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1onbehalfof",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "On behalf of",
			Components:  []types.VMCPComponent{{ID: "one", Name: "one"}, {ID: "two", Name: "two"}},
			Profiles: []types.VMCPProfile{
				{
					ID:       "vp1read",
					Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
					Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
						"one": {AllowedTools: []string{"read"}},
					}},
				},
				{
					ID:       "vp1write",
					Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
					Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
						"one": {AllowedTools: []string{"write"}},
						"two": {AllowedTools: []string{"deploy"}},
					}},
				},
				{
					ID:          "vp1users",
					Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}, {Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				},
			},
		}},
	}
	sharedOne := vmcpComponentServer("ms1shared-one", "", "", "one", "one", "https://one.example.com/mcp")
	sharedOne.Spec.VMCPID = vmcp.Name
	sharedTwo := vmcpComponentServer("ms1shared-two", "", "", "two", "two", "https://two.example.com/mcp")
	sharedTwo.Spec.VMCPID = vmcp.Name
	storageClient := newVMCPTestStorage(vmcp, sharedOne, sharedTwo)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	manager := newAgentTestManager(storageClient)

	toolsByComponent := func(cfg ServerConfig) map[string][]string {
		tools := map[string][]string{}
		for _, component := range cfg.Components {
			for _, tool := range component.Tools {
				tools[component.ComponentID] = append(tools[component.ComponentID], tool.Name)
			}
			slices.Sort(tools[component.ComponentID])
		}
		return tools
	}

	own := connectVMCP(t, manager, vmcp.Name, asAgent(5, 11))
	forSeven := connectVMCP(t, manager, vmcp.Name, asAgentOnBehalfOf(t, 5, 11, 7))
	require.Equal(t, own.MCPServerName, forSeven.MCPServerName, "the agent's own instance")
	require.Equal(t, principal.Agent(5).String(), forSeven.UserID)
	if got, want := toolsByComponent(forSeven), map[string][]string{"one": {"read", "write"}, "two": {"deploy"}}; !equalToolMaps(got, want) {
		t.Fatalf("tools on behalf of a user = %v, want %v", got, want)
	}
	if got, want := toolsByComponent(forSeven), toolsByComponent(own); !equalToolMaps(got, want) {
		t.Fatalf("tools on behalf of a user = %v, want the agent's own %v", got, want)
	}
	require.Equal(t, "vp1read,vp1write", forSeven.AuditLogMetadata[AuditLogProfiles])
}

// TestAgentConnectOnBehalfOfAUserNeedsApproval checks that a component the agent is granted, which needs something the
// user would set up, refuses the agent acting on behalf of the user with agent_user_approval_required, naming the vMCP
// and the component.
func TestAgentConnectOnBehalfOfAUserNeedsApproval(t *testing.T) {
	vmcp, shared := sharedComponentVMCP("vmcp1approval",
		types.VMCPProfile{
			ID:          "vp1needs",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
	)
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Usage: types.Header, Required: true}}
	vmcp.Spec.Manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp)
	manager := newAgentTestManager(storageClient)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, err := manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgentOnBehalfOf(t, 5, 11, 7))
	notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
	require.True(t, ok, "connect error = %v, want an AgentNotReadyError", err)
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		VMCPID:  vmcp.Name,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessConfigurationRequired, Field: "X-Token"}},
	}, notReady.Failure())

	// The agent acting for itself is told an administrator must fix it, as before.
	_, err = manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgent(5, 11))
	notReady, ok = errors.AsType[*vmcpaccess.AgentNotReadyError](err)
	require.True(t, ok, "connect error = %v, want an AgentNotReadyError", err)
	require.Equal(t, vmcpaccess.AgentConfigurationRequiredError, notReady.Failure().Error)
	require.Empty(t, notReady.Failure().VMCPID)
}

// TestAgentOnBehalfOfRunsComponentsWhereTheyNeedTheUser checks where an agent acting on behalf of two users reaches each
// component of a vMCP: a component that needs nothing from the user, because the vMCP fixes it or its preconfigured
// OAuth covers it, through the agent's own single instance, which every user shares, with the grant; a single-user component
// that takes a value from the user through a server of each pair's own; and a shared component that takes a header
// from the user through each pair's own connection. At the component hop, a pair's server and connection are the pair's,
// with its upstream token, to that agent acting for that user only.
func TestAgentOnBehalfOfRunsComponentsWhereTheyNeedTheUser(t *testing.T) {
	remote := func(id string, singleUser bool, userKey string, usage types.Usage) types.VMCPComponent {
		component := types.VMCPComponent{
			ID:              id,
			Name:            id,
			ForceSingleUser: singleUser,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://" + id + ".example.com/mcp"},
			}},
		}
		if userKey != "" {
			// Optional, so that nothing keeps the agent from connecting before the user sets it.
			component.Configuration = []types.VMCPConfigurationPolicy{{Key: userKey, Policy: types.VMCPConfigurationPolicyUserAllowed}}
			component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: userKey, Usage: usage}}
		}
		return component
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1placement",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Placement",
			Components: []types.VMCPComponent{
				{ID: "covered", Name: "covered"},
				{ID: "fixed-dedicated", Name: "fixed-dedicated", ForceSingleUser: true},
				remote("granted", false, "", ""),
				remote("headers", false, "X-Team", types.Header),
				remote("dedicated", true, "TOKEN", types.Env),
			},
			Profiles: []types.VMCPProfile{{
				ID:          "vp1agent",
				Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}},
		}},
	}
	objects := []kclient.Object{vmcp}
	for _, id := range []string{"covered", "granted", "headers"} {
		shared := vmcpComponentServer("ms1shared-"+id, "", "", id, id, "https://"+id+".example.com/mcp")
		shared.Spec.VMCPID = vmcp.Name
		objects = append(objects, shared)
	}
	storageClient := newVMCPTestStorage(objects...)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	manager, _ := newGrantTestManager(storageClient, gateway)
	manager.gatewayClient = gateway
	manager.backend = &kubernetesBackend{}
	manager.baseURL = "https://obot.example.com"
	t.Cleanup(manager.Close)
	grantOwner, grantMCPID := principal.PreconfiguredOAuthOwner(vmcp.Name), vmcpaccess.PreconfiguredOAuthMCPID(vmcp.Name, "granted")
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), grantOwner, grantMCPID, "https://granted.example.com/mcp", "grant-granted", "1",
		&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "granted", Expiry: time.Now().Add(time.Hour)}))

	forSeven := asAgentOnBehalfOf(t, 5, 11, 7)
	forEight := asAgentOnBehalfOf(t, 5, 11, 8)
	itself := asAgent(5, 11)
	byComponent := func(cfg ServerConfig) map[string]ComponentServer {
		components := map[string]ComponentServer{}
		for _, component := range cfg.Components {
			components[component.ComponentID] = component
		}
		return components
	}
	own := connectVMCP(t, manager, vmcp.Name, itself)
	seven := connectVMCP(t, manager, vmcp.Name, forSeven)
	eight := connectVMCP(t, manager, vmcp.Name, forEight)
	instance := func(owner string) *v1.VMCPInstance {
		t.Helper()
		found, err := vmcpaccess.FindInstance(t.Context(), storageClient, system.DefaultNamespace, vmcp.Name, owner)
		require.NoError(t, err)
		require.NotNil(t, found, "no instance for %s", owner)
		return found
	}
	agentInstance := instance("agent:5")
	pairSeven, pairEight := instance("agent:5/user:7"), instance("agent:5/user:8")
	require.Equal(t, own.MCPServerName, seven.MCPServerName, "the agent connects through its own instance for every user")
	require.Equal(t, own.MCPServerName, eight.MCPServerName)

	ownComponents, sevenComponents, eightComponents := byComponent(own), byComponent(seven), byComponent(eight)
	for _, id := range []string{"covered", "fixed-dedicated", "granted"} {
		require.Equal(t, ownComponents[id].ConnectID(), sevenComponents[id].ConnectID(), "%s is the agent's for user 7", id)
		require.Equal(t, ownComponents[id].ConnectID(), eightComponents[id].ConnectID(), "%s is the agent's for user 8", id)
	}
	require.Equal(t, vmcpaccess.ComponentServerName(agentInstance.Name, "fixed-dedicated"), sevenComponents["fixed-dedicated"].Name)
	grant := &OAuthGrantRef{ComponentID: "granted", Owner: grantOwner, MCPID: grantMCPID}
	require.Equal(t, grant, sevenComponents["granted"].OAuthGrant)
	require.Equal(t, grant, eightComponents["granted"].OAuthGrant)

	require.Equal(t, vmcpaccess.ComponentConnectionName(pairSeven.Name, "headers"), sevenComponents["headers"].ConnectID())
	require.Equal(t, vmcpaccess.ComponentConnectionName(pairEight.Name, "headers"), eightComponents["headers"].ConnectID())
	require.Equal(t, vmcpaccess.ComponentServerName(pairSeven.Name, "dedicated"), sevenComponents["dedicated"].ConnectID())
	require.Equal(t, vmcpaccess.ComponentServerName(pairEight.Name, "dedicated"), eightComponents["dedicated"].ConnectID())
	for _, id := range []string{"headers", "dedicated"} {
		require.NotEqual(t, ownComponents[id].ConnectID(), sevenComponents[id].ConnectID(), "%s is the pair's, not the agent's", id)
		require.Nil(t, sevenComponents[id].OAuthGrant)
	}

	// At the component hop, the pair's connection and server are used as the pair's: its configuration and its upstream
	// token, never the agent's or the user's own.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, id := range []string{"headers", "dedicated"} {
		_, cfg, err := manager.ServerForAction(ctx, sevenComponents[id].ConnectID(), forSeven)
		require.NoError(t, err, id)
		require.Equal(t, "agent:5/user:7", cfg.UserID, id)
		owner, mcpID, ok := cfg.OAuthTokenKey()
		require.True(t, ok, id)
		require.Equal(t, "agent:5/user:7", owner, id)
		require.Equal(t, sevenComponents[id].ConnectID(), mcpID, id)

		// Nobody else reaches it: the agent acting for another user, or for itself.
		for _, other := range []kuser.Info{forEight, itself} {
			_, _, err := manager.ServerForAction(ctx, sevenComponents[id].ConnectID(), other)
			require.Error(t, err, "%s reached the pair's %s", other.GetUID(), id)
		}
	}
	// The agent's own connection keeps the agent as its owner on behalf of a user, with the vMCP's grant.
	_, cfg, err := manager.ServerForAction(ctx, sevenComponents["granted"].ConnectID(), forSeven)
	require.NoError(t, err)
	require.Equal(t, "agent:5", cfg.UserID)
	require.Equal(t, grant, cfg.OAuthGrant)
}

// TestAgentOnBehalfOfNeedsTheUsersAccount checks that a component known to ask for OAuth, which the vMCP holds no agent
// grant for, refuses the agent acting on behalf of a user with agent_user_approval_required until the user connects an
// account through the agent, and that the account is the pair's alone: the user's own token for their own connection,
// and another user's through the agent, never count.
func TestAgentOnBehalfOfNeedsTheUsersAccount(t *testing.T) {
	vmcp, one, two := grantVMCP("vmcp1pairoauth", true)
	vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, types.VMCPProfile{
		ID:          "vp1five",
		Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	})
	storageClient := newVMCPTestStorage(vmcp, one, two)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	manager, _ := newGrantTestManager(storageClient, gateway)
	forSeven := asAgentOnBehalfOf(t, 5, 11, 7)

	refused := func() vmcpaccess.AgentReadinessFailure {
		t.Helper()
		_, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, forSeven)
		notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
		require.True(t, ok, "connect error = %v, want an AgentNotReadyError", err)
		return notReady.Failure()
	}
	want := vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		VMCPID:  vmcp.Name,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthRequired}},
	}
	require.Equal(t, want, refused())

	pair, err := vmcpaccess.FindInstance(t.Context(), storageClient, system.DefaultNamespace, vmcp.Name, "agent:5/user:7")
	require.NoError(t, err)
	require.NotNil(t, pair, "the connect created the pair's instance, for the user to set up")
	connection := vmcpaccess.ComponentConnectionName(pair.Name, "one")
	conf := &oauth2.Config{ClientID: "client"}
	token := &oauth2.Token{AccessToken: "account", Expiry: time.Now().Add(time.Hour)}

	// The user's own account, and another user's through the agent, are not this pair's.
	require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "7", connection, grantComponentURL, "", conf, token))
	require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "agent:5/user:8", connection, grantComponentURL, "", conf, token))
	require.Equal(t, want, refused())

	// Once the user connects their account through the agent, the agent connects, through the pair's connection.
	require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "agent:5/user:7", connection, grantComponentURL, "", conf, token))
	cfg := connectVMCP(t, manager, vmcp.Name, forSeven)
	for _, component := range cfg.Components {
		if component.ComponentID == "one" {
			require.Equal(t, connection, component.ConnectID())
			require.Nil(t, component.OAuthGrant)
		}
	}

	// The vMCP's grant, once authorized, is used in place of the user's account.
	storeGrant(t, gateway, vmcp.Name, grantComponentURL, "grant-one")
	cfg = connectVMCP(t, manager, vmcp.Name, forSeven)
	owner, mcpID := grantKey(vmcp.Name)
	for _, component := range cfg.Components {
		if component.ComponentID == "one" {
			require.Equal(t, &OAuthGrantRef{ComponentID: "one", Owner: owner, MCPID: mcpID}, component.OAuthGrant)
			require.NotEqual(t, connection, component.ConnectID(), "a component the grant covers runs on the agent's own instance")
		}
	}
}
