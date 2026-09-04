package mcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ServerConfigForVMCP resolves the component servers for a VMCP instance into
// the aggregate configuration consumed by the MCP gateway. VMCPs are backed by
// the MCPServers created by the VMCPInstance controller; the VMCP itself never
// gets launched as a runtime.
func (sm *SessionManager) ServerConfigForVMCP(ctx context.Context, vmcpID, userID string) (ServerConfig, error) {
	var vmcp v1.VMCP
	if err := sm.storageClient.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      vmcpID,
	}, &vmcp); err != nil {
		return ServerConfig{}, fmt.Errorf("get VMCP %q: %w", vmcpID, err)
	}

	var instances v1.VMCPInstanceList
	if err := sm.storageClient.List(ctx, &instances,
		kclient.InNamespace(vmcp.Namespace),
		kclient.MatchingFields{
			"spec.userID":          userID,
			"spec.manifest.vmcpID": vmcpID,
		},
	); err != nil {
		return ServerConfig{}, fmt.Errorf("list VMCP instances for VMCP %q and user %q: %w", vmcpID, userID, err)
	}
	if len(instances.Items) > 1 {
		return ServerConfig{}, fmt.Errorf("found multiple VMCP instances for VMCP %q and user %q", vmcpID, userID)
	}

	var instance v1.VMCPInstance
	if len(instances.Items) == 1 {
		instance = instances.Items[0]
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

	expectedComponents := make(map[string]types.VMCPComponent, len(vmcp.Spec.Manifest.Components))
	for _, component := range vmcp.Spec.Manifest.Components {
		expectedComponents[component.ID] = component
	}

	// Collect the servers via a map here, but return them in the same order as the components in the manifest.
	serversByComponent := make(map[string]v1.MCPServer, len(expectedComponents))
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
				kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name},
			},
		}); err != nil {
			return ServerConfig{}, fmt.Errorf("wait for MCPServers for VMCP %q: %w", vmcpID, err)
		}
	}

	components := make([]ComponentServer, 0, len(vmcp.Spec.Manifest.Components))
	for _, component := range vmcp.Spec.Manifest.Components {
		server := serversByComponent[component.ID]
		components = append(components, ComponentServer{
			Name:        server.Name,
			DisplayName: component.Name,
			URL:         system.LocalMCPConnectURL(server.Name, sm.httpListenPort),
			Tools:       slices.Clone(component.ToolOverrides),
			ToolPrefix:  component.ToolPrefix,
		})
	}

	return ServerConfig{
		Runtime:              types.RuntimeVMCP,
		MCPServerName:        vmcpID,
		MCPServerDisplayName: vmcp.Spec.Manifest.DisplayName,
		UserID:               userID,
		OwnerUserID:          vmcp.Spec.UserID,
		MCPServerNamespace:   vmcp.Namespace,
		Components:           components,
		AuditLogMetadata: map[string]string{
			"mcpID":                vmcpID,
			"mcpServerDisplayName": vmcp.Spec.Manifest.DisplayName,
			"userID":               userID,
		},
	}, nil
}
