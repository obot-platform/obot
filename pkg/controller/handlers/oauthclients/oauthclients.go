package oauthclients

import (
	"github.com/obot-platform/nah/pkg/router"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	gatewayClient *gateway.Client
}

func NewHandler(gatewayClient *gateway.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) CleanupOAuthClientCred(req router.Request, _ router.Response) error {
	o := req.Object.(*v1.OAuthClient)

	if o.Spec.MCPServerName == "" {
		return nil
	}

	_, err := h.gatewayClient.DeleteCredential(req.Ctx, o.Spec.MCPServerName, o.Spec.MCPServerName)
	return err
}
