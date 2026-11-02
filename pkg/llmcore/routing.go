package llmcore

import (
	"strings"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/plugins/governance"
	"github.com/maximhq/bifrost/plugins/routing/rules"
)

const (
	routingPluginName = "obot-routing"
)

// routingPlugin applies routing rules once per request, before any attempt, using the engine's
// own rule evaluator. Bifrost's routing plugin can't be used as is: it loads rules from
// Bifrost's config store and needs Bifrost's governance plugin for virtual keys.
type routingPlugin struct {
	rules  rules.Store
	engine *rules.Engine
	logger schemas.Logger
}

// noBudgets answers the budget variables routing rules can read (budget_used, tokens_used,
// request) with nothing. Obot could answer them from its own budgets, which are accurate across
// replicas, unlike the engine's.
type noBudgets struct{}

func (noBudgets) GetBudgetAndRateLimitStatus(*schemas.BifrostContext, schemas.ModelProvider, string, map[string]float64, map[string]int64, map[string]int64) *governance.BudgetAndRateLimitStatus {
	return nil
}

func newRoutingPlugin(store rules.Store, logger schemas.Logger) (*routingPlugin, error) {
	depth := rules.DefaultChainMaxDepth
	engine, err := rules.NewEngine(store, noBudgets{}, logger, &depth)
	if err != nil {
		return nil, err
	}
	return &routingPlugin{rules: store, engine: engine, logger: logger}, nil
}

func (p *routingPlugin) GetName() string { return routingPluginName }

func (p *routingPlugin) Cleanup() error { return nil }

func (p *routingPlugin) PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	if !p.rules.HasRules(ctx) {
		return nil
	}
	provider, model, _ := req.GetRequestFields()
	if model == "" {
		return nil
	}

	headers, _ := ctx.Value(schemas.BifrostContextKeyRequestHeaders).(map[string]string)
	decision, err := p.engine.EvaluateRoutingRules(ctx, &rules.EvaluationContext{
		Provider:    provider,
		Model:       model,
		RequestType: string(req.RequestType),
		Headers:     headers,
	})
	if err != nil {
		// A rule that fails to evaluate leaves the request as it was, as the engine's plugin does.
		p.logger.Warn("failed to evaluate routing rules: %v", err)
		return nil
	}

	if decision == nil {
		return nil
	}
	if decision.Provider != "" {
		req.SetProvider(schemas.ModelProvider(decision.Provider))
	}
	if decision.Model != "" {
		req.SetModel(decision.Model)
	}
	if len(decision.Fallbacks) > 0 {
		fallbacks := make([]schemas.Fallback, 0, len(decision.Fallbacks))
		for _, fb := range decision.Fallbacks {
			resolved := fb.Resolved()
			resolved.Provider = schemas.ModelProvider(strings.TrimSpace(string(resolved.Provider)))
			resolved.Model = strings.TrimSpace(resolved.Model)
			if resolved.Provider == "" {
				continue
			}
			if resolved.Model == "" {
				resolved.Model = model
			}
			fallbacks = append(fallbacks, resolved)
		}
		req.SetFallbacks(fallbacks)
	}
	if decision.KeyID != "" {
		ctx.SetValue(schemas.BifrostContextKeyRoutingPinnedAPIKeyID, decision.KeyID)
	}
	if decision.TTFTTimeout > 0 && bifrost.IsStreamRequestType(req.RequestType) {
		ctx.SetValue(schemas.BifrostContextKeyStreamFirstTokenTimeout, decision.TTFTTimeout)
	}
	return nil
}

func (p *routingPlugin) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

func (p *routingPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}
