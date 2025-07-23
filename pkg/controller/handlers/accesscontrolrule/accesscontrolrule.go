package accesscontrolrule

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

type Handler struct {
	acrHelper *Helper
}

func New(acrHelper *Helper) *Handler {
	return &Handler{
		acrHelper: acrHelper,
	}
}

func (h *Handler) PruneDeletedResources(req router.Request, _ router.Response) error {
	acr := req.Object.(*v1.AccessControlRule)

	// Make sure each resource still exists, and remove it if it is gone.
	var (
		mcpservercatalogentry v1.MCPServerCatalogEntry
		mcpserver             v1.MCPServer
		newResources          = make([]types.Resource, 0, len(acr.Spec.Manifest.Resources))
	)

	for _, resource := range acr.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeMCPServerCatalogEntry:
			if err := req.Get(&mcpservercatalogentry, req.Namespace, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to get MCPServerCatalogEntry %s: %w", resource.ID, err)
			}
		case types.ResourceTypeMCPServer:
			if err := req.Get(&mcpserver, req.Namespace, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to get MCPServer %s: %w", resource.ID, err)
			}
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
