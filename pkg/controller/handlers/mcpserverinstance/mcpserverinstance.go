package mcpserverinstance

import (
	"github.com/obot-platform/nah/pkg/router"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	gatewayClient *gateway.Client
}

func New(gatewayClient *gateway.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) RemoveOAuthToken(req router.Request, _ router.Response) error {
	return h.gatewayClient.DeleteMCPOAuthToken(req.Ctx, req.Object.GetName())
}

func (h *Handler) MigrationDeleteSingleUserInstances(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	var server v1.MCPServer
	if err := req.Get(&server, req.Namespace, instance.Spec.MCPServerName); err != nil {
		return err
	}

	if server.Spec.SharedWithinMCPCatalogName == "" {
		// This server is unshared, so it should not have any server instances.
		// Delete this instance.
		return req.Delete(instance)
	}

	return nil
}
