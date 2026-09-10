package vmcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ServerInstanceComponent validates the connection's ownership and current sharing mode.
func ServerInstanceComponent(ctx context.Context, client kclient.Client, connection v1.MCPServerInstance, server v1.MCPServer) (types.VMCPComponent, error) {
	var instance v1.VMCPInstance
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: connection.Namespace, Name: connection.Spec.VMCPInstanceID}, &instance); err != nil {
		return types.VMCPComponent{}, err
	}
	if instance.Spec.UserID != connection.Spec.UserID || instance.Spec.Manifest.VMCPID != server.Spec.VMCPID ||
		server.Spec.VMCPInstanceID != "" || connection.Spec.MCPServerName != server.Name || connection.Spec.VMCPComponentID != server.Spec.VMCPComponentID {
		return types.VMCPComponent{}, fmt.Errorf("MCPServerInstance %q has inconsistent vMCP ownership", connection.Name)
	}
	var vmcp v1.VMCP
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: instance.Namespace, Name: instance.Spec.Manifest.VMCPID}, &vmcp); err != nil {
		return types.VMCPComponent{}, err
	}
	for _, component := range ComponentsForInstance(vmcp, instance) {
		if component.ID == connection.Spec.VMCPComponentID && IsMultiUser(component) {
			return component, nil
		}
	}
	return types.VMCPComponent{}, fmt.Errorf("shared component %q no longer exists for vMCP instance %q", connection.Spec.VMCPComponentID, instance.Name)
}

// ComponentConfig applies vMCP policy rather than the source catalog's sharing flags.
func ComponentConfig(component types.VMCPComponent) []types.MCPConfig {
	config := component.CatalogEntry.Manifest.DeepCopy().Config
	for i := range config {
		config[i].UserAllowed = slices.ContainsFunc(component.Configuration, func(policy types.VMCPConfigurationPolicy) bool {
			return policy.Key == config[i].Key && policy.Policy == types.VMCPConfigurationPolicyUserAllowed
		})
	}
	return config
}
