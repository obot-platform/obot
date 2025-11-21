package projectmcpserver

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) EnsureMCPServerName(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.ProjectMCPServer)
	if server.Spec.MCPServerName != "" {
		return nil
	}

	if !system.IsMCPServerInstanceID(server.Spec.Manifest.MCPID) {
		server.Spec.MCPServerName = server.Spec.Manifest.MCPID
		return req.Client.Update(req.Ctx, server)
	}

	var mcpServerInstance v1.MCPServerInstance
	if err := req.Get(&mcpServerInstance, server.Namespace, server.Spec.Manifest.MCPID); err != nil {
		return err
	}

	server.Spec.MCPServerName = mcpServerInstance.Spec.MCPServerName
	return req.Client.Update(req.Ctx, server)
}
