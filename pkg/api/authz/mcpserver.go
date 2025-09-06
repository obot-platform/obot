package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkMCPServer(req *http.Request, resources *Resources, u user.Info) (bool, error) {
	if resources.MCPServerID == "" {
		return true, nil
	}

	var mcpServer v1.MCPServer
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPServerID), &mcpServer); err != nil {
		return false, err
	}

	// If the user owns the MCP server, then authorization is granted.
	if mcpServer.Spec.UserID == u.GetUID() {
		resources.Authorizated.MCPServer = &mcpServer
		return true, nil
	}

	// If this MCP server is shared within the default catalog,
	// and an ACR allows the user to access it, then authorization is granted.
	if mcpServer.Spec.SharedWithinMCPCatalogName != "" {
		if mcpServer.Spec.SharedWithinMCPCatalogName == system.DefaultCatalog {
			// Check AccessControlRule authorization for this specific MCP server
			hasAccess, err := a.acrHelper.UserHasAccessToMCPServerInCatalog(u, mcpServer.Name, system.DefaultCatalog)
			if err != nil {
				return false, err
			}

			if hasAccess {
				resources.Authorizated.MCPServer = &mcpServer
				return true, nil
			}
		}
	}

	return false, nil
}
