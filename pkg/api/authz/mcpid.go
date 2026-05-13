package authz

import (
	"net/http"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkMCPID(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.MCPID == "" || user.GetName() == "anonymous" && strings.HasPrefix(req.URL.Path, "/mcp-connect/") {
		// If this is an MCP connect URL and the user is anonymous, then allow access.
		// The handler will catch this and support the WWW-Authenticate header to trigger the login flow.
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

		if mcpServer.Spec.IsCatalogServer() {
			return a.acrHelper.UserHasAccessToMCPServerInCatalog(user, resources.MCPID, mcpServer.Spec.MCPCatalogID)
		} else if mcpServer.Spec.IsPowerUserWorkspaceServer() {
			return a.acrHelper.UserHasAccessToMCPServerInWorkspace(user, resources.MCPID, mcpServer.Spec.PowerUserWorkspaceID, mcpServer.Spec.UserID)
		}

		// For single-user MCP servers, ensure the user owns the server.
		return mcpServer.Spec.UserID == user.GetUID(), nil

	case system.IsSystemMCPServerID(resources.MCPID):
		var systemMCPServer v1.SystemMCPServer
		if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPID), &systemMCPServer); err != nil {
			return false, err
		}
		// If this is a system MCP server, then allow access. The system MCP server will enforce its own authorization.
		return systemMCPServer.Spec.Manifest.Enabled == nil || *systemMCPServer.Spec.Manifest.Enabled, nil
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
