package authz

import (
	"context"
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkCatalogEntry(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.MCPServerCatalogEntryID == "" {
		return true, nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPServerCatalogEntryID), &entry); err != nil {
		return false, err
	}

	return UserCanReadCatalogEntry(req.Context(), u, &entry, a.acrHelper)
}

// UserCanReadCatalogEntry checks access using the entry's stored scope.
func UserCanReadCatalogEntry(ctx context.Context, u kuser.Info, entry *v1.MCPServerCatalogEntry, helper *accesscontrolrule.Helper) (bool, error) {
	if entry.Spec.MCPCatalogName != "" {
		return helper.UserHasAccessToMCPServerCatalogEntryInCatalog(u, entry.Name, entry.Spec.MCPCatalogName)
	} else if entry.Spec.PowerUserWorkspaceID != "" {
		return helper.UserHasAccessToMCPServerCatalogEntryInWorkspace(ctx, u, entry.Name, entry.Spec.PowerUserWorkspaceID)
	}
	return false, nil
}
