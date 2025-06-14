package usercatalogauthorization

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Handler struct {
	gatewayClient *gclient.Client
}

func New(gatewayClient *gclient.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) DeleteServers(req router.Request, _ router.Response) error {
	usercatalogauthorization := req.Object.(*v1.UserCatalogAuthorization)

	if usercatalogauthorization.Spec.UserID == "*" {
		// All user access has been removed from this catalog.
		// Determine which users still have access.

		userAuthorizations, err := GetAuthorizationsForCatalog(req.Ctx, req.Client, usercatalogauthorization.Spec.MCPCatalogName)
		if err != nil {
			return fmt.Errorf("failed to get user authorizations: %w", err)
		}

		authorizedUsers := map[string]struct{}{}
		for _, auth := range userAuthorizations {
			authorizedUsers[auth.Spec.UserID] = struct{}{}
		}

		// List the entries for this catalog, and then list all the servers for those entries.
		var entries v1.MCPServerCatalogEntryList
		if err := req.Client.List(req.Ctx, &entries, client.InNamespace(system.DefaultNamespace), client.MatchingFields{
			"spec.mcpCatalogName": usercatalogauthorization.Spec.MCPCatalogName,
		}); err != nil {
			return fmt.Errorf("failed to list entries: %w", err)
		}

		for _, entry := range entries.Items {
			// List the servers for this entry.
			var servers v1.MCPServerList
			if err := req.Client.List(req.Ctx, &servers, client.InNamespace(system.DefaultNamespace), client.MatchingFields{
				"spec.mcpServerCatalogEntryName": entry.Name,
			}); err != nil {
				return fmt.Errorf("failed to list servers: %w", err)
			}

			for _, server := range servers.Items {
				if _, ok := authorizedUsers[server.Spec.UserID]; !ok {
					// Check if the user is an admin.
					// We ignore the error here in case the user somehow doesn't exist.
					// It's unlikely (and perhaps impossible) that we would ever be in this state though.
					if user, err := h.gatewayClient.UserByID(req.Ctx, server.Spec.UserID); err == nil && user.Role == types.RoleAdmin {
						// Don't delete servers for admins. They have access to all catalogs.
						continue
					}

					// This user is no longer authorized for this catalog, so delete it.
					log.Infof("Deleting server %s for user %s because they are no longer authorized for catalog %s", server.Name, server.Spec.UserID, usercatalogauthorization.Spec.MCPCatalogName)
					if err := req.Client.Delete(req.Ctx, &server); err != nil {
						return fmt.Errorf("failed to delete server: %w", err)
					}
				}
			}
		}
	} else {
		// Check to see if all users are authorized for this catalog.
		if allUsersAuthorized, err := AreAllUsersAuthorizedForCatalog(req.Ctx, req.Client, usercatalogauthorization.Spec.MCPCatalogName); err == nil && allUsersAuthorized {
			// Everyone is still authorized for this catalog, so no need to shut anything down.
			return nil
		}

		// We ignore the error here in case the user somehow doesn't exist.
		if user, err := h.gatewayClient.UserByID(req.Ctx, usercatalogauthorization.Spec.UserID); err == nil && user.Role == types.RoleAdmin {
			// Don't delete servers for admins. They have access to all catalogs.
			return nil
		}

		// List the users' servers and delete the ones that were from this catalog.
		var servers v1.MCPServerList
		if err := req.Client.List(req.Ctx, &servers, client.InNamespace(system.DefaultNamespace), client.MatchingFields{
			"spec.userID": usercatalogauthorization.Spec.UserID,
		}); err != nil {
			return fmt.Errorf("failed to list servers: %w", err)
		}

		for _, server := range servers.Items {
			// Check the catalog entry for this server, so that we can check which catalog it came from.
			var entry v1.MCPServerCatalogEntry
			if err := req.Client.Get(req.Ctx, client.ObjectKey{
				Namespace: system.DefaultNamespace,
				Name:      server.Spec.MCPServerCatalogEntryName,
			}, &entry); err != nil {
				return fmt.Errorf("failed to get catalog entry: %w", err)
			}

			if entry.Spec.MCPCatalogName == usercatalogauthorization.Spec.MCPCatalogName {
				// This server is from the catalog that the user is no longer authorized to use.
				// Delete it.
				log.Infof("Deleting server %s for user %s because they are no longer authorized for catalog %s", server.Name, server.Spec.UserID, usercatalogauthorization.Spec.MCPCatalogName)
				if err := req.Client.Delete(req.Ctx, &server); err != nil {
					return fmt.Errorf("failed to delete server: %w", err)
				}
			}
		}
	}

	return nil
}
