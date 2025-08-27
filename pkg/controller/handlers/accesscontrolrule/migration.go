package accesscontrolrule

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// MigrateToDefaultCatalog migrates existing AccessControlRules to the default catalog
func (h *Handler) MigrateToDefaultCatalog(req router.Request, _ router.Response) error {
	acr := req.Object.(*v1.AccessControlRule)

	if acr.Spec.MCPCatalogID == "" {
		acr.Spec.MCPCatalogID = system.DefaultCatalog
		return req.Client.Update(req.Ctx, acr)
	}

	return nil
}
