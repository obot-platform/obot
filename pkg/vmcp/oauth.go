package vmcp

import (
	"context"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// StaticOAuthCredentialReference returns the catalog-owned credential context.
// The credential name within that context is always StaticOAuthCredentialName.
func StaticOAuthCredentialReference(manifest types.MCPServerCatalogEntryManifest, entryID string) string {
	if manifest.Runtime == types.RuntimeRemote && manifest.RemoteConfig != nil && manifest.RemoteConfig.StaticOAuthRequired {
		return system.MCPOAuthCredentialName(entryID)
	}
	return ""
}

func ComponentOAuthCredentialReference(component types.VMCPComponent) string {
	if component.OAuthCredentialID != "" {
		return component.OAuthCredentialID
	}
	// Existing snapshots predate the explicit reference field being populated.
	return StaticOAuthCredentialReference(component.CatalogEntry.Manifest, component.MCPServerCatalogEntryID)
}

// ServerOAuthCredentialReference resolves shared and dedicated component servers
// without depending on the continued existence of the source catalog entry.
func ServerOAuthCredentialReference(ctx context.Context, client kclient.Reader, server v1.MCPServer) (reference, sourceID string, err error) {
	vmcpID := server.Spec.VMCPID
	if server.Spec.VMCPInstanceID != "" {
		var instance v1.VMCPInstance
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: server.Spec.VMCPInstanceID}, &instance); err != nil {
			return "", "", err
		}
		vmcpID = instance.Spec.Manifest.VMCPID
	}
	if vmcpID == "" {
		if server.Spec.MCPServerCatalogEntryName != "" {
			return system.MCPOAuthCredentialName(server.Spec.MCPServerCatalogEntryName), server.Spec.MCPServerCatalogEntryName, nil
		}
		return "", "", nil
	}
	var vmcp v1.VMCP
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: vmcpID}, &vmcp); err != nil {
		return "", "", err
	}
	for _, component := range vmcp.Spec.Manifest.Components {
		if component.ID == server.Spec.VMCPComponentID {
			return ComponentOAuthCredentialReference(component), component.MCPServerCatalogEntryID, nil
		}
	}
	return "", "", fmt.Errorf("VMCP %q has no component %q", vmcpID, server.Spec.VMCPComponentID)
}
