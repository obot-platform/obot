package controller

import (
	"context"
	"log/slog"

	"github.com/obot-platform/obot/pkg/controller/handlers/providersync"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (c *Controller) setupEveryReplicaRoutes() {
	if c.services.EveryReplicaRouter == nil {
		return
	}

	engine := c.services.BifrostEngine
	reloadGateway := func(ctx context.Context, revision int64) (bool, error) {
		reloaded, err := engine.Reload(revision)
		if reloaded {
			// The leader lists the gateway's models; have it list them again now that its
			// gateway has the new configuration.
			if err := llmengine.TriggerModelSync(ctx, c.services.StorageClient); err != nil {
				slog.Warn("failed to trigger LLM gateway model sync after a reload", "error", err)
			}
		}
		return reloaded, err
	}
	providerSync := providersync.New(c.services.ProviderDispatcher, c.services.LicenseProvider, reloadGateway)
	c.services.EveryReplicaRouter.Type(&v1.ProviderSync{}).
		Namespace(system.DefaultNamespace).
		Name(system.ProviderSyncName).
		HandlerFunc(providerSync.Reconcile)
}
