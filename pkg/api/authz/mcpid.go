package authz

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkMCPID(req *http.Request, resources *Resources, user User) (bool, error) {
	if resources.MCPID == "" || user.GetName() == "anonymous" && strings.HasPrefix(req.URL.Path, "/mcp-connect/") {
		// If this is an MCP connect URL and the user is anonymous, then allow access.
		// The handler will catch this and support the WWW-Authenticate header to trigger the login flow.
		return true, nil
	}
	if authorized, err := CheckMCPIDAccess(req.Context(), a.uncached, a.acrHelper, user.Info, resources.MCPID); err != nil || !authorized {
		return false, err
	}

	if authorizedMCPIDs := user.GetExtra()["authorized_mcp_ids"]; len(authorizedMCPIDs) > 0 {
		return MCPIDIsAuthorized(req.Context(), a.uncached, authorizedMCPIDs, user.GetUID(), resources.MCPID)
	}

	return true, nil
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

func MCPIDIsAuthorized(ctx context.Context, client kclient.Client, authorizedMCPServers []string, userID, mcpID string) (bool, error) {
	// Check if this server is in the key's allowed list.
	// "*" is a special wildcard that grants access to all servers the user can access.
	if slices.Contains(authorizedMCPServers, "*") || slices.Contains(authorizedMCPServers, mcpID) {
		return true, nil
	}

	switch {
	case system.IsMCPServerInstanceID(mcpID):
		var mcpServerInstance v1.MCPServerInstance
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: mcpID}, &mcpServerInstance); err != nil {
			return false, err
		}
		if mcpServerInstance.Spec.CompositeName != "" {
			return slices.Contains(authorizedMCPServers, mcpServerInstance.Spec.CompositeName), nil
		}

		// Check the associated MCP server
		mcpID = mcpServerInstance.Spec.MCPServerName
		fallthrough
	case system.IsMCPServerID(mcpID):
		// Check if this is a component server - if so, check the composite server ID.
		var mcpServer v1.MCPServer
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: mcpID}, &mcpServer); err != nil {
			return false, err
		}

		return mcpServer.Spec.CompositeName != "" && slices.Contains(authorizedMCPServers, mcpServer.Spec.CompositeName), nil
	default:
		// Check for MCP servers associated with a catalog entry with this ID.
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: mcpID}, &v1.MCPServerCatalogEntry{}); apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}

		var mcpServers v1.MCPServerList
		if err := client.List(ctx, &mcpServers, kclient.MatchingFields{"spec.mcpServerCatalogEntryName": mcpID, "spec.userID": userID}); err != nil {
			return false, err
		}

		for _, mcpServer := range mcpServers.Items {
			if slices.Contains(authorizedMCPServers, mcpServer.Name) || mcpServer.Spec.CompositeName != "" && slices.Contains(authorizedMCPServers, mcpServer.Spec.CompositeName) {
				return true, nil
			}
		}

		return false, nil
	}
}
