package mcpcatalog

import (
	"context"
	"fmt"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// HandleUserGroupChange is triggered when a user loses one or more group memberships.
// It checks all of the user's MCPServers and MCPServerInstances to ensure they are still authorized.
// Any servers or instances that are no longer authorized (because the user lost group access) are deleted.
func (h *Handler) HandleUserGroupChange(req router.Request, _ router.Response) error {
	userGroupChange := req.Object.(*v1.UserGroupChange)
	userIDStr := strconv.Itoa(int(userGroupChange.Spec.UserID))

	// Get the user and their current groups
	user, err := h.getUserInfoForAccessControl(req.Ctx, userIDStr)
	if err != nil {
		// If user doesn't exist anymore, just delete the event
		log.Infof("User %s not found, deleting UserGroupChange event", userIDStr)
		return req.Delete(userGroupChange)
	}

	// Check and delete unauthorized servers in catalogs
	if err := h.deleteUnauthorizedServersForUser(req.Ctx, req.Client, req.Namespace, userIDStr, user); err != nil {
		return fmt.Errorf("failed to delete unauthorized servers for user %s: %w", userIDStr, err)
	}

	// Check and delete unauthorized instances in catalogs
	if err := h.deleteUnauthorizedInstancesForUser(req.Ctx, req.Client, req.Namespace, userIDStr, user); err != nil {
		return fmt.Errorf("failed to delete unauthorized instances for user %s: %w", userIDStr, err)
	}

	// Delete the UserGroupChange event after processing
	return req.Delete(userGroupChange)
}

// deleteUnauthorizedServersForUser checks all MCPServers owned by the user and deletes any that are no longer authorized.
func (h *Handler) deleteUnauthorizedServersForUser(ctx context.Context, client kclient.Client, namespace, userID string, user *userInfo) error {
	// Get all MCPServers owned by this user
	var mcpServers v1.MCPServerList
	if err := client.List(ctx, &mcpServers, &kclient.ListOptions{
		Namespace:     namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.userID", userID),
	}); err != nil {
		return fmt.Errorf("failed to list MCP servers for user %s: %w", userID, err)
	}

	for _, server := range mcpServers.Items {
		if !server.DeletionTimestamp.IsZero() {
			continue
		}

		// Skip special server types that don't need access checks
		if server.Spec.ThreadName != "" || server.Spec.UserID == "anonymous" || server.Spec.CompositeName != "" {
			// Legacy project-scoped servers, anonymous servers, and composite servers
			continue
		}

		// Skip multi-user servers - we only care about a user's own single-user servers
		// Multi-user servers have MCPCatalogID or PowerUserWorkspaceID set
		if server.Spec.MCPCatalogID != "" || server.Spec.PowerUserWorkspaceID != "" {
			continue
		}

		// At this point, we only have single-user servers created from catalog entries
		// For every server, fetch its catalog entry and check that the user has an ACR
		// that gives them access to that catalog entry
		var (
			hasAccess bool
			err       error
		)

		if server.Spec.MCPServerCatalogEntryName != "" {
			// Get the catalog entry to determine which catalog/workspace it belongs to
			var entry v1.MCPServerCatalogEntry
			if err := client.Get(ctx, kclient.ObjectKey{
				Namespace: namespace,
				Name:      server.Spec.MCPServerCatalogEntryName,
			}, &entry); err != nil {
				log.Warnf("Failed to get catalog entry %s: %v", server.Spec.MCPServerCatalogEntryName, err)
				continue
			}

			// Check access based on whether the catalog entry is in a workspace or regular catalog
			if entry.Spec.PowerUserWorkspaceID != "" {
				// Catalog entry is in a PowerUserWorkspace
				hasAccess, err = h.accessControlRuleHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(
					ctx, user, server.Spec.MCPServerCatalogEntryName, entry.Spec.PowerUserWorkspaceID)
			} else {
				// Catalog entry is in a regular catalog (e.g., default)
				hasAccess, err = h.accessControlRuleHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(
					user, server.Spec.MCPServerCatalogEntryName, entry.Spec.MCPCatalogName)
			}
		}

		if err != nil {
			return fmt.Errorf("failed to check access for server %s: %w", server.Name, err)
		}

		if !hasAccess {
			log.Infof("Deleting MCP server %q because user %s lost group access", server.Name, userID)
			if err := client.Delete(ctx, &server); err != nil {
				return fmt.Errorf("failed to delete MCP server %s: %w", server.Name, err)
			}
		}
	}

	return nil
}

// deleteUnauthorizedInstancesForUser checks all MCPServerInstances owned by the user and deletes any that are no longer authorized.
func (h *Handler) deleteUnauthorizedInstancesForUser(ctx context.Context, client kclient.Client, namespace, userID string, user *userInfo) error {
	// Get all MCPServerInstances owned by this user
	var mcpInstances v1.MCPServerInstanceList
	if err := client.List(ctx, &mcpInstances, &kclient.ListOptions{
		Namespace:     namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.userID", userID),
	}); err != nil {
		return fmt.Errorf("failed to list MCP server instances for user %s: %w", userID, err)
	}

	for _, instance := range mcpInstances.Items {
		if !instance.DeletionTimestamp.IsZero() || instance.Spec.CompositeName != "" {
			// Skip instances being deleted or composite instances
			continue
		}

		// Get the MCPServer this instance points to
		var server v1.MCPServer
		if err := client.Get(ctx, kclient.ObjectKey{
			Namespace: namespace,
			Name:      instance.Spec.MCPServerName,
		}, &server); err != nil {
			log.Warnf("Failed to get MCP server %s for instance %s: %v", instance.Spec.MCPServerName, instance.Name, err)
			continue
		}

		// Check authorization based on server location
		var (
			hasAccess bool
			err       error
		)

		if server.Spec.PowerUserWorkspaceID != "" {
			// Workspace-scoped multi-user server
			hasAccess, err = h.accessControlRuleHelper.UserHasAccessToMCPServerInWorkspace(
				user, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
		} else if server.Spec.MCPCatalogID != "" {
			// Catalog-scoped multi-user server
			hasAccess, err = h.accessControlRuleHelper.UserHasAccessToMCPServerInCatalog(
				user, server.Name, server.Spec.MCPCatalogID)
		}

		if err != nil {
			return fmt.Errorf("failed to check access for instance %s: %w", instance.Name, err)
		}

		if !hasAccess {
			log.Infof("Deleting MCPServerInstance %q because user %s lost group access", instance.Name, userID)
			if err := client.Delete(ctx, &instance); err != nil {
				return fmt.Errorf("failed to delete MCPServerInstance %s: %w", instance.Name, err)
			}
		}
	}

	return nil
}
