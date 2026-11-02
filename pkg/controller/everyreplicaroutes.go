package controller

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/handlers/providersync"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (c *Controller) setupEveryReplicaRoutes() {
	if c.services.EveryReplicaRouter == nil {
		return
	}

	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayProvider{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(c.reconcileLLMGatewayConfig)
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayRoutingRule{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(c.reconcileLLMGatewayConfig)
	// Global settings are a singleton: Load and GetSettings read only "default".
	// Providers and routing rules can have multiple names, so their watches are unfiltered.
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayConfig{}).
		Namespace(system.DefaultNamespace).
		Name("default").
		IncludeRemoved().
		HandlerFunc(c.reconcileLLMGatewayConfig)

	providerSync := providersync.New(c.services.ProviderDispatcher, c.services.LicenseProvider, nil)
	c.services.EveryReplicaRouter.Type(&v1.ProviderSync{}).
		Namespace(system.DefaultNamespace).
		Name(system.ProviderSyncName).
		HandlerFunc(providerSync.Reconcile)
}

// Reload current desired resources rather than applying a possibly stale event payload.
func (c *Controller) reconcileLLMGatewayConfig(req router.Request, resp router.Response) error {
	if !c.services.BifrostEngine.Ready() {
		resp.RetryAfter(time.Second)
		return nil
	}
	if _, err := c.services.BifrostEngine.Sync(req.Ctx); err != nil {
		return err
	}
	// A prior attempt may have applied the configuration but failed to trigger discovery.
	// Retry discovery even when Sync reports no runtime changes.
	return llmengine.TriggerModelSync(req.Ctx, c.services.StorageClient)
}
