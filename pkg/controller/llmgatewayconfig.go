package controller

import (
	"errors"

	"github.com/obot-platform/nah/pkg/router"

	"github.com/obot-platform/obot/pkg/controller/handlers/llmgatewayconfig"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// setupLLMGatewayConfigRoutes activates resource reconciliation for the core runtime.
// The legacy transport uses its reload mechanism; cutover calls this helper when
// the service engine implements the core lifecycle interface.
func (c *Controller) setupLLMGatewayConfigRoutes(engine llmgatewayconfig.Engine) {
	if c.services.EveryReplicaRouter == nil {
		return
	}
	configSync := llmgatewayconfig.New(engine)
	reconcile := func(req router.Request, resp router.Response) error {
		if !engine.Ready() {
			return configSync.Reconcile(req, resp)
		}
		// Apply successfully imported resources even when another import fails;
		// join both errors so failed imports and runtime application remain retryable.
		var importErr error
		if c.services.GatewayClient != nil {
			_, importErr = c.services.BifrostEngine.ImportLegacyModelProviders(req.Ctx, c.services.StorageClient, c.services.GatewayClient)
		}
		return errors.Join(importErr, configSync.Reconcile(req, resp))
	}
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayProvider{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(reconcile)
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayRoutingRule{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(reconcile)
	// Global settings use only the default singleton. Providers and rules have
	// independent names, so their watches remain unfiltered.
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayConfig{}).
		Namespace(system.DefaultNamespace).
		Name(system.LLMGatewayConfigName).
		IncludeRemoved().
		HandlerFunc(reconcile)
}
