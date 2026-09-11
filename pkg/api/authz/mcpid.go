package authz

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkMCPID(req *http.Request, resources *Resources, user User) (bool, error) {
	if resources.MCPID == "" || user.GetName() == "anonymous" && strings.HasPrefix(req.URL.Path, "/mcp-connect") {
		// If this is an MCP connect URL and the user is anonymous, then allow access.
		// The handler will catch this and support the WWW-Authenticate header to trigger the login flow.
		return true, nil
	}
	// A hosted agent is authorized by what it was granted, not by what its
	// owner can currently reach.
	//
	// The checks below ask whether the caller is a user with access to this
	// server -- through a catalog, a workspace, or ownership. An agent is none
	// of those: it is a principal of its own, so every one of them denies it and
	// an agent could never reach the MCP servers it was configured with.
	//
	// Its grant list is the authority instead. That list is not self-asserted:
	// servers on the template were granted by the administrator who published
	// it, and servers on the instance were checked against the owner when they
	// were attached.
	return userCanConnectToMCP(req.Context(), a.uncached, a.acrHelper, user.Info, resources.MCPID, resources)
}

// UserCanConnectToMCP applies the same current-user authorization used by the
// MCP gateway. API handlers that act on an MCP deployment without traversing
// /mcp-connect must call this rather than relying on management visibility.
func UserCanConnectToMCP(ctx context.Context, client kclient.Client, acrHelper *accesscontrolrule.Helper, user kuser.Info, mcpID string) (bool, error) {
	return userCanConnectToMCP(ctx, client, acrHelper, user, mcpID, nil)
}

func userCanConnectToMCP(ctx context.Context, client kclient.Client, acrHelper *accesscontrolrule.Helper, user kuser.Info, mcpID string, resources *Resources) (bool, error) {
	if principal.IsHostedAgent(user) {
		serverID := mcpID
		if system.IsMCPServerInstanceID(serverID) {
			var instance v1.MCPServerInstance
			if err := client.Get(ctx, router.Key(system.DefaultNamespace, serverID), &instance); err != nil {
				return false, err
			}
			serverID = instance.Spec.MCPServerName
		}
		if system.IsMCPServerID(serverID) {
			var server v1.MCPServer
			if err := client.Get(ctx, router.Key(system.DefaultNamespace, serverID), &server); err != nil {
				return false, err
			}
			if server.Spec.VMCPID != "" || server.Spec.VMCPInstanceID != "" {
				return false, nil
			}
		}
		return mcpIDIsAuthorized(ctx, client, user.GetExtra()["authorized_mcp_ids"], user.GetUID(), mcpID, resources)
	}

	authorized, err := checkMCPIDAccess(ctx, client, acrHelper, user, mcpID, resources)
	if err != nil || !authorized {
		return false, err
	}

	if authorizedMCPIDs := user.GetExtra()["authorized_mcp_ids"]; len(authorizedMCPIDs) > 0 {
		return mcpIDIsAuthorized(ctx, client, authorizedMCPIDs, user.GetUID(), mcpID, resources)
	}

	return true, nil
}

func CheckMCPIDAccess(ctx context.Context, client kclient.Client, acrHelper *accesscontrolrule.Helper, user kuser.Info, mcpID string) (bool, error) {
	return checkMCPIDAccess(ctx, client, acrHelper, user, mcpID, nil)
}

func checkMCPIDAccess(ctx context.Context, client kclient.Client, acrHelper *accesscontrolrule.Helper, user kuser.Info, mcpID string, resources *Resources) (bool, error) {
	if vmcp, instance, err := vmcpaccess.ResolveConnectID(ctx, client, mcpID, user.GetUID()); err != nil {
		return false, err
	} else if vmcp != nil {
		if !UserCanConnectVMCP(user, vmcp) {
			return false, nil
		}
		if resources != nil {
			if instance == nil && resources.VMCPComponentMCPID != "" {
				instance, err = vmcpaccess.FindInstance(ctx, client, vmcp.Namespace, vmcp.Name, user.GetUID())
				if err != nil || instance == nil {
					return false, err
				}
			}
			resources.Authorizated.VMCP = vmcp
			resources.Authorizated.VMCPInstance = instance
		}
		return true, nil
	}
	switch {
	case system.IsMCPServerInstanceID(mcpID):
		var mcpServerInstance v1.MCPServerInstance
		if err := client.Get(ctx, router.Key(system.DefaultNamespace, mcpID), &mcpServerInstance); err != nil {
			return false, err
		}

		if mcpServerInstance.Spec.UserID != user.GetUID() {
			return false, nil
		}
		var server v1.MCPServer
		if err := client.Get(ctx, router.Key(mcpServerInstance.Namespace, mcpServerInstance.Spec.MCPServerName), &server); err != nil {
			return false, err
		}
		return server.Spec.VMCPID == "" && server.Spec.VMCPInstanceID == "", nil

	case system.IsMCPServerID(mcpID):
		var mcpServer v1.MCPServer
		if err := client.Get(ctx, router.Key(system.DefaultNamespace, mcpID), &mcpServer); err != nil {
			return false, err
		}

		vmcpID := mcpServer.Spec.VMCPID
		if vmcpID != "" || mcpServer.Spec.VMCPInstanceID != "" {
			// Only the signed aggregate loopback token may reach component servers.
			// External clients must pass through the aggregate's tool filtering.
			if !slices.Contains(user.GetGroups(), types.GroupCompositeMCP) ||
				!slices.Contains(user.GetExtra()["authorized_mcp_ids"], mcpID) {
				return false, nil
			}
		}
		if mcpServer.Spec.VMCPInstanceID != "" {
			var instance v1.VMCPInstance
			if err := client.Get(ctx, router.Key(mcpServer.Namespace, mcpServer.Spec.VMCPInstanceID), &instance); err != nil {
				return false, err
			}
			if instance.Spec.UserID != user.GetUID() || mcpServer.Spec.UserID != user.GetUID() {
				return false, nil
			}
			vmcpID = instance.Spec.Manifest.VMCPID
		}
		if vmcpID != "" {
			var vmcp v1.VMCP
			if err := client.Get(ctx, router.Key(mcpServer.Namespace, vmcpID), &vmcp); err != nil {
				return false, err
			}
			return UserCanConnectVMCP(user, &vmcp) && slices.ContainsFunc(vmcp.Spec.Manifest.Components, func(component types.VMCPComponent) bool {
				return component.ID == mcpServer.Spec.VMCPComponentID
			}), nil
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

	case system.IsVMCPID(mcpID):
		var vmcp v1.VMCP
		if err := client.Get(ctx, router.Key(system.DefaultNamespace, mcpID), &vmcp); err != nil {
			return false, err
		}

		return UserCanConnectVMCP(user, &vmcp), nil
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
	return mcpIDIsAuthorized(ctx, client, authorizedMCPServers, userID, mcpID, nil)
}

func mcpIDIsAuthorized(ctx context.Context, client kclient.Client, authorizedMCPServers []string, userID, mcpID string, resources *Resources) (bool, error) {
	// Check if this server is in the key's allowed list.
	// "*" is a special wildcard that grants access to all servers the user can access.
	if slices.Contains(authorizedMCPServers, "*") || slices.Contains(authorizedMCPServers, mcpID) {
		return true, nil
	}
	var (
		vmcp     *v1.VMCP
		instance *v1.VMCPInstance
		err      error
	)
	if resources != nil && resources.Authorizated.VMCP != nil {
		vmcp, instance = resources.Authorizated.VMCP, resources.Authorizated.VMCPInstance
	} else {
		vmcp, instance, err = vmcpaccess.ResolveConnectID(ctx, client, mcpID, userID)
		if err != nil {
			return false, err
		}
	}
	if vmcp != nil {
		if vmcpScopeMatches(authorizedMCPServers, vmcp, instance) {
			if resources != nil {
				resources.Authorizated.VMCP = vmcp
				resources.Authorizated.VMCPInstance = instance
			}
			return true, nil
		}
		if instance == nil {
			instance, err = vmcpaccess.FindInstance(ctx, client, vmcp.Namespace, vmcp.Name, userID)
			if err != nil {
				return false, err
			}
		}
		authorized := vmcpScopeMatches(authorizedMCPServers, vmcp, instance)
		if authorized && resources != nil {
			resources.Authorizated.VMCP = vmcp
			resources.Authorizated.VMCPInstance = instance
		}
		return authorized, nil
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

		if mcpServer.Spec.VMCPInstanceID != "" {
			var instance v1.VMCPInstance
			if err := client.Get(ctx, kclient.ObjectKey{Namespace: mcpServer.Namespace, Name: mcpServer.Spec.VMCPInstanceID}, &instance); err != nil {
				return false, err
			}
			if instance.Spec.UserID != userID || mcpServer.Spec.UserID != userID {
				return false, nil
			}
			var vmcp v1.VMCP
			if err := client.Get(ctx, kclient.ObjectKey{Namespace: instance.Namespace, Name: instance.Spec.Manifest.VMCPID}, &vmcp); err != nil {
				return false, err
			}
			return vmcpScopeMatches(authorizedMCPServers, &vmcp, &instance), nil
		}
		return slices.Contains(authorizedMCPServers, mcpServer.Name) ||
			mcpServer.Spec.VMCPID != "" && slices.Contains(authorizedMCPServers, mcpServer.Spec.VMCPID) ||
			mcpServer.Spec.CompositeName != "" && slices.Contains(authorizedMCPServers, mcpServer.Spec.CompositeName) ||
			mcpServer.Spec.VMCPID == "" && mcpServer.Spec.MCPServerCatalogEntryName != "" && userID == mcpServer.Spec.UserID && slices.Contains(authorizedMCPServers, mcpServer.Spec.MCPServerCatalogEntryName), nil
	case system.IsVMCPID(mcpID):
		// Only an explicit vMCP scope (or wildcard above) grants its endpoint.
		return false, nil
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
			if mcpServer.Spec.VMCPID != "" || mcpServer.Spec.VMCPInstanceID != "" {
				continue
			}
			if slices.Contains(authorizedMCPServers, mcpServer.Name) || mcpServer.Spec.CompositeName != "" && slices.Contains(authorizedMCPServers, mcpServer.Spec.CompositeName) {
				return true, nil
			}
		}

		return false, nil
	}
}

func vmcpScopeMatches(scopes []string, vmcp *v1.VMCP, instance *v1.VMCPInstance) bool {
	if slices.Contains(scopes, vmcp.Name) || vmcp.Spec.LegacySlug != "" && slices.Contains(scopes, vmcp.Spec.LegacySlug) {
		return true
	}
	return instance != nil && (slices.Contains(scopes, instance.Name) || instance.Spec.LegacySlug != "" && slices.Contains(scopes, instance.Spec.LegacySlug))
}
