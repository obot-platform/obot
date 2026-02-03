package listing

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Lister provides methods for listing and filtering MCP servers with ACR checks.
type Lister struct {
	storageClient kclient.Client
	acrHelper     *accesscontrolrule.Helper
}

// NewLister creates a new Lister.
func NewLister(storageClient kclient.Client, acrHelper *accesscontrolrule.Helper) *Lister {
	return &Lister{
		storageClient: storageClient,
		acrHelper:     acrHelper,
	}
}

// ListCatalogEntries returns all catalog entries the user has access to.
// If limit > 0, only that many entries will be returned.
func (l *Lister) ListCatalogEntries(ctx context.Context, user kuser.Info, isAdmin bool, limit int) ([]v1.MCPServerCatalogEntry, error) {
	listOpts := []kclient.ListOption{kclient.InNamespace(system.DefaultNamespace)}

	// For admins, we can apply the limit directly to the query
	if isAdmin && limit > 0 {
		listOpts = append(listOpts, kclient.Limit(int64(limit)))
	}

	var list v1.MCPServerCatalogEntryList
	if err := l.storageClient.List(ctx, &list, listOpts...); err != nil {
		return nil, err
	}

	// Admin bypass
	if isAdmin {
		return list.Items, nil
	}

	// Apply ACR filtering
	var entries []v1.MCPServerCatalogEntry
	for _, entry := range list.Items {
		hasAccess, err := l.userHasAccessToCatalogEntry(ctx, user, entry)
		if err != nil {
			return nil, err
		}
		if hasAccess {
			entries = append(entries, entry)
			// Stop early if we've reached the limit
			if limit > 0 && len(entries) >= limit {
				break
			}
		}
	}

	return entries, nil
}

// GetCatalogEntry returns a single catalog entry if the user has access.
func (l *Lister) GetCatalogEntry(ctx context.Context, user kuser.Info, id string, isAdmin bool) (*v1.MCPServerCatalogEntry, error) {
	var entry v1.MCPServerCatalogEntry
	if err := l.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &entry); err != nil {
		return nil, err
	}

	if isAdmin {
		return &entry, nil
	}

	hasAccess, err := l.userHasAccessToCatalogEntry(ctx, user, entry)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, types.NewErrForbidden("user is not authorized to access this catalog entry")
	}

	return &entry, nil
}

// ListServers returns all multi-user MCP servers the user has access to.
// This returns servers that are scoped to catalogs or workspaces (multi-user servers).
// If limit > 0, only that many servers will be returned.
func (l *Lister) ListServers(ctx context.Context, user kuser.Info, isAdmin bool, limit int) ([]v1.MCPServer, error) {
	var list v1.MCPServerList
	if err := l.storageClient.List(ctx, &list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, err
	}

	// Admin bypass
	if isAdmin {
		var servers []v1.MCPServer
		for _, server := range list.Items {
			// Only include multi-user servers (those with catalog or workspace IDs)
			if server.Spec.MCPCatalogID != "" || server.Spec.PowerUserWorkspaceID != "" {
				if server.Spec.Template || server.Spec.CompositeName != "" {
					continue
				}
				servers = append(servers, server)
				// Stop early if we've reached the limit
				if limit > 0 && len(servers) >= limit {
					break
				}
			}
		}
		return servers, nil
	}

	// Apply ACR filtering
	var servers []v1.MCPServer
	for _, server := range list.Items {
		// Only include multi-user servers (those with catalog or workspace IDs)
		if server.Spec.MCPCatalogID == "" && server.Spec.PowerUserWorkspaceID == "" {
			continue
		}

		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}

		hasAccess, err := l.userHasAccessToServer(user, server)
		if err != nil {
			return nil, err
		}
		if hasAccess {
			servers = append(servers, server)
			// Stop early if we've reached the limit
			if limit > 0 && len(servers) >= limit {
				break
			}
		}
	}

	return servers, nil
}

// GetServer returns a single MCP server if the user has access.
func (l *Lister) GetServer(ctx context.Context, user kuser.Info, id string, isAdmin bool) (*v1.MCPServer, error) {
	var server v1.MCPServer
	if err := l.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &server); err != nil {
		return nil, err
	}

	// Check if server is from default catalog or workspace
	if server.Spec.MCPCatalogID != system.DefaultCatalog && server.Spec.PowerUserWorkspaceID == "" {
		return nil, types.NewErrNotFound("MCP server not found")
	}

	if isAdmin {
		return &server, nil
	}

	hasAccess, err := l.userHasAccessToServer(user, server)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, types.NewErrForbidden("user is not authorized to access this MCP server")
	}

	return &server, nil
}

// userHasAccessToCatalogEntry checks if a user has access to a catalog entry.
func (l *Lister) userHasAccessToCatalogEntry(ctx context.Context, user kuser.Info, entry v1.MCPServerCatalogEntry) (bool, error) {
	if entry.Spec.MCPCatalogName != "" {
		return l.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(user, entry.Name, entry.Spec.MCPCatalogName)
	} else if entry.Spec.PowerUserWorkspaceID != "" {
		return l.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(ctx, user, entry.Name, entry.Spec.PowerUserWorkspaceID)
	}
	return false, nil
}

// userHasAccessToServer checks if a user has access to an MCP server.
func (l *Lister) userHasAccessToServer(user kuser.Info, server v1.MCPServer) (bool, error) {
	if server.Spec.MCPCatalogID != "" {
		return l.acrHelper.UserHasAccessToMCPServerInCatalog(user, server.Name, server.Spec.MCPCatalogID)
	} else if server.Spec.PowerUserWorkspaceID != "" {
		return l.acrHelper.UserHasAccessToMCPServerInWorkspace(user, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
	}
	return false, nil
}
