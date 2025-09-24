package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkMCPID(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.MCPID == "" {
		return true, nil
	}

	switch {
	case system.IsMCPServerInstanceID(resources.MCPID):
		var mcpServerInstance v1.MCPServerInstance
		if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPID), &mcpServerInstance); err != nil {
			return false, err
		}

		return mcpServerInstance.Spec.UserID == user.GetUID(), nil

	case system.IsMCPServerID(resources.MCPID):
		var mcpServer v1.MCPServer
		if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPID), &mcpServer); err != nil {
			return false, err
		}

		if mcpServer.Spec.MCPCatalogID != "" {
			return a.acrHelper.UserHasAccessToMCPServerInCatalog(user, resources.MCPID, mcpServer.Spec.MCPCatalogID)
		} else if mcpServer.Spec.PowerUserWorkspaceID != "" {
			return a.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), user, resources.MCPID, mcpServer.Spec.PowerUserWorkspaceID)
		}

		// For single-user MCP servers, ensure the user owns the server.
		return mcpServer.Spec.UserID == user.GetUID(), nil
	default:
		var entry v1.MCPServerCatalogEntry
		if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPID), &entry); err != nil {
			return false, err
		}

		if entry.Spec.MCPCatalogName != "" {
			return a.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(user, resources.MCPID, entry.Spec.MCPCatalogName)
		} else if entry.Spec.PowerUserWorkspaceID != "" {
			return a.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), user, resources.MCPID, entry.Spec.PowerUserWorkspaceID)
		}

		return false, nil
	}
}
