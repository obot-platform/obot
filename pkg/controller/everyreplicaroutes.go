package controller

import (
	"github.com/obot-platform/obot/pkg/controller/handlers/providersync"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (c *Controller) setupEveryReplicaRoutes() {
	if c.services.EveryReplicaRouter == nil {
		return
	}

	providerSync := providersync.New(c.services.ProviderDispatcher, c.services.LicenseProvider)
	c.services.EveryReplicaRouter.Type(&v1.ProviderSync{}).
		Namespace(system.DefaultNamespace).
		Name(system.ProviderSyncName).
		HandlerFunc(providerSync.Reconcile)
}
