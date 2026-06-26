package authz

import (
	"context"
	"net/http"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkMCPID(req *http.Request, resources *Resources, user User) (bool, error) {
	if resources.MCPID == "" || user.GetName() == "anonymous" && strings.HasPrefix(req.URL.Path, "/mcp-connect/") {
		// If this is an MCP connect URL and the user is anonymous, then allow access.
		// The handler will catch this and support the WWW-Authenticate header to trigger the login flow.
		return true, nil
	}

	return CheckMCPIDAccess(req.Context(), a.uncached, a.acrHelper, user.Info, resources.MCPID)
}

func CheckMCPIDAccess(ctx context.Context, client kclient.Client, acrHelper *accesscontrolrule.Helper, user kuser.Info, mcpID string) (bool, error) {
	switch {
	case system.IsMCPServerInstanceID(mcpID):
		var mcpServerInstance v1.MCPServerInstance
		if err := client.Get(ctx, router.Key(system.DefaultNamespace, mcpID), &mcpServerInstance); err != nil {
			return false, err
		}

		return mcpServerInstance.Spec.UserID == user.GetUID(), nil

	case system.IsMCPServerID(mcpID):
		var mcpServer v1.MCPServer
		if err := client.Get(ctx, router.Key(system.DefaultNamespace, mcpID), &mcpServer); err != nil {
			return false, err
		}

		if mcpServer.Spec.IsCatalogServer() {
			return acrHelper.UserHasAccessToMCPServerInCatalog(user, mcpID, mcpServer.Spec.MCPCatalogID)
		} else if mcpServer.Spec.IsPowerUserWorkspaceServer() {
			return acrHelper.UserHasAccessToMCPServerInWorkspace(user, mcpID, mcpServer.Spec.PowerUserWorkspaceID, mcpServer.Spec.UserID)
		}

		return mcpServer.Spec.IsOwnedBy(user.GetUID()), nil

	case system.IsSystemMCPServerID(mcpID):
		var systemMCPServer v1.SystemMCPServer
		if err := client.Get(ctx, router.Key(system.DefaultNamespace, mcpID), &systemMCPServer); err != nil {
			return false, err
		}
		// If this is a system MCP server, then allow access. The system MCP server will enforce its own authorization.
		return systemMCPServer.Spec.Manifest.Enabled == nil || *systemMCPServer.Spec.Manifest.Enabled, nil
	default:
		var entry v1.MCPServerCatalogEntry
		if err := client.Get(ctx, router.Key(system.DefaultNamespace, mcpID), &entry); err != nil {
			return false, err
		}

		if entry.Spec.MCPCatalogName != "" {
			return acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(user, mcpID, entry.Spec.MCPCatalogName)
		} else if entry.Spec.PowerUserWorkspaceID != "" {
			return acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(ctx, user, mcpID, entry.Spec.PowerUserWorkspaceID)
		}

		return false, nil
	}
}
