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
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

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
			Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
		types.VMCPProfile{
			ID:          "vp1agents",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
	)
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}

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
// it needs setup that agents cannot be given yet, and on a vMCP whose wildcard profiles make its selection ambiguous,
// while a user still connects to the same vMCP.
func TestAgentConnectRefusedUntilReady(t *testing.T) {
	everyone := func(id string) types.VMCPProfile {
		return types.VMCPProfile{
			ID:          id,
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		}
	}
	explicit := func(id string) types.VMCPProfile {
		return types.VMCPProfile{
			ID:          id,
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		}
	}
	for _, tt := range []struct {
		name        string
		profiles    []types.VMCPProfile
		configure   func(*types.VMCPComponent)
		wantError   string
		wantReasons []vmcpaccess.ReadinessReason
	}{
		{
			name:      "legacy wildcard profiles",
			profiles:  []types.VMCPProfile{everyone("vp1first"), everyone("vp1second")},
			wantError: vmcpaccess.AgentSelectionAmbiguousError,
			wantReasons: []vmcpaccess.ReadinessReason{{
				Code: vmcpaccess.ReadinessSelectionAmbiguous,
			}},
		},
		{
			// An agent with its own profile falls back for the configuration only agents' fallback could supply, and
			// that fallback is ambiguous.
			name:     "legacy wildcard profiles with an explicit profile and a header a user would enter",
			profiles: []types.VMCPProfile{explicit("vp1agent"), everyone("vp1first"), everyone("vp1second")},
			configure: func(component *types.VMCPComponent) {
				component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Usage: types.Header, Required: true}}
				component.Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
			},
			wantError: vmcpaccess.AgentSelectionAmbiguousError,
			wantReasons: []vmcpaccess.ReadinessReason{{
				ComponentID: "one",
				Code:        vmcpaccess.ReadinessSelectionAmbiguous,
			}},
		},
		{
			name:     "header a user would enter",
			profiles: []types.VMCPProfile{everyone("vp1everyone")},
			configure: func(component *types.VMCPComponent) {
				component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Usage: types.Header, Required: true}}
				component.Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
			},
			wantError: vmcpaccess.AgentConfigurationRequiredError,
			wantReasons: []vmcpaccess.ReadinessReason{{
				ComponentID: "one",
				Code:        vmcpaccess.ReadinessConfigurationRequired,
				Field:       "X-Token",
				ProfileID:   "vp1everyone",
			}},
		},
		{
			name:     "static OAuth",
			profiles: []types.VMCPProfile{everyone("vp1everyone")},
			configure: func(component *types.VMCPComponent) {
				component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
				component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://one.example.com/mcp", StaticOAuthRequired: true}
			},
			wantError: vmcpaccess.AgentOAuthRequiredError,
			wantReasons: []vmcpaccess.ReadinessReason{{
				ComponentID: "one",
				Code:        vmcpaccess.ReadinessOAuthRequired,
				ProfileID:   "vp1everyone",
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp, shared := sharedComponentVMCP("vmcp1notready", tt.profiles...)
			if tt.configure != nil {
				tt.configure(&vmcp.Spec.Manifest.Components[0])
			}
			storageClient := newVMCPTestStorage(vmcp, shared)
			runVMCPInstanceController(t, storageClient, vmcp)
			manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			_, err := manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgent(5, 11))
			notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
			if !ok {
				t.Fatalf("agent connect error = %v, want an AgentNotReadyError", err)
			}
			if failure := notReady.Failure(); failure.Error != tt.wantError || len(failure.Reasons) != len(tt.wantReasons) || failure.Reasons[0] != tt.wantReasons[0] {
				t.Fatalf("failure = %#v, want %q with %#v", failure, tt.wantError, tt.wantReasons)
			}

			// A second connect is refused again: readiness is never cached as ready, nor as not ready.
			if _, err := manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgent(5, 12)); !errors.As(err, &notReady) {
				t.Fatalf("second agent connect error = %v, want an AgentNotReadyError", err)
			}

			// Users are not held to agent readiness: on the legacy vMCP, a user connects as before.
			if strings.HasPrefix(tt.name, "legacy wildcard profiles") {
				cfg := connectVMCP(t, manager, vmcp.Name, principaltest.AsUser(&kuser.DefaultInfo{UID: "5"}))
				// The user with the agent's numeric ID never matches the agent's profile.
				if len(cfg.Components) != 1 || cfg.AuditLogMetadata[AuditLogProfiles] != "vp1first,vp1second" {
					t.Fatalf("user connection = %#v, want the component, attributed to both profiles", cfg)
				}
			}
		})
	}
}

// TestAgentConnectReadinessFollowsTheVMCP checks that readiness is computed on every connect: once the component that
// needed setup no longer does, the agent's next connect succeeds.
func TestAgentConnectReadinessFollowsTheVMCP(t *testing.T) {
	vmcp, shared := sharedComponentVMCP("vmcp1fixed", types.VMCPProfile{
		ID:          "vp1agent",
		Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	})
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Runtime = types.RuntimeRemote
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://one.example.com/mcp", StaticOAuthRequired: true}
	storageClient := newVMCPTestStorage(vmcp, shared)
	controlled := vmcp.DeepCopy()
	runVMCPInstanceController(t, storageClient, controlled)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}

	if _, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, asAgent(5, 11)); !errors.As(err, new(*vmcpaccess.AgentNotReadyError)) {
		t.Fatalf("agent connect error = %v, want an AgentNotReadyError", err)
	}

	var current v1.VMCP
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &current); err != nil {
		t.Fatal(err)
	}
	current.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.StaticOAuthRequired = false
	if err := storageClient.Update(t.Context(), &current); err != nil {
		t.Fatal(err)
	}
	// The agent's instance was created by the refused connect; the controller recomputes its status for the change.
	var instances v1.VMCPInstanceList
	if err := storageClient.List(t.Context(), &instances); err != nil {
		t.Fatal(err)
	}
	if len(instances.Items) != 1 || instances.Items[0].Spec.UserID != "agent:5" {
		t.Fatalf("instances = %#v, want the agent's", instances.Items)
	}
	syncedVMCPInstanceForUser(&instances.Items[0], &current, asAgent(5, 0))
	if err := storageClient.Update(t.Context(), &instances.Items[0]); err != nil {
		t.Fatal(err)
	}

	cfg := connectVMCP(t, manager, vmcp.Name, asAgent(5, 11))
	if cfg.MCPServerName != instances.Items[0].Name || cfg.AuditLogMetadata[AuditLogProfiles] != "vp1agent" || cfg.AuditLogMetadata[AuditLogVMCPID] != vmcp.Name {
		t.Fatalf("agent connection = %#v, want the agent's instance attributed to its profile", cfg)
	}
}

// TestExplicitAgentConnectsToLegacyVMCP checks that an agent with its own profile connects to a vMCP whose wildcard
// profiles make the fallback ambiguous, when none of its components needs anything from that fallback. Its tools are
// still the union of every profile that names it.
func TestExplicitAgentConnectsToLegacyVMCP(t *testing.T) {
	vmcp, shared := sharedComponentVMCP("vmcp1legacy",
		types.VMCPProfile{
			ID:       "vp1agent",
			Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
				"one": {AllowedTools: []string{"read"}},
			}},
		},
		types.VMCPProfile{
			ID:       "vp1first",
			Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
				"one": {AllowedTools: []string{"write"}},
			}},
		},
		types.VMCPProfile{
			ID:       "vp1second",
			Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
		},
	)
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp)
	manager := &SessionManager{storageClient: storageClient, httpListenPort: vmcpTestListenPort}

	cfg := connectVMCP(t, manager, vmcp.Name, asAgent(5, 11))
	if len(cfg.Components) != 1 || cfg.Components[0].ComponentID != "one" {
		t.Fatalf("components = %#v, want component one", cfg.Components)
	}
	var tools []string
	for _, tool := range cfg.Components[0].Tools {
		tools = append(tools, tool.Name)
	}
	if !slices.Equal(tools, []string{"read", "write"}) {
		t.Fatalf("tools = %v, want the union of the agent's and the wildcard profiles' grants", tools)
	}
	if cfg.AuditLogMetadata[AuditLogProfiles] != "vp1agent,vp1first,vp1second" {
		t.Fatalf("profiles = %q, want every profile that names the agent", cfg.AuditLogMetadata[AuditLogProfiles])
	}

	// An agent without its own profile is still refused on the same vMCP.
	if _, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, asAgent(6, 12)); !errors.As(err, new(*vmcpaccess.AgentNotReadyError)) {
		t.Fatalf("wildcard-only agent connect error = %v, want an AgentNotReadyError", err)
	}
}
