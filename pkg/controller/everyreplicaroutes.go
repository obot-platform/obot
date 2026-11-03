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
	reload := lockedGatewayReload(c.services.GatewayClient.WithLLMGatewayLock, engine.Reload)
	reloadGateway := func(ctx context.Context, revision int64) (bool, error) {
		reloaded, err := reload(ctx, revision)
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

// lockedGatewayReload reloads this replica's LLM gateway while holding the lock every replica
// takes to change the gateway's configuration. A reload rewrites every provider in the shared
// store from what it read when it started, so without the lock, a reload on one replica could
// undo a change another replica made in the meantime.
func lockedGatewayReload(withLock func(context.Context, func() error) error, reload func(int64) (bool, error)) func(context.Context, int64) (bool, error) {
	return func(ctx context.Context, revision int64) (bool, error) {
		var reloaded bool
		err := withLock(ctx, func() error {
			var err error
			reloaded, err = reload(revision)
			return err
		})
		return reloaded, err
	}
}
