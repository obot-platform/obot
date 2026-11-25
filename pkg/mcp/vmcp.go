package mcp

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/obot-platform/obot/pkg/wait"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// componentPlacement is the components a connection reaches through one instance: its own servers, and its connections
// to shared servers, by component ID.
type componentPlacement struct {
	instance    *v1.VMCPInstance
	servers     map[string]struct{}
	connections map[string]struct{}
}

// ServerConfigForVMCP resolves the component servers for a VMCP instance into
// the aggregate configuration consumed by the MCP gateway. VMCPs are backed by
// the MCPServers created by the VMCPInstance controller; the VMCP itself never
// gets launched as a runtime.
func (sm *SessionManager) ServerConfigForVMCP(ctx context.Context, vmcpID string, user kuser.Info) (ServerConfig, error) {
	userID, err := principal.CallerOwner(user)
	if err != nil {
		return ServerConfig{}, err
	}
	vmcp, resolvedInstance, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, vmcpID, userID)
	if err != nil {
		return ServerConfig{}, err
	}
	if vmcp == nil {
		return ServerConfig{}, fmt.Errorf("unknown VMCP %q", vmcpID)
	}

	return sm.serverConfigForVMCP(ctx, vmcp, resolvedInstance, user)
}

func (sm *SessionManager) serverConfigForVMCP(ctx context.Context, vmcp *v1.VMCP, instance *v1.VMCPInstance, user kuser.Info) (ServerConfig, error) {
	// The owner is the caller's encoded principal, so an agent ("agent:<id>") and the user with the same numeric ID
	// get separate instances, component servers, connections and credentials, and every key of one agent shares the
	// agent's single canonical (oldest) instance.
	userID, err := principal.CallerOwner(user)
	if err != nil {
		return ServerConfig{}, err
	}
	if len(vmcp.Spec.Manifest.Components) == 0 {
		return ServerConfig{}, types.NewErrBadRequest("cannot connect to a VMCP without components")
	}
	vmcpID := vmcp.Name
	if instance == nil {
		instance, err = vmcpaccess.FindInstance(ctx, sm.storageClient, vmcp.Namespace, vmcp.Name, userID)
		if err != nil {
			return ServerConfig{}, err
		}
	}
	var currentInstance v1.VMCPInstance
	if instance != nil {
		currentInstance = *instance
	}
	for _, component := range vmcpaccess.EnabledComponents(user, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, currentInstance)) {
		manifest := component.CatalogEntry.Manifest
		if manifest.RemoteConfig == nil || !manifest.RemoteConfig.StaticOAuthRequired {
			continue
		}
		for _, status := range vmcp.Status.Components {
			// An empty check hash means the controller has not checked the credential yet.
			if status.Name == component.Name && status.ConfigurationError == "" && status.OAuthCredentialCheckHash != "" && !status.OAuthCredentialConfigured {
				return ServerConfig{}, types.NewErrBadRequest("%s requires administrator static OAuth configuration", component.Name)
			}
		}
	}

	if instance == nil {
		if instance, err = sm.createVMCPInstance(ctx, vmcp, userID); err != nil {
			return ServerConfig{}, err
		}
	}

	if instance.Spec.Manifest.VMCPID != vmcpID || instance.Spec.UserID != userID {
		return ServerConfig{}, fmt.Errorf("VMCP instance %q does not belong to VMCP %q and user %q", instance.Name, vmcpID, userID)
	}

	instance, err = sm.waitForVMCPInstanceConfiguration(ctx, vmcp, instance, user)
	if err != nil {
		return ServerConfig{}, err
	}

	// Components disabled for the user are omitted entirely, so users never need
	// to configure or authenticate servers they cannot use.
	configuredComponents := vmcpaccess.EnabledComponents(user, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, *instance))

	// The vMCP's preconfigured OAuth for these components, in their order, which every caller's connections to them
	// use. An agent cannot complete setup interactively, so it is refused with the reasons it is not ready rather than
	// connected to a subset of its tools or challenged; its readiness selects the grants. For an agent acting on behalf
	// of a user, readiness is computed over the components of the vMCP profiles that name the agent, with what the
	// user set up through the agent, and what the user can set up is reported as needing their approval; a component
	// with preconfigured OAuth uses it, never the account the user connected through the agent. A user is never
	// refused for a grant: one that does not work is reported when the component is reached. Both are computed on
	// every connect and never cached.
	var (
		grants []vmcpaccess.ComponentOAuthGrant
		// pair is the instance of what the user set up through the agent, for an agent acting on behalf of a user, and
		// usesPair reports, for each enabled component, whether the agent reaches it through the pair's component
		// server or connection rather than its own (vmcpaccess.UsesPairSetup).
		pair     *v1.VMCPInstance
		usesPair []bool
	)
	if principal.IsAgent(user) {
		var agentReadiness vmcpaccess.AgentReadinessReport
		if delegated, ok := principal.DelegatedOwnerOf(user); ok {
			var selection vmcpaccess.DelegatedSelection
			if selection, pair, agentReadiness.Readiness, err = sm.delegatedSetup(ctx, vmcp, instance, user, delegated); err != nil {
				return ServerConfig{}, err
			}
			agentReadiness.Grants, usesPair = selection.Grants, selection.UsesPair
		} else if agentReadiness, err = sm.AgentReadiness(ctx, *vmcp, instance, user); err != nil {
			return ServerConfig{}, err
		}
		if !agentReadiness.Readiness.Ready() {
			notReady := &vmcpaccess.AgentNotReadyError{Readiness: agentReadiness.Readiness}
			if _, ok := principal.OnBehalfOf(user); ok {
				notReady.VMCPID, notReady.OnBehalfOfUser = vmcp.Name, true
			}
			return ServerConfig{}, notReady
		}
		grants = agentReadiness.Grants
	} else if vmcp.Spec.UserID == "" && sm.preconfiguredOAuth != nil {
		if grants, err = sm.preconfiguredOAuth.SelectGrants(ctx, *vmcp, configuredComponents); err != nil {
			return ServerConfig{}, err
		}
	}

	if usesPair != nil && len(usesPair) != len(configuredComponents) {
		// The selection was made from the same vMCP and instance, so this cannot happen.
		return ServerConfig{}, fmt.Errorf("the components of VMCP %q changed while connecting; retry", vmcp.Name)
	}
	// Each component is reached through the instance it is placed on: the caller's own, or for a component an agent
	// acting on behalf of a user needs the user for, the pair's.
	placements := []componentPlacement{newComponentPlacement(instance)}
	if pair != nil {
		placements = append(placements, newComponentPlacement(pair))
	}
	sharedComponents := make(map[string]struct{}, len(configuredComponents))
	for i, component := range configuredComponents {
		placement := placements[0]
		if usesPair != nil && usesPair[i] {
			if pair == nil {
				return ServerConfig{}, fmt.Errorf("component %q of VMCP %q needs the user's setup through the agent, which has no instance", component.ID, vmcp.Name)
			}
			placement = placements[1]
		}
		if vmcpaccess.IsMultiUser(component) {
			sharedComponents[component.ID] = struct{}{}
			placement.connections[component.ID] = struct{}{}
		} else {
			placement.servers[component.ID] = struct{}{}
		}
	}

	// Controllers copy configuration to the component servers and connections after
	// it changes, so each wait below also requires that copy, rather than letting
	// callers read the configuration from before it.
	componentsByID := make(map[string]types.VMCPComponent, len(configuredComponents))
	for _, component := range configuredComponents {
		componentsByID[component.ID] = component
	}

	// Collect the servers via a map here, but return them in the same order as the components in the manifest.
	serversByComponent := make(map[string]v1.MCPServer, len(configuredComponents))
	waitForServers := func(expected map[string]struct{}, selector kclient.MatchingFields, owner v1.VMCPInstance) error {
		if len(expected) == 0 {
			return nil
		}
		return wait.ForList(ctx, sm.storageClient, &v1.MCPServer{}, vmcp.Namespace, func(server *v1.MCPServer) (bool, error) {
			if _, ok := expected[server.Spec.VMCPComponentID]; !ok {
				return false, nil
			}
			if !vmcpaccess.ServerConfigurationSynced(*server, *vmcp, owner) {
				return false, nil
			}
			serversByComponent[server.Spec.VMCPComponentID] = *server
			delete(expected, server.Spec.VMCPComponentID)
			return len(expected) == 0, nil
		}, wait.ListOption{
			ListOptions: []kclient.ListOption{selector},
		})
	}
	// Shared servers hold only fixed configuration; user values travel on each connection.
	if err := waitForServers(sharedComponents, kclient.MatchingFields{"spec.vmcpID": vmcp.Name}, v1.VMCPInstance{}); err != nil {
		return ServerConfig{}, fmt.Errorf("wait for MCPServers for VMCP %q: %w", vmcpID, err)
	}
	connections := map[string]string{}
	for _, placement := range placements {
		owner := placement.instance
		if err := waitForServers(placement.servers, kclient.MatchingFields{"spec.vmcpInstanceID": owner.Name}, *owner); err != nil {
			return ServerConfig{}, fmt.Errorf("wait for MCPServers for VMCP %q: %w", vmcpID, err)
		}
		connectionsNeeded := placement.connections
		if len(connectionsNeeded) == 0 {
			continue
		}
		// A connection is the instance owner's: the caller's, or the pair's for an agent acting on behalf of a user.
		if err := wait.ForList(ctx, sm.storageClient, &v1.MCPServerInstance{}, vmcp.Namespace, func(connection *v1.MCPServerInstance) (bool, error) {
			if _, ok := connectionsNeeded[connection.Spec.VMCPComponentID]; !ok {
				return false, nil
			}
			if connection.Spec.UserID != owner.Spec.UserID || connection.Spec.MCPServerName != serversByComponent[connection.Spec.VMCPComponentID].Name || !connection.DeletionTimestamp.IsZero() {
				return false, nil
			}
			if !vmcpaccess.ConnectionConfigurationSynced(*connection, componentsByID[connection.Spec.VMCPComponentID], *owner) {
				return false, nil
			}
			connections[connection.Spec.VMCPComponentID] = connection.Name
			delete(connectionsNeeded, connection.Spec.VMCPComponentID)
			return len(connectionsNeeded) == 0, nil
		}, wait.ListOption{ListOptions: []kclient.ListOption{kclient.MatchingFields{"spec.vmcpInstanceID": owner.Name}}}); err != nil {
			return ServerConfig{}, fmt.Errorf("wait for component connections for VMCP instance %q: %w", owner.Name, err)
		}
	}

	components := make([]ComponentServer, 0, len(configuredComponents))
	injectObotHeaders := make(map[types.ObotHeader]struct{})
	for _, component := range configuredComponents {
		server := serversByComponent[component.ID]
		if types.SupportsObotHeaders(server.Spec.Manifest.Runtime) {
			for _, header := range server.Spec.Manifest.InjectObotHeaders {
				injectObotHeaders[header] = struct{}{}
			}
		}
		connectID := server.Name
		if connections[component.ID] != "" {
			connectID = connections[component.ID]
		}
		components = append(components, ComponentServer{
			ComponentID:         component.ID,
			Name:                server.Name,
			MCPServerInstanceID: connections[component.ID],
			DisplayName:         component.Name,
			URL:                 system.LocalMCPConnectURL(connectID, sm.httpListenPort),
			Tools:               slices.Clone(component.ToolOverrides),
			ToolPrefix:          component.ToolPrefix,
		})
	}

	// The grants were selected for these same components, in the same order. Each component's connection selects its
	// grant again when the composite reaches it (applyPreconfiguredOAuth); the grants selected here name the grant in
	// the composite's configuration, so that an authorization failure of the component is reported as the grant being
	// invalid: to an agent as its readiness, and to a user as a grant an administrator must authorize again, never as a
	// challenge to authorize the component themselves.
	for i, grant := range grants {
		if grant.Held() {
			components[i].OAuthGrant = newOAuthGrantRef(grant)
		}
	}

	allowedTools := vmcpaccess.InstanceGrant(user, *vmcp, *instance)
	for i := range components {
		if err := restrictComponentTools(&components[i], allowedTools, configuredComponents[i].ID); err != nil {
			return ServerConfig{}, err
		}
	}

	// Always retain the selected connection through the aggregate loopback.
	connectID := instance.Name
	// Match filters against the parent vMCP so every instance inherits them.
	webhooks, err := sm.webhooksForServerConfig(ServerConfig{
		MCPServerName:      vmcp.Name,
		MCPServerNamespace: vmcp.Namespace,
	})
	if err != nil {
		return ServerConfig{}, err
	}
	return ServerConfig{
		Runtime:              types.RuntimeVMCP,
		ConfigHash:           instance.Status.ConfigurationCheckHash,
		ToolSearch:           vmcp.Spec.Manifest.ToolSearch,
		MCPServerName:        connectID,
		MCPServerDisplayName: vmcp.Spec.Manifest.DisplayName,
		Audiences:            []string{system.MCPConnectURL(sm.baseURL, connectID), system.MCPConnectURL(sm.baseURL, vmcp.Name)},
		UserID:               userID,
		OwnerUserID:          vmcp.Spec.UserID,
		MCPServerNamespace:   vmcp.Namespace,
		Components:           components,
		InjectObotHeaders:    slices.Collect(maps.Keys(injectObotHeaders)),
		Webhooks:             webhooks,
		StartupTimeout:       types.DefaultStartupTimeoutSeconds * time.Second,
		AuditLogMetadata: map[string]string{
			"mcpID":                vmcp.Name,
			"mcpServerDisplayName": vmcp.Spec.Manifest.DisplayName,
			"userID":               userID,
			AuditLogVMCPID:         vmcp.Name,
			AuditLogProfiles:       strings.Join(contributingProfileIDs(user, *vmcp), ","),
		},
	}, nil
}

func newComponentPlacement(instance *v1.VMCPInstance) componentPlacement {
	return componentPlacement{instance: instance, servers: map[string]struct{}{}, connections: map[string]struct{}{}}
}

// createVMCPInstance creates the instance of vmcp that owner owns, and waits for the VMCPInstance controller to process
// it.
func (sm *SessionManager) createVMCPInstance(ctx context.Context, vmcp *v1.VMCP, owner string) (*v1.VMCPInstance, error) {
	instance, err := wait.For(ctx, sm.storageClient, &v1.VMCPInstance{
		Finalizers:   []string{v1.VMCPInstanceFinalizer},
		GenerateName: system.VMCPInstancePrefix,
		Namespace:    vmcp.Namespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
			UserID:   owner,
		},
	}, func(i *v1.VMCPInstance) (bool, error) {
		return i.Status.ConfigurationCheckHash != "", nil
	}, wait.Option{Timeout: 15 * time.Second, Create: true})
	if err != nil {
		return nil, fmt.Errorf("failed to create vMCP connection: %w", err)
	}
	return instance, nil
}

// delegatedSetup returns what agent, an agent acting on behalf of a user, is granted of vmcp through instance, the
// agent's own instance (vmcpaccess.SelectDelegated), the instance of what the user set up through the agent, owned by
// owner, and the agent's readiness on behalf of the user. The pair's instance is created when a selected component
// needs the user and the pair has none yet, so that its components are ready for the user's setup, and it is returned
// only when a selected component runs on it.
func (sm *SessionManager) delegatedSetup(ctx context.Context, vmcp *v1.VMCP, instance *v1.VMCPInstance, agent kuser.Info, owner principal.DelegatedOwner) (vmcpaccess.DelegatedSelection, *v1.VMCPInstance, vmcpaccess.Readiness, error) {
	selection, err := vmcpaccess.SelectDelegated(ctx, sm.preconfiguredOAuth, *vmcp, instance, vmcpaccess.CallerProfiles(agent, *vmcp))
	if err != nil {
		return vmcpaccess.DelegatedSelection{}, nil, vmcpaccess.Readiness{}, err
	}
	if !selection.NeedsPair() {
		return selection, nil, selection.Readiness(vmcpaccess.PairSetup{}), nil
	}
	pair, err := vmcpaccess.FindInstance(ctx, sm.storageClient, vmcp.Namespace, vmcp.Name, owner.String())
	if err != nil {
		return vmcpaccess.DelegatedSelection{}, nil, vmcpaccess.Readiness{}, err
	}
	if pair == nil {
		if pair, err = sm.createVMCPInstance(ctx, vmcp, owner.String()); err != nil {
			return vmcpaccess.DelegatedSelection{}, nil, vmcpaccess.Readiness{}, err
		}
	}
	if pair, err = sm.waitForVMCPInstanceConfiguration(ctx, vmcp, pair, agent); err != nil {
		return vmcpaccess.DelegatedSelection{}, nil, vmcpaccess.Readiness{}, err
	}
	connected, err := sm.preconfiguredOAuth.PairConnections(ctx, pair, selection.Components)
	if err != nil {
		return vmcpaccess.DelegatedSelection{}, nil, vmcpaccess.Readiness{}, err
	}
	return selection, pair, selection.Readiness(vmcpaccess.PairSetup{Instance: pair, Connected: connected}), nil
}

// AgentReadiness reports whether agent can use vmcp through instance, or nil when it has not connected, as a connect
// decides it (vmcpaccess.AgentReadiness, with the session manager's store of preconfigured OAuth grants). The readiness
// API uses it, so that it never reports an agent ready that a connect refuses.
func (sm *SessionManager) AgentReadiness(ctx context.Context, vmcp v1.VMCP, instance *v1.VMCPInstance, agent kuser.Info) (vmcpaccess.AgentReadinessReport, error) {
	return vmcpaccess.AgentReadiness(ctx, sm.preconfiguredOAuth, vmcp, instance, agent)
}

// newOAuthGrantRef names the grant a connection uses for a component, as SelectGrants selected it.
func newOAuthGrantRef(grant vmcpaccess.ComponentOAuthGrant) *OAuthGrantRef {
	return &OAuthGrantRef{ComponentID: grant.ComponentID, Owner: grant.Owner, MCPID: grant.MCPID}
}

// contributingProfileIDs returns the IDs of the profiles that grant the caller its permissions on a shared vMCP: every
// profile that names it, for an agent as for a user, and for an agent acting on behalf of a user, every profile that
// names the agent (vmcpaccess.CallerProfiles). A personal vMCP has no profiles.
func contributingProfileIDs(user kuser.Info, vmcp v1.VMCP) []string {
	if vmcp.Spec.UserID != "" {
		return nil
	}
	return vmcpaccess.ProfileIDs(vmcpaccess.CallerProfiles(user, vmcp))
}

// waitForVMCPInstanceConfiguration waits for the VMCPInstance controller to process
// the instance and the configuration the user last saved. Until it does, the
// instance's user configuration hash is stale (empty on a new instance), and the
// component waits would compare servers against a hash the controller has replaced.
func (sm *SessionManager) waitForVMCPInstanceConfiguration(ctx context.Context, vmcp *v1.VMCP, instance *v1.VMCPInstance, user kuser.Info) (*v1.VMCPInstance, error) {
	// An instance without saved configuration still needs the controller to record
	// its user configuration hash, so an empty sync hash is waited on like any other.
	syncHash := instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation]

	if _, ok := vmcpaccess.PairOwner(*instance); ok {
		// The instance of what a user set up through an agent carries the components that may need the user, whatever
		// the vMCP profiles grant, as the controller records it.
		want := vmcpaccess.ConfigurationCheckHash(vmcpaccess.PairInstanceComponents(vmcpaccess.ComponentsForInstance(*vmcp, *instance)), syncHash)
		if instance.Status.ConfigurationCheckHash == want {
			return instance, nil
		}
		instance, err := wait.For(ctx, sm.storageClient, instance, func(current *v1.VMCPInstance) (bool, error) {
			return current.Status.ConfigurationCheckHash == want, nil
		})
		if err != nil {
			return nil, fmt.Errorf("wait for vMCP setup through an agent to sync: %w", err)
		}
		return instance, nil
	}

	checkHash := func(u kuser.Info) string {
		return vmcpaccess.ConfigurationCheckHash(vmcpaccess.EnabledComponents(u, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, *instance)), syncHash)
	}
	want := checkHash(user)
	if delegated, ok := principal.DelegatedOwnerOf(user); ok {
		// An agent acting on behalf of a user connects through the agent's own instance, which the controller
		// configures for the agent itself, with every profile that names it. Matching an agent reads nothing but its
		// principal.
		want = checkHash(gateway.AgentUserInfo(&gatewaytypes.Agent{ID: delegated.AgentID}, nil, nil))
	}
	if instance.Status.ConfigurationCheckHash == want {
		return instance, nil
	}
	if vmcp.Spec.UserID == "" && sm.gatewayClient != nil {
		// The controller evaluates shared vMCP profiles with the stored user, whose
		// groups can differ from the requester's.
		owner, err := principal.Parse(instance.Spec.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid VMCP instance user ID: %w", err)
		}
		stored, err := sm.gatewayClient.IdentityInfo(ctx, owner)
		if err != nil {
			return nil, fmt.Errorf("resolve VMCP instance user: %w", err)
		}
		want = checkHash(stored)
	}

	instance, err := wait.For(ctx, sm.storageClient, instance, func(current *v1.VMCPInstance) (bool, error) {
		return current.Status.ConfigurationCheckHash == want, nil
	})
	if err != nil {
		return nil, fmt.Errorf("wait for vMCP connection configuration to sync: %w", err)
	}
	return instance, nil
}

// An empty intersection must disable tools explicitly: no overrides means unrestricted.
func restrictComponentTools(component *ComponentServer, allowed []types.VMCPToolReference, componentID string) error {
	if component.DisableTools {
		return nil
	}
	tools, restricted := vmcpaccess.GrantedToolOverrides(componentID, component.Tools, allowed)
	if !restricted {
		return nil
	}
	component.Tools = tools
	component.DisableTools = len(tools) == 0
	return nil
}

// ValidateSecretBindingsVMCP validates the Secrets selected by vMCP fixed
// configuration against the component runtimes they are applied to.
func ValidateSecretBindingsVMCP(manifest types.VMCPManifest, mcpBackend string) error {
	for _, component := range manifest.Components {
		if !slices.ContainsFunc(component.Configuration, func(policy types.VMCPConfigurationPolicy) bool {
			return policy.SecretBinding != nil
		}) {
			continue
		}
		entry := component.CatalogEntry.Manifest
		entry.Config = vmcpaccess.ComponentConfig(component)
		if err := ValidateSecretBindingsCatalogEntry(entry, true, false, mcpBackend); err != nil {
			return fmt.Errorf("component %q: %w", component.Name, err)
		}
	}
	return nil
}

// PairComponentServer returns the component server through which the pair that owns pair, the instance of what a user
// set up through an agent, reaches the component of vmcp with componentID, and the configuration of the pair's
// connection to it, once the controllers have created them and copied the pair's configuration to them, with the
// configuration the connection still lacks. The setup API authorizes the pair's upstream OAuth account for the
// component against it, as a user's own connection is authorized: the account is stored for the pair and the
// connection (PairConnectID), where an agent acting on behalf of the user finds it.
func (sm *SessionManager) PairComponentServer(ctx context.Context, vmcp *v1.VMCP, pair *v1.VMCPInstance, componentID string) (v1.MCPServer, ServerConfig, []string, error) {
	if _, ok := vmcpaccess.PairOwner(*pair); !ok || pair.Spec.Manifest.VMCPID != vmcp.Name {
		return v1.MCPServer{}, ServerConfig{}, nil, fmt.Errorf("VMCP instance %q is not a user's setup of VMCP %q through an agent", pair.Name, vmcp.Name)
	}
	components := vmcpaccess.PairInstanceComponents(vmcpaccess.ComponentsForInstance(*vmcp, *pair))
	index := slices.IndexFunc(components, func(component types.VMCPComponent) bool { return component.ID == componentID })
	if index < 0 {
		return v1.MCPServer{}, ServerConfig{}, nil, types.NewErrNotFound("component %q of vMCP %q needs nothing from users", componentID, vmcp.Name)
	}
	component := components[index]
	pair, err := sm.waitForVMCPInstanceConfiguration(ctx, vmcp, pair, nil)
	if err != nil {
		return v1.MCPServer{}, ServerConfig{}, nil, err
	}
	owner := pair.Spec.UserID
	options := wait.Option{Timeout: 15 * time.Second, WaitForExists: true}

	if vmcpaccess.IsMultiUser(component) {
		connection, err := wait.For(ctx, sm.storageClient, &v1.MCPServerInstance{
			Name:      vmcpaccess.ComponentConnectionName(pair.Name, component.ID),
			Namespace: pair.Namespace,
		}, func(connection *v1.MCPServerInstance) (bool, error) {
			return connection.DeletionTimestamp.IsZero() && vmcpaccess.ConnectionConfigurationSynced(*connection, component, *pair), nil
		}, options)
		if err != nil {
			return v1.MCPServer{}, ServerConfig{}, nil, fmt.Errorf("wait for the connection of VMCP instance %q to component %q: %w", pair.Name, component.ID, err)
		}
		if connection.Spec.UserID != owner || connection.Spec.VMCPInstanceID != pair.Name || connection.Spec.VMCPComponentID != component.ID {
			return v1.MCPServer{}, ServerConfig{}, nil, fmt.Errorf("MCPServerInstance %q has inconsistent vMCP ownership", connection.Name)
		}
		return sm.serverFromMCPServerInstance(ctx, *connection, owner, true)
	}

	server, err := wait.For(ctx, sm.storageClient, &v1.MCPServer{
		Name:      vmcpaccess.ComponentServerName(pair.Name, component.ID),
		Namespace: pair.Namespace,
	}, func(server *v1.MCPServer) (bool, error) {
		return server.DeletionTimestamp.IsZero() && vmcpaccess.ServerConfigurationSynced(*server, *vmcp, *pair), nil
	}, options)
	if err != nil {
		return v1.MCPServer{}, ServerConfig{}, nil, fmt.Errorf("wait for the server of VMCP instance %q for component %q: %w", pair.Name, component.ID, err)
	}
	if server.Spec.UserID != owner || server.Spec.VMCPInstanceID != pair.Name || server.Spec.VMCPComponentID != component.ID {
		return v1.MCPServer{}, ServerConfig{}, nil, fmt.Errorf("MCPServer %q has inconsistent vMCP ownership", server.Name)
	}
	config, missing, err := sm.serverConfigForAction(ctx, *server, owner, true)
	return *server, config, missing, err
}
