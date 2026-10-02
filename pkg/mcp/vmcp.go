package mcp

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/obot-platform/obot/pkg/wait"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ServerConfigForVMCP resolves the component servers for a VMCP instance into
// the aggregate configuration consumed by the MCP gateway. VMCPs are backed by
// the MCPServers created by the VMCPInstance controller; the VMCP itself never
// gets launched as a runtime.
func (sm *SessionManager) ServerConfigForVMCP(ctx context.Context, vmcpID string, user kuser.Info) (ServerConfig, error) {
	userID := principal.ResourceOwnerID(user)
	vmcp, resolvedInstance, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, vmcpID, userID)
	if err != nil {
		return ServerConfig{}, err
	}
	if vmcp == nil {
		return ServerConfig{}, fmt.Errorf("unknown VMCP %q", vmcpID)
	}

	user, err = sm.vmcpResourceOwner(ctx, user)
	if err != nil {
		return ServerConfig{}, err
	}
	return sm.serverConfigForVMCP(ctx, vmcp, resolvedInstance, user)
}

// Hosted agents use their owner's connection and profile grants after the API
// authorizes the agent's own access. People already have their full identity.
func (sm *SessionManager) vmcpResourceOwner(ctx context.Context, user kuser.Info) (kuser.Info, error) {
	ownerID := principal.ResourceOwnerID(user)
	if ownerID == user.GetUID() {
		return user, nil
	}
	id, err := strconv.ParseUint(ownerID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid VMCP owner ID: %w", err)
	}
	owner, err := sm.gatewayClient.UserInfoByID(ctx, uint(id))
	if err != nil {
		return nil, fmt.Errorf("resolve VMCP owner: %w", err)
	}
	return owner, nil
}

func (sm *SessionManager) serverConfigForVMCP(ctx context.Context, vmcp *v1.VMCP, instance *v1.VMCPInstance, user kuser.Info) (ServerConfig, error) {
	userID := user.GetUID()
	if len(vmcp.Spec.Manifest.Components) == 0 {
		return ServerConfig{}, types.NewErrBadRequest("cannot connect to a VMCP without components")
	}
	vmcpID := vmcp.Name
	if instance == nil {
		var err error
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
		instance = &v1.VMCPInstance{
			Finalizers:   []string{v1.VMCPInstanceFinalizer},
			GenerateName: system.VMCPInstancePrefix,
			Namespace:    vmcp.Namespace,
			Spec: v1.VMCPInstanceSpec{
				Manifest: types.VMCPInstanceManifest{VMCPID: vmcpID},
				UserID:   userID,
			},
		}

		var err error
		instance, err = wait.For(ctx, sm.storageClient, instance, func(i *v1.VMCPInstance) (bool, error) {
			return i.Status.ConfigurationCheckHash != "", nil
		}, wait.Option{Timeout: 15 * time.Second, Create: true})
		if err != nil {
			return ServerConfig{}, fmt.Errorf("failed to create vMCP connection: %w", err)
		}
	}

	if instance.Spec.Manifest.VMCPID != vmcpID || instance.Spec.UserID != userID {
		return ServerConfig{}, fmt.Errorf("VMCP instance %q does not belong to VMCP %q and user %q", instance.Name, vmcpID, userID)
	}

	instance, err := sm.waitForVMCPInstanceConfiguration(ctx, vmcp, instance, user)
	if err != nil {
		return ServerConfig{}, err
	}

	// Components disabled for the user are omitted entirely, so users never need
	// to configure or authenticate servers they cannot use.
	configuredComponents := vmcpaccess.EnabledComponents(user, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, *instance))
	sharedComponents := make(map[string]struct{}, len(configuredComponents))
	connectionsNeeded := make(map[string]struct{}, len(configuredComponents))
	instanceComponents := make(map[string]struct{}, len(configuredComponents))
	for _, component := range configuredComponents {
		if vmcpaccess.IsMultiUser(component) {
			sharedComponents[component.ID] = struct{}{}
			connectionsNeeded[component.ID] = struct{}{}
		} else {
			instanceComponents[component.ID] = struct{}{}
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
	if err := waitForServers(instanceComponents, kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name}, *instance); err != nil {
		return ServerConfig{}, fmt.Errorf("wait for MCPServers for VMCP %q: %w", vmcpID, err)
	}
	connections := map[string]string{}
	if len(connectionsNeeded) > 0 {
		if err := wait.ForList(ctx, sm.storageClient, &v1.MCPServerInstance{}, vmcp.Namespace, func(connection *v1.MCPServerInstance) (bool, error) {
			if _, ok := connectionsNeeded[connection.Spec.VMCPComponentID]; !ok {
				return false, nil
			}
			if connection.Spec.UserID != userID || connection.Spec.MCPServerName != serversByComponent[connection.Spec.VMCPComponentID].Name || !connection.DeletionTimestamp.IsZero() {
				return false, nil
			}
			if !vmcpaccess.ConnectionConfigurationSynced(*connection, componentsByID[connection.Spec.VMCPComponentID], *instance) {
				return false, nil
			}
			connections[connection.Spec.VMCPComponentID] = connection.Name
			delete(connectionsNeeded, connection.Spec.VMCPComponentID)
			return len(connectionsNeeded) == 0, nil
		}, wait.ListOption{ListOptions: []kclient.ListOption{kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name}}}); err != nil {
			return ServerConfig{}, fmt.Errorf("wait for component connections for VMCP instance %q: %w", instance.Name, err)
		}
	}

	components := make([]ComponentServer, 0, len(configuredComponents))
	for _, component := range configuredComponents {
		server := serversByComponent[component.ID]
		connectID := server.Name
		if connections[component.ID] != "" {
			connectID = connections[component.ID]
		}
		components = append(components, ComponentServer{
			Name:                server.Name,
			MCPServerInstanceID: connections[component.ID],
			DisplayName:         component.Name,
			URL:                 system.LocalMCPConnectURL(connectID, sm.httpListenPort),
			Tools:               slices.Clone(component.ToolOverrides),
			ToolPrefix:          component.ToolPrefix,
		})
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
		MCPServerName:        connectID,
		MCPServerDisplayName: vmcp.Spec.Manifest.DisplayName,
		Audiences:            []string{system.MCPConnectURL(sm.baseURL, connectID), system.MCPConnectURL(sm.baseURL, vmcp.Name)},
		UserID:               userID,
		OwnerUserID:          vmcp.Spec.UserID,
		MCPServerNamespace:   vmcp.Namespace,
		Components:           components,
		Webhooks:             webhooks,
		StartupTimeout:       types.DefaultStartupTimeoutSeconds * time.Second,
		AuditLogMetadata: map[string]string{
			"mcpID":                vmcp.Name,
			"mcpServerDisplayName": vmcp.Spec.Manifest.DisplayName,
			"userID":               userID,
		},
	}, nil
}

// waitForVMCPInstanceConfiguration waits for the VMCPInstance controller to process
// the instance and the configuration the user last saved. Until it does, the
// instance's user configuration hash is stale (empty on a new instance), and the
// component waits would compare servers against a hash the controller has replaced.
func (sm *SessionManager) waitForVMCPInstanceConfiguration(ctx context.Context, vmcp *v1.VMCP, instance *v1.VMCPInstance, user kuser.Info) (*v1.VMCPInstance, error) {
	// An instance without saved configuration still needs the controller to record
	// its user configuration hash, so an empty sync hash is waited on like any other.
	syncHash := instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation]

	checkHash := func(u kuser.Info) string {
		return vmcpaccess.ConfigurationCheckHash(vmcpaccess.EnabledComponents(u, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, *instance)), syncHash)
	}
	want := checkHash(user)
	if instance.Status.ConfigurationCheckHash == want {
		return instance, nil
	}
	if vmcp.Spec.UserID == "" && sm.gatewayClient != nil {
		// The controller evaluates shared vMCP profiles with the stored user, whose
		// groups can differ from the requester's.
		id, err := strconv.ParseUint(instance.Spec.UserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid VMCP instance user ID: %w", err)
		}
		stored, err := sm.gatewayClient.UserInfoByID(ctx, uint(id))
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
