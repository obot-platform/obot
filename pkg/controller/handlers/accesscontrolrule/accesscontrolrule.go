package accesscontrolrule

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/errors"
)

type Handler struct {
	acrHelper *accesscontrolrule.Helper
}

func New(acrHelper *accesscontrolrule.Helper) *Handler {
	return &Handler{
		acrHelper: acrHelper,
	}
}

func (h *Handler) PruneDeletedResources(req router.Request, _ router.Response) error {
	acr := req.Object.(*v1.AccessControlRule)

	// Make sure each resource still exists and belongs to the same catalog, remove it if not.
	var (
		mcpservercatalogentry v1.MCPServerCatalogEntry
		mcpserver             v1.MCPServer
		newResources          = make([]types.Resource, 0, len(acr.Spec.Manifest.Resources))
		catalogID             = acr.Spec.MCPCatalogID
	)

	// Default to default catalog for ACRs that have not yet been migrated
	if catalogID == "" && acr.Spec.PowerUserWorkspaceID == "" {
		catalogID = system.DefaultCatalog
	}

	// Loop through each resource and make sure that it exists in the catalog or workspace.
	// We shouldn't ever have a situation where the resource has somehow "moved" to a different catalog or workspace,
	// but we'll check anyway.
	for _, resource := range acr.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeMCPServerCatalogEntry:
			if err := req.Get(&mcpservercatalogentry, req.Namespace, resource.ID); err == nil {
				// Check if entry belongs to the same catalog or workspace
				var match bool
				if acr.Spec.PowerUserWorkspaceID != "" {
					match = mcpservercatalogentry.Spec.PowerUserWorkspaceID == acr.Spec.PowerUserWorkspaceID
				} else {
					match = mcpservercatalogentry.Spec.MCPCatalogName == catalogID
				}
				if match {
					newResources = append(newResources, resource)
				}
				// If entry belongs to different catalog or workspace, remove it from the rule
			} else if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to get MCPServerCatalogEntry %s: %w", resource.ID, err)
			}
			// If entry not found, remove it from the rule
		case types.ResourceTypeMCPServer:
			if err := req.Get(&mcpserver, req.Namespace, resource.ID); err == nil {
				// Check if server belongs to the same catalog and workspace (if workspace-scoped)
				var match bool
				if acr.Spec.PowerUserWorkspaceID != "" {
					match = mcpserver.Spec.PowerUserWorkspaceID == acr.Spec.PowerUserWorkspaceID
				} else {
					match = mcpserver.Spec.MCPCatalogID == catalogID
				}
				if match {
					newResources = append(newResources, resource)
				}
				// If server belongs to different catalog or workspace, remove it from the rule
			} else if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to get MCPServer %s: %w", resource.ID, err)
			}
			// If server not found, remove it from the rule
		case types.ResourceTypeSelector:
			newResources = append(newResources, resource)
		}
	}

	if len(newResources) != len(acr.Spec.Manifest.Resources) {
		acr.Spec.Manifest.Resources = newResources
		return req.Client.Update(req.Ctx, acr)
	}

	return nil
}
