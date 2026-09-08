package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
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
func (sm *SessionManager) ServerConfigForVMCP(ctx context.Context, vmcpID, userID string) (ServerConfig, error) {
	vmcp, resolvedInstance, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, vmcpID, userID)
	if err != nil {
		return ServerConfig{}, err
	}
	if vmcp == nil {
		return ServerConfig{}, fmt.Errorf("unknown VMCP %q", vmcpID)
	}
	if len(vmcp.Spec.Manifest.Components) == 0 {
		return ServerConfig{}, types.NewErrBadRequest("cannot connect to a VMCP without components")
	}
	vmcpID = vmcp.Name
	if resolvedInstance == nil {
		resolvedInstance, err = vmcpaccess.FindInstance(ctx, sm.storageClient, vmcp.Namespace, vmcp.Name, userID)
		if err != nil {
			return ServerConfig{}, err
		}
	}

	var instance v1.VMCPInstance
	if resolvedInstance != nil {
		instance = *resolvedInstance
	} else {
		instance = v1.VMCPInstance{
			Finalizers:   []string{v1.VMCPInstanceFinalizer},
			GenerateName: system.VMCPInstancePrefix,
			Namespace:    vmcp.Namespace,
			Spec: v1.VMCPInstanceSpec{
				Manifest: types.VMCPInstanceManifest{VMCPID: vmcpID},
				UserID:   userID,
			},
		}
		if err := sm.storageClient.Create(ctx, &instance); err != nil {
			return ServerConfig{}, fmt.Errorf("create VMCP instance for VMCP %q and user %q: %w", vmcpID, userID, err)
		}
	}

	if instance.Spec.Manifest.VMCPID != vmcpID || instance.Spec.UserID != userID {
		return ServerConfig{}, fmt.Errorf("VMCP instance %q does not belong to VMCP %q and user %q", instance.Name, vmcpID, userID)
	}

	configuredComponents := vmcpaccess.ComponentsForInstance(*vmcp, instance)
	expectedComponents := make(map[string]types.VMCPComponent, len(configuredComponents))
	for _, component := range configuredComponents {
		expectedComponents[component.ID] = component
	}

	// Collect the servers via a map here, but return them in the same order as the components in the manifest.
	serversByComponent := make(map[string]v1.MCPServer, len(expectedComponents))
	serverSelector := kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name}
	if vmcpaccess.IsMultiUser(vmcp.Spec.Manifest) {
		serverSelector = kclient.MatchingFields{"spec.vmcpID": vmcp.Name}
	}
	if len(expectedComponents) > 0 {
		if err := wait.ForList(ctx, sm.storageClient, &v1.MCPServer{}, vmcp.Namespace, func(server *v1.MCPServer) (bool, error) {
			if _, ok := expectedComponents[server.Spec.VMCPComponentID]; !ok {
				return false, nil
			}
			serversByComponent[server.Spec.VMCPComponentID] = *server
			delete(expectedComponents, server.Spec.VMCPComponentID)
			return len(expectedComponents) == 0, nil
		}, wait.ListOption{
			ListOptions: []kclient.ListOption{
				serverSelector,
			},
		}); err != nil {
			return ServerConfig{}, fmt.Errorf("wait for MCPServers for VMCP %q: %w", vmcpID, err)
		}
	}

	components := make([]ComponentServer, 0, len(configuredComponents))
	for _, component := range configuredComponents {
		server := serversByComponent[component.ID]
		components = append(components, ComponentServer{
			Name:        server.Name,
			DisplayName: component.Name,
			URL:         system.LocalMCPConnectURL(server.Name, sm.httpListenPort),
			Tools:       slices.Clone(component.ToolOverrides),
			ToolPrefix:  component.ToolPrefix,
		})
	}

	// Resolve current group membership for group profiles, including action paths
	// that have only the resource owner's ID rather than an authenticated request.
	var user kuser.Info = &kuser.DefaultInfo{UID: userID}
	needsGroups := false
	for _, profile := range vmcp.Spec.Manifest.Profiles {
		for _, subject := range profile.Subjects {
			needsGroups = needsGroups || subject.Type == types.SubjectTypeGroup
		}
	}
	if needsGroups {
		id, err := strconv.ParseUint(userID, 10, 64)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("invalid VMCP user ID: %w", err)
		}
		user, err = sm.gatewayClient.UserInfoByID(ctx, uint(id))
		if err != nil {
			return ServerConfig{}, fmt.Errorf("resolve VMCP user groups: %w", err)
		}
	}
	allowedTools := vmcpaccess.AllowedTools(user, vmcp.Spec.Manifest.Profiles, instance.Spec.Manifest.EnabledTools)
	if vmcp.Spec.UserID != "" && vmcp.Spec.UserID != userID {
		allowedTools = []types.VMCPToolReference{}
	}
	for i := range components {
		if err := restrictComponentTools(&components[i], allowedTools, configuredComponents[i].ID); err != nil {
			return ServerConfig{}, err
		}
	}
	// Keep a migrated connection's identity through the gateway loopback. Using
	// only the vMCP ID there would select the oldest connection a second time.
	connectID := vmcpID
	if instance.Spec.LegacySlug != "" {
		connectID = instance.Name
	}
	return ServerConfig{
		Runtime:              types.RuntimeVMCP,
		MCPServerName:        connectID,
		MCPServerDisplayName: vmcp.Spec.Manifest.DisplayName,
		UserID:               userID,
		OwnerUserID:          vmcp.Spec.UserID,
		MCPServerNamespace:   vmcp.Namespace,
		Components:           components,
		AuditLogMetadata: map[string]string{
			"mcpID":                connectID,
			"mcpServerDisplayName": vmcp.Spec.Manifest.DisplayName,
			"userID":               userID,
		},
	}, nil
}

// Match stable component identities and original tool names. An empty
// intersection must disable tools explicitly: no overrides means unrestricted.
func restrictComponentTools(component *ComponentServer, allowed []types.VMCPToolReference, componentID string) error {
	if allowed == nil || component.DisableTools || slices.Contains(allowed, types.VMCPToolReference{ComponentID: componentID, Name: "*"}) {
		return nil
	}
	var tools []types.ToolOverride
	if len(component.Tools) > 0 {
		for _, tool := range component.Tools {
			if tool.Enabled && slices.Contains(allowed, types.VMCPToolReference{ComponentID: componentID, Name: tool.Name}) {
				tools = append(tools, tool)
			}
		}
	} else {
		for _, ref := range allowed {
			if ref.ComponentID == componentID {
				tools = append(tools, types.ToolOverride{Name: ref.Name, Enabled: true})
			}
		}
	}
	component.Tools = tools
	component.DisableTools = len(tools) == 0
	return nil
}

// Shared servers persist fixed values only. User headers are resolved for this
// request, never copied into the shared server's credential.
func (sm *SessionManager) sharedVMCPConfiguration(ctx context.Context, server v1.MCPServer, userID string, fixed map[string]string) (map[string]string, error) {
	var vmcp v1.VMCP
	if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: server.Spec.VMCPID}, &vmcp); err != nil {
		return nil, err
	}
	if !vmcpaccess.IsMultiUser(vmcp.Spec.Manifest) {
		return nil, fmt.Errorf("VMCP %q is no longer multi-user", vmcp.Name)
	}
	var instances v1.VMCPInstanceList
	if err := sm.storageClient.List(ctx, &instances, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.userID":          userID,
		"spec.manifest.vmcpID": vmcp.Name,
	}); err != nil {
		return nil, err
	}
	if len(instances.Items) != 1 {
		return nil, fmt.Errorf("expected one VMCP instance for user %q", userID)
	}
	credential, err := sm.gatewayClient.RevealCredential(ctx, []string{vmcpaccess.InstanceConfigurationCredentialContext(instances.Items[0].Name)}, vmcpaccess.ConfigurationCredentialName())
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return nil, err
	}
	values := maps.Clone(fixed)
	if values == nil {
		values = map[string]string{}
	}
	for _, component := range vmcp.Spec.Manifest.Components {
		if component.ID != server.Spec.VMCPComponentID {
			continue
		}
		for _, policy := range component.Configuration {
			if policy.Policy != types.VMCPConfigurationPolicyUserAllowed {
				continue
			}
			// Ignore any stale shared value left over from a previous fixed policy.
			delete(values, policy.Key)
			if value, ok := credential.Secrets[vmcpaccess.ConfigurationKey(component.ID, policy.Key)]; ok {
				values[policy.Key] = value
			}
		}
	}
	return values, nil
}
