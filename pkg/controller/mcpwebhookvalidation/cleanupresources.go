package mcpwebhookvalidation

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) CleanupResources(req router.Request, _ router.Response) error {
	webhookValidation := req.Object.(*v1.MCPWebhookValidation)
	newResources := make([]types.Resource, 0, len(webhookValidation.Spec.Manifest.Resources))

	var (
		mcpServer v1.MCPServer
		catalog   v1.MCPCatalog
		err       error
	)
	for _, resource := range webhookValidation.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeSelector:
			newResources = append(newResources, resource)
		case types.ResourceTypeMCPServer:
			if err = req.Get(&mcpServer, req.Namespace, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get mcp server %s: %w", resource.ID, err)
			}
		case types.ResourceTypeMCPServerCatalogEntry:
			if err = req.Get(&catalog, req.Namespace, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get mcp server catalog entry %s: %w", resource.ID, err)
			}
		}
	}

	if len(newResources) != len(webhookValidation.Spec.Manifest.Resources) {
		webhookValidation.Spec.Manifest.Resources = newResources
		return req.Client.Update(req.Ctx, webhookValidation)
	}

	return nil
}
