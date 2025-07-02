package mcpserverinstance

import (
	"github.com/obot-platform/nah/pkg/router"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gatewayClient *gateway.Client
}

func New(gatewayClient *gateway.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

// Migrate makes sure that all spec fields are set properly.
func (h *Handler) Migrate(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	// Check to see if we need to update.
	// Pre-migration, if there is a catalog name, it points to a shared server, and we don't need to add any new information.
	if instance.Spec.MCPCatalogName != "" {
		return nil
	}

	var server v1.MCPServer
	if err := req.Client.Get(req.Ctx, client.ObjectKey{
		Namespace: instance.Namespace,
		Name:      instance.Spec.MCPServerName,
	}, &server); err != nil {
		return err
	}

	if server.Spec.MCPServerCatalogEntryName == "" {
		instance.Spec.MCPServerCatalogEntryName = server.Spec.MCPServerCatalogEntryName

		var entry v1.MCPServerCatalogEntry
		if err := req.Client.Get(req.Ctx, client.ObjectKey{
			Namespace: instance.Namespace,
			Name:      instance.Spec.MCPServerCatalogEntryName,
		}, &entry); err != nil {
			return err
		}

		instance.Spec.MCPCatalogName = entry.Spec.MCPCatalogName

		return req.Client.Update(req.Ctx, instance)
	}

	return nil
}

func (h *Handler) RemoveOAuthToken(req router.Request, _ router.Response) error {
	return h.gatewayClient.DeleteMCPOAuthToken(req.Ctx, req.Object.GetName())
}
