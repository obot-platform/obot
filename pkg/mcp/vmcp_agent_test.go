package mcp

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// noGrants is a gateway whose vMCP profiles hold no OAuth grant.
type noGrants struct{}

func (noGrants) MCPOAuthGrantMetadata(context.Context, []string, []string) ([]gatewaytypes.MCPOAuthGrantMetadata, error) {
	return nil, nil
}

func (noGrants) MCPOAuthGrantOwners(context.Context, []string) ([]string, error) {
	return nil, nil
}

func (noGrants) DeleteMCPOAuthGrants(context.Context, string, string, []string) error {
	return nil
}

func (noGrants) DeleteMCPOAuthGrantsForOwners(context.Context, []string) error {
	return nil
}

// newAgentTestManager returns a session manager that connects to vMCPs stored in storageClient, whose profiles hold no
// OAuth grant.
func newAgentTestManager(storageClient storage.Client) *SessionManager {
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}
	manager.SetProfileOAuthGrants(vmcpaccess.NewProfileOAuthGrantStore(noGrants{}))
	return manager
}

// asAgent returns the principal of the agent with id, as the API key authenticator builds it for the agent's key
// keyID.
func asAgent(id, keyID uint) *kuser.DefaultInfo {
	extra := map[string][]string{
		principal.APIKeyIDExtra: {strconv.FormatUint(uint64(keyID), 10)},
	}
	principal.RecordKind(extra, principal.KindAgent)
	principal.RecordUserStatus(extra, types.UserStatusActive)
	uid := principal.Agent(id).String()
	return &kuser.DefaultInfo{
		Name:   uid,
		UID:    uid,
		Groups: []string{types.GroupMCP, types.GroupAuthenticated},
		Extra:  extra,
	}
}

// ownerInfo returns the principal of an instance's owner, as the gateway client's IdentityInfo loads it.
func ownerInfo(owner string) kuser.Info {
	if p, err := principal.Parse(owner); err == nil {
		if id, ok := p.AgentID(); ok {
			return asAgent(id, 0)
		}
	}
	return principaltest.AsUser(&kuser.DefaultInfo{UID: owner})
}

// runVMCPInstanceController does for every instance created in storage what the VMCPInstance and MCPServerInstance
// controllers do for a vMCP whose components are all shared: it records the instance's configuration status, as
// evaluated for its owner, and creates the owner's connection to each shared component server.
func runVMCPInstanceController(t *testing.T, storageClient storage.Client, vmcp *v1.VMCP) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			var instances v1.VMCPInstanceList
			if err := storageClient.List(ctx, &instances); err != nil {
				continue
			}
			for i := range instances.Items {
				instance := &instances.Items[i]
				if instance.Status.ConfigurationCheckHash != "" {
					continue
				}
				owner := ownerInfo(instance.Spec.UserID)
				for _, component := range vmcpaccess.EnabledComponents(owner, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, *instance)) {
					config := vmcpaccess.ConnectionConfiguration(component)
					connection := &v1.MCPServerInstance{
						Name:      "msi1" + instance.Name + "-" + component.ID,
						Namespace: system.DefaultNamespace,
						Spec: v1.MCPServerInstanceSpec{
							UserID:          instance.Spec.UserID,
							VMCPInstanceID:  instance.Name,
							VMCPComponentID: component.ID,
							MCPServerName:   "ms1shared-" + component.ID,
							Config:          config,
						},
					}
					connection.Status.VMCPConfigurationHash = vmcpaccess.ConnectionConfigurationHash(config, *instance)
					_ = storageClient.Create(ctx, connection)
				}
				syncedVMCPInstanceForUser(instance, vmcp, owner)
				_ = storageClient.Update(ctx, instance)
			}
		}
	}()
}

// sharedComponentVMCP returns a shared vMCP with one shared component that needs nothing from its callers, and its
// shared component server.
func sharedComponentVMCP(name string, profiles ...types.VMCPProfile) (*v1.VMCP, *v1.MCPServer) {
	vmcp := &v1.VMCP{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Shared",
			Components:  []types.VMCPComponent{{ID: "one", Name: "one"}},
			Profiles:    profiles,
		}},
	}
	shared := vmcpComponentServer("ms1shared-one", "", "", "one", "one", "https://one.example.com/mcp")
	shared.Spec.VMCPID = vmcp.Name
	return vmcp, shared
}

func connectVMCP(t *testing.T, manager *SessionManager, vmcpID string, caller kuser.Info) ServerConfig {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cfg, err := manager.ServerConfigForVMCP(ctx, vmcpID, caller)
	if err != nil {
		t.Fatalf("connect %s: %v", caller.GetUID(), err)
	}
	return cfg
}

// TestVMCPConnectionsAreIsolatedPerPrincipal connects a user, an agent with the user's numeric ID through two of its
// keys, and another agent to one shared vMCP. Each principal gets its own instance, connection, credentials, client
// cache key and session owner, and the agent's two keys share the agent's one instance.
func TestVMCPConnectionsAreIsolatedPerPrincipal(t *testing.T) {
	vmcp, shared := sharedComponentVMCP("vmcp1isolation",
		types.VMCPProfile{
			ID:          "vp1users",
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
		types.VMCPProfile{
			ID:          "vp1agents",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}, {Type: types.SubjectTypeAgent, ID: "6"}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
	)
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp)
	manager := newAgentTestManager(storageClient)

	user := connectVMCP(t, manager, vmcp.Name, principaltest.AsUser(&kuser.DefaultInfo{UID: "5"}))
	agentFirstKey := connectVMCP(t, manager, vmcp.Name, asAgent(5, 11))
	agentSecondKey := connectVMCP(t, manager, vmcp.Name, asAgent(5, 12))
	otherAgent := connectVMCP(t, manager, vmcp.Name, asAgent(6, 13))

	var instances v1.VMCPInstanceList
	if err := storageClient.List(t.Context(), &instances); err != nil {
		t.Fatal(err)
	}
	owners := map[string]string{}
	for _, instance := range instances.Items {
		if _, ok := owners[instance.Spec.UserID]; ok {
			t.Fatalf("owner %q has more than one instance", instance.Spec.UserID)
		}
		owners[instance.Spec.UserID] = instance.Name
	}
	if len(owners) != 3 || owners["5"] == "" || owners["agent:5"] == "" || owners["agent:6"] == "" {
		t.Fatalf("instance owners = %v, want one each for user 5, agent 5 and agent 6", owners)
	}

	// The connection is the caller's instance, and the configuration is the caller's.
	for _, tt := range []struct {
		name  string
		cfg   ServerConfig
		owner string
	}{
		{
			name:  "user",
			cfg:   user,
			owner: "5",
		},
		{
			name:  "agent, first key",
			cfg:   agentFirstKey,
			owner: "agent:5",
		},
		{
			name:  "agent, second key",
			cfg:   agentSecondKey,
			owner: "agent:5",
		},
		{
			name:  "other agent",
			cfg:   otherAgent,
			owner: "agent:6",
		},
	} {
		if tt.cfg.MCPServerName != owners[tt.owner] || tt.cfg.UserID != tt.owner || tt.cfg.AuditLogMetadata["userID"] != tt.owner {
			t.Errorf("%s: connection %q for %q (audit %q), want instance %q for %q", tt.name, tt.cfg.MCPServerName, tt.cfg.UserID, tt.cfg.AuditLogMetadata["userID"], owners[tt.owner], tt.owner)
		}
		if len(tt.cfg.Components) != 1 || tt.cfg.Components[0].ComponentID != "one" {
			t.Errorf("%s: components = %#v, want component one", tt.name, tt.cfg.Components)
		}
	}

	// Shared components are reached through each principal's own connection, whose credential is its own.
	connections := map[string]v1.MCPServerInstance{}
	var list v1.MCPServerInstanceList
	if err := storageClient.List(t.Context(), &list); err != nil {
		t.Fatal(err)
	}
	for _, connection := range list.Items {
		connections[connection.Name] = connection
	}
	credentialContexts := map[string]string{}
	for owner, cfg := range map[string]ServerConfig{"5": user, "agent:5": agentFirstKey, "agent:6": otherAgent} {
		connection, ok := connections[cfg.Components[0].MCPServerInstanceID]
		if !ok || connection.Spec.UserID != owner {
			t.Fatalf("%s reaches the shared component through %q, owned by %q", owner, cfg.Components[0].MCPServerInstanceID, connection.Spec.UserID)
		}
		context := serverInstanceCredentialContext(connection)
		if other, ok := credentialContexts[context]; ok {
			t.Fatalf("%s and %s share the connection credential context %q", owner, other, context)
		}
		credentialContexts[context] = owner
		if !strings.HasPrefix(context, owner+"-") {
			t.Fatalf("%s connection credential context = %q, want it scoped to its owner", owner, context)
		}
		if got := vmcpaccess.InstanceConfigurationCredentialContext(cfg.MCPServerName); got == vmcpaccess.InstanceConfigurationCredentialContext(user.MCPServerName) && owner != "5" {
			t.Fatalf("%s shares the user's instance configuration credential %q", owner, got)
		}
	}
	if agentFirstKey.Components[0].URL != agentSecondKey.Components[0].URL {
		t.Fatal("two keys of one agent reached the shared component through different connections")
	}

	// Cached MCP clients are keyed by the caller, so the user and the agent with its numeric ID never share one.
	if clientID(user, "scope") == clientID(agentFirstKey, "scope") {
		t.Fatal("the user and the agent with the user's ID share an MCP client cache key")
	}
	if clientID(agentFirstKey, "scope") != clientID(agentSecondKey, "scope") {
		t.Fatal("two keys of one agent do not share the agent's MCP client cache key")
	}
}

// TestAgentConnectRefusedUntilReady checks that an agent is refused, with the reasons, when a component enabled for
// it needs setup that its profile does not give it, while a user still connects to the same vMCP through the profile
// that names every user, which never applies to the agent.
func TestAgentConnectRefusedUntilReady(t *testing.T) {
	profiles := []types.VMCPProfile{
		{
			ID:          "vp1agent",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
		{
			ID:          "vp1everyone",
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
	}
	for _, tt := range []struct {
		name        string
		configure   func(*types.VMCPComponent)
		wantError   string
		wantReasons []vmcpaccess.ReadinessReason
	}{
		{
			name: "header a user would enter",
			configure: func(component *types.VMCPComponent) {
				component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Usage: types.Header, Required: true}}
				component.Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
			},
			wantError: vmcpaccess.AgentConfigurationRequiredError,
			wantReasons: []vmcpaccess.ReadinessReason{{
				ComponentID: "one",
				Code:        types.AgentReadinessConfigurationRequired,
				Field:       "X-Token",
				ProfileID:   "vp1agent",
			}},
		},
		{
			name: "static OAuth",
			configure: func(component *types.VMCPComponent) {
				component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
				component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://one.example.com/mcp", StaticOAuthRequired: true}
			},
			wantError: vmcpaccess.AgentOAuthRequiredError,
			wantReasons: []vmcpaccess.ReadinessReason{{
				ComponentID: "one",
				Code:        types.AgentReadinessOAuthRequired,
				ProfileID:   "vp1agent",
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp, shared := sharedComponentVMCP("vmcp1notready", profiles...)
			tt.configure(&vmcp.Spec.Manifest.Components[0])
			storageClient := newVMCPTestStorage(vmcp, shared)
			runVMCPInstanceController(t, storageClient, vmcp)
			manager := newAgentTestManager(storageClient)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			_, err := manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgent(5, 11))
			notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
			if !ok {
				t.Fatalf("agent connect error = %v, want an AgentNotReadyError", err)
			}
			if failure := notReady.Failure(); failure.Error != tt.wantError || !slices.Equal(failure.Reasons, tt.wantReasons) {
				t.Fatalf("failure = %#v, want %q with %#v", failure, tt.wantError, tt.wantReasons)
			}

			// A second connect is refused again: readiness is never cached as ready, nor as not ready.
			if _, err := manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgent(5, 12)); !errors.As(err, &notReady) {
				t.Fatalf("second agent connect error = %v, want an AgentNotReadyError", err)
			}

			// Users are not held to agent readiness. The user with the agent's numeric ID never matches the agent's
			// profile.
			cfg := connectVMCP(t, manager, vmcp.Name, principaltest.AsUser(&kuser.DefaultInfo{UID: "5"}))
			if len(cfg.Components) != 1 || cfg.AuditLogMetadata[AuditLogProfiles] != "vp1everyone" {
				t.Fatalf("user connection = %#v, want the component, attributed to the profile for every user", cfg)
			}
		})
	}
}

// TestSelectorWildcardProfileNeverAppliesToAgents checks that the profile that names every user ({selector, "*"})
// grants an agent nothing: an agent named in another profile gets that profile's tools only, and is attributed to it
// alone, and an agent named in no profile is given no component of the vMCP. The MCP gateway's authorization refuses
// such an agent before it connects (authz.UserCanReadVMCP); the session manager never widens it to the wildcard.
func TestSelectorWildcardProfileNeverAppliesToAgents(t *testing.T) {
	vmcp, shared := sharedComponentVMCP("vmcp1wildcard",
		types.VMCPProfile{
			ID:       "vp1agent",
			Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
				"one": {AllowedTools: []string{"read"}},
			}},
		},
		types.VMCPProfile{
			ID:       "vp1everyone",
			Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
				"one": {AllowedTools: []string{"write"}},
			}},
		},
	)
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp)
	manager := newAgentTestManager(storageClient)

	toolNames := func(cfg ServerConfig) []string {
		var tools []string
		for _, component := range cfg.Components {
			for _, tool := range component.Tools {
				tools = append(tools, tool.Name)
			}
		}
		return tools
	}

	agent := connectVMCP(t, manager, vmcp.Name, asAgent(5, 11))
	if len(agent.Components) != 1 || !slices.Equal(toolNames(agent), []string{"read"}) {
		t.Fatalf("agent tools = %v, want only its own profile's", toolNames(agent))
	}
	if agent.AuditLogMetadata[AuditLogProfiles] != "vp1agent" {
		t.Fatalf("agent profiles = %q, want only the profile that names it", agent.AuditLogMetadata[AuditLogProfiles])
	}

	user := connectVMCP(t, manager, vmcp.Name, principaltest.AsUser(&kuser.DefaultInfo{UID: "5"}))
	if !slices.Equal(toolNames(user), []string{"write"}) || user.AuditLogMetadata[AuditLogProfiles] != "vp1everyone" {
		t.Fatalf("user connection = %#v, want the profile for every user", user)
	}

	unnamed := connectVMCP(t, manager, vmcp.Name, asAgent(6, 12))
	if len(unnamed.Components) != 0 || unnamed.AuditLogMetadata[AuditLogProfiles] != "" {
		t.Fatalf("unnamed agent connection = %#v, want no component and no profile", unnamed)
	}
}
