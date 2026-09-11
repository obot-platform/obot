package vmcp

import (
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureMCPServers owns the single set of servers used by a multi-user VMCP.
func EnsureMCPServers(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	var existing v1.MCPServerList
	if err := req.List(&existing, &kclient.ListOptions{Namespace: vmcp.Namespace, FieldSelector: fields.OneTermEqualSelector("spec.vmcpID", vmcp.Name)}); err != nil {
		return err
	}
	if !vmcpconfig.IsMultiUser(vmcp.Spec.Manifest) {
		for i := range existing.Items {
			if err := req.Client.Delete(req.Ctx, &existing.Items[i]); kclient.IgnoreNotFound(err) != nil {
				return err
			}
		}
		return nil
	}
	// Validate every component before creating any server.
	servers := make([]v1.MCPServer, 0, len(vmcp.Spec.Manifest.Components))
	for _, component := range vmcp.Spec.Manifest.Components {
		if component.ID == "" {
			return fmt.Errorf("component ID is required")
		}
		manifest, err := types.MapCatalogEntryToServer(component.CatalogEntry.Manifest, "", true)
		if err != nil {
			return err
		}

		servers = append(servers, v1.MCPServer{
			Name:        name.SafeConcatName(system.MCPServerPrefix+vmcp.Name, component.ID),
			Namespace:   vmcp.Namespace,
			Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: utils.Digest(component.CatalogEntry)},
			Spec: v1.MCPServerSpec{
				MCPCatalogID:              component.MCPCatalogID,
				MCPServerCatalogEntryName: component.MCPServerCatalogEntryID,
				Manifest:                  manifest,
				UnsupportedTools:          slices.Clone(component.CatalogEntry.UnsupportedTools),
				UserID:                    vmcp.Spec.CreatorUserID,
				VMCPID:                    vmcp.Name,
				VMCPComponentID:           component.ID,
			},
		})
	}
	desired := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		desired[server.Spec.VMCPComponentID] = struct{}{}
	}
	for i := range existing.Items {
		if _, ok := desired[existing.Items[i].Spec.VMCPComponentID]; ok {
			continue
		}
		if err := req.Client.Delete(req.Ctx, &existing.Items[i]); kclient.IgnoreNotFound(err) != nil {
			return err
		}
	}
	for i := range servers {
		server := &servers[i]
		var current v1.MCPServer
		if err := req.Get(&current, server.Namespace, server.Name); err == nil {
			if current.Spec.VMCPID != vmcp.Name || current.Spec.VMCPInstanceID != "" || current.Spec.VMCPComponentID != server.Spec.VMCPComponentID {
				return fmt.Errorf("MCPServer %q already exists with different VMCP ownership", server.Name)
			}
			if current.Annotations[v1.VMCPSnapshotDigestAnnotation] != server.Annotations[v1.VMCPSnapshotDigestAnnotation] ||
				current.Spec.UserID != server.Spec.UserID ||
				current.Spec.MCPCatalogID != server.Spec.MCPCatalogID ||
				current.Spec.MCPServerCatalogEntryName != server.Spec.MCPServerCatalogEntryName {
				current.Spec.Manifest = server.Spec.Manifest
				current.Spec.UnsupportedTools = server.Spec.UnsupportedTools
				current.Spec.UserID = server.Spec.UserID
				current.Spec.MCPCatalogID = server.Spec.MCPCatalogID
				current.Spec.MCPServerCatalogEntryName = server.Spec.MCPServerCatalogEntryName
				if current.Annotations == nil {
					current.Annotations = make(map[string]string, 1)
				}
				current.Annotations[v1.VMCPSnapshotDigestAnnotation] = server.Annotations[v1.VMCPSnapshotDigestAnnotation]
				if err := req.Client.Update(req.Ctx, &current); err != nil {
					return err
				}
			}
			continue
		} else if !apierrors.IsNotFound(err) {
			return err
		}
		if err := req.Client.Create(req.Ctx, server); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}
