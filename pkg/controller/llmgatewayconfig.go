package controller

import (
	"context"

	"github.com/obot-platform/obot/pkg/controller/handlers/llmgatewayconfig"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// setupLLMGatewayConfigRoutes activates resource reconciliation for the core runtime.
// The legacy transport uses its reload mechanism; cutover calls this helper when
// the service engine implements the core lifecycle interface.
//
//nolint:unused // Activated by core-runtime cutover (https://github.com/obot-platform/obot/issues/8248); the legacy transport does not register these watches.
func (c *Controller) setupLLMGatewayConfigRoutes(engine llmgatewayconfig.Engine) {
	if c.services.EveryReplicaRouter == nil {
		return
	}
	configSync := llmgatewayconfig.New(engine, func(ctx context.Context) error {
		return llmengine.TriggerModelSync(ctx, c.services.StorageClient)
	})
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayProvider{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(configSync.Reconcile)
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayRoutingRule{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(configSync.Reconcile)
	// Global settings use only the default singleton. Providers and rules have
	// independent names, so their watches remain unfiltered.
	c.services.EveryReplicaRouter.Type(&v1.LLMGatewayConfig{}).
		Namespace(system.DefaultNamespace).
		Name("default").
		IncludeRemoved().
		HandlerFunc(configSync.Reconcile)
}
