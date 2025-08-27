package accesscontrolrule

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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
		catalogID             = acr.Spec.Manifest.MCPCatalogID
	)

	// Default to default catalog for legacy rules without catalog ID
	if catalogID == "" {
		catalogID = "default"
	}

	for _, resource := range acr.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeMCPServerCatalogEntry:
			if resource.ID == "*" {
				newResources = append(newResources, resource)
			} else if err := req.Get(&mcpservercatalogentry, req.Namespace, resource.ID); err == nil {
				// Check if entry belongs to the same catalog
				if mcpservercatalogentry.Spec.MCPCatalogName == catalogID {
					newResources = append(newResources, resource)
				}
				// If entry belongs to different catalog, remove it from the rule
			} else if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to get MCPServerCatalogEntry %s: %w", resource.ID, err)
			}
			// If entry not found, remove it from the rule
		case types.ResourceTypeMCPServer:
			if resource.ID == "*" {
				newResources = append(newResources, resource)
			} else if err := req.Get(&mcpserver, req.Namespace, resource.ID); err == nil {
				// Check if server belongs to the same catalog
				if h.serverBelongsToCatalog(&mcpserver, catalogID, req) {
					newResources = append(newResources, resource)
				}
				// If server belongs to different catalog, remove it from the rule
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

// serverBelongsToCatalog checks if an MCP server belongs to the specified catalog
func (h *Handler) serverBelongsToCatalog(server *v1.MCPServer, catalogID string, req router.Request) bool {
	// Check if server is shared within this catalog
	if server.Spec.SharedWithinMCPCatalogName != "" {
		return server.Spec.SharedWithinMCPCatalogName == catalogID
	}
	
	// Check if server came from a catalog entry
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, req.Namespace, server.Spec.MCPServerCatalogEntryName); err == nil {
			return entry.Spec.MCPCatalogName == catalogID
		}
	}
	
	// For servers without catalog association, they belong to default catalog only
	return catalogID == "default"
}
