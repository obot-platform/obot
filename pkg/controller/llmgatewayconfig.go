package controller

import (
	"context"
	"errors"

	"github.com/obot-platform/nah/pkg/router"

	"github.com/obot-platform/obot/pkg/controller/handlers/llmgatewayconfig"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

type legacyGatewayConfigSync struct {
	llmgatewayconfig.Engine
	controller *Controller
}

// setupLLMGatewayConfigRoutes applies runtime state on every replica. Only the
// elected router imports legacy resources; both handlers coalesce overlapping events.
func (c *Controller) setupLLMGatewayConfigRoutes(engine llmgatewayconfig.Engine) {
	if c.services.EveryReplicaRouter != nil {
		registerLLMGatewayConfigRoutes(c.services.EveryReplicaRouter, llmgatewayconfig.New(engine).Reconcile)
	}
	if c.services.Router != nil {
		leaderSync := &legacyGatewayConfigSync{Engine: engine, controller: c}
		registerLLMGatewayConfigRoutes(c.services.Router, llmgatewayconfig.New(leaderSync).Reconcile)
	}
}

func (s *legacyGatewayConfigSync) Sync(ctx context.Context) (bool, error) {
	c := s.controller
	var importErr error
	if c.services.GatewayClient != nil {
		_, importErr = c.services.BifrostEngine.ImportLegacyModelProviders(ctx, c.services.StorageClient, c.services.GatewayClient)
	}
	// Apply independent successes even when another family fails; retry both errors.
	changed, syncErr := s.Engine.Sync(ctx)
	return changed, errors.Join(importErr, syncErr)
}

func registerLLMGatewayConfigRoutes(r *router.Router, reconcile router.HandlerFunc) {
	r.Type(&v1.LLMGatewayProvider{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(reconcile)
	r.Type(&v1.LLMGatewayRoutingRule{}).
		Namespace(system.DefaultNamespace).
		IncludeRemoved().
		HandlerFunc(reconcile)
	// Global settings use only the default singleton. Providers and rules have
	// independent names, so their watches remain unfiltered.
	r.Type(&v1.LLMGatewayConfig{}).
		Namespace(system.DefaultNamespace).
		Name(system.LLMGatewayConfigName).
		IncludeRemoved().
		HandlerFunc(reconcile)
}
