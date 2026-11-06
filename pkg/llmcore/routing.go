package llmcore

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
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
	// Bifrost replaces scope entries and compiled programs in separate steps.
	// Hold a read lock for the whole evaluation and a write lock for mutations
	// so requests never see a missing rule or mix rules with another program version.
	mu        sync.RWMutex
	rules     rules.Store
	validator *ruleValidator
	engine    *rules.Engine
	logger    schemas.Logger
	account   *account
}

// noBudgets answers the budget variables routing rules can read (budget_used, tokens_used,
// request) with nothing. Obot could answer them from its own budgets, which are accurate across
// replicas, unlike the engine's.
type noBudgets struct{}

func (noBudgets) GetBudgetAndRateLimitStatus(*schemas.BifrostContext, schemas.ModelProvider, string, map[string]float64, map[string]int64, map[string]int64) *governance.BudgetAndRateLimitStatus {
	return nil
}

func newRoutingPlugin(store rules.Store, logger schemas.Logger, account *account) (*routingPlugin, error) {
	depth := rules.DefaultChainMaxDepth
	engine, err := rules.NewEngine(store, noBudgets{}, logger, &depth)
	if err != nil {
		return nil, err
	}
	validator, err := newRuleValidator()
	if err != nil {
		return nil, err
	}
	return &routingPlugin{rules: store, validator: validator, engine: engine, logger: logger, account: account}, nil
}

// UpsertRule owns all live edits to the store; callers must not mutate it directly.
func (p *routingPlugin) UpsertRule(ctx context.Context, rule *tables.TableRoutingRule) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if rule == nil {
		return fmt.Errorf("routing rule cannot be nil")
	}

	// A matching rule with unusable targets is skipped by the evaluator, which
	// would silently bypass policy if it replaced a working rule.
	if len(rule.Targets) == 0 {
		return fmt.Errorf("routing rule %s: at least one target is required", rule.ID)
	}

	var total float64
	for i, target := range rule.Targets {
		if target.Weight < 0 || math.IsNaN(target.Weight) || math.IsInf(target.Weight, 0) {
			return fmt.Errorf("routing rule %s: target %d weight must be finite and nonnegative", rule.ID, i)
		}
		total += target.Weight
	}

	if math.IsInf(total, 0) {
		return fmt.Errorf("routing rule %s: total target weight must be finite", rule.ID)
	}

	// LocalStore replaces the active rule even when compilation fails and only
	// logs the error. Validate a fresh, boolean-typed expression before publishing;
	// the live program cache is keyed by ID and may still contain the old program.
	if err := p.validator.validate(rule.CelExpression); err != nil {
		return fmt.Errorf("routing rule %s: %w", rule.ID, err)
	}
	return p.rules.UpsertRule(ctx, rule)
}

func (p *routingPlugin) DeleteRule(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rules.DeleteRule(ctx, id)
}

func (p *routingPlugin) GetName() string { return routingPluginName }

func (p *routingPlugin) Cleanup() error { return nil }

// PreRequestHook runs once before Bifrost selects keys or starts retries and
// fallbacks. Resolve the primary route and fallback chain here so later attempts
// use that decision instead of evaluating routing rules again.
func (p *routingPlugin) PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.rules.HasRules(ctx) {
		return nil
	}
	provider, model, _ := req.GetRequestFields()
	if model == "" {
		return nil
	}

	headers, _ := ctx.Value(schemas.BifrostContextKeyRequestHeaders).(map[string]string)
	query, _ := ctx.Value(schemas.BifrostContextKeyRequestQuery).(map[string]string)
	decision, err := p.engine.EvaluateRoutingRules(ctx, &rules.EvaluationContext{
		Provider:    provider,
		Model:       model,
		RequestType: string(req.RequestType),
		Headers:     headers,
		QueryParams: query,
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
		cfg := requestModelConfig(ctx, p.account)
		fallbacks := make([]schemas.Fallback, 0, len(decision.Fallbacks))
		for _, fb := range decision.Fallbacks {
			resolved := resolveConfiguredRuleFallback(cfg, fb)
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
