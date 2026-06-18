package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (a *Authorizer) checkCatalogEntry(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.MCPServerCatalogEntryID == "" {
		return true, nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPServerCatalogEntryID), &entry); err != nil {
		return false, err
	}

	var (
		hasAccess bool
		err       error
	)

	if entry.Spec.MCPCatalogName != "" {
		hasAccess, err = a.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(u, entry.Name, entry.Spec.MCPCatalogName)
	} else if entry.Spec.PowerUserWorkspaceID != "" {
		hasAccess, err = a.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), u, entry.Name, entry.Spec.PowerUserWorkspaceID)
	}

	return hasAccess, err
}
