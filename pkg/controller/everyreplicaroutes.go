package controller

import (
	"github.com/obot-platform/obot/pkg/controller/handlers/providerdaemonsync"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (c *Controller) setupEveryReplicaRoutes() {
	if c.services.EveryReplicaRouter == nil {
		return
	}

	providerDaemonSync := providerdaemonsync.New(c.services.ProviderDispatcher)
	c.services.EveryReplicaRouter.Type(&v1.ProviderDaemonSync{}).
		Namespace(system.DefaultNamespace).
		Name(system.ProviderDaemonSyncName).
		HandlerFunc(providerDaemonSync.Reconcile)
}
