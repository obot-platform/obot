package server

import (
	"sync"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/tidwall/gjson"
)

// tokenUsageTracker accumulates usage for one upstream response.
type tokenUsageTracker interface {
	addTokenUsage(line []byte)
	getTokenUsage() types.TokenUsage
	setCost(cost types2.ModelCost)
	modelCost() types2.ModelCost
}

// threadSafeTokenUsageTracker serializes access to a nil-safe usage tracker.
type threadSafeTokenUsageTracker struct {
	lock  sync.Mutex
	inner tokenUsageTracker
	// overridden is true when an administrator set the prices of the model usage is costed
	// for.
	overridden bool
}

// messageTokenUsageTracker tracks Anthropic Messages usage.
type messageTokenUsageTracker struct {
	cost                                                                 types2.ModelCost
	inputTokens, cacheRead, cacheWrite5m, cacheWrite1h, output, thinking int
}

// responseTokenUsageTracker tracks OpenAI Responses usage.
//
// Streaming responses emit usage under response.usage on terminal events;
// non-streaming responses put the same shape at the top-level usage field.
type responseTokenUsageTracker struct {
	cost                                         types2.ModelCost
	inputTokens, cachedTokens, output, reasoning int
}

// chatCompletionsTokenUsageTracker tracks OpenAI Chat Completions usage.
//
// Streaming responses carry usage on the final chunk; non-streaming responses put it at
// the top-level usage field.
type chatCompletionsTokenUsageTracker struct {
	cost                                         types2.ModelCost
	inputTokens, cachedTokens, output, reasoning int
}

// newTokenUsageTracker chooses a parser that matches the upstream usage shape.
func newTokenUsageTracker(model v1.Model) *threadSafeTokenUsageTracker {
	return newTokenUsageTrackerForDialect(model, llmtypes.Dialect(model.Spec.Manifest.Dialect))
}

// newTokenUsageTrackerForDialect chooses a parser for the given response dialect. The LLM
// gateway serves every model in every dialect, so its tracker follows the route rather than
// the model's native dialect.
func newTokenUsageTrackerForDialect(model v1.Model, dialect llmtypes.Dialect) *threadSafeTokenUsageTracker {
	var (
		cost  = model.GetCost()
		inner tokenUsageTracker
	)

	switch dialect {
	case llmtypes.DialectAnthropicMessages:
		inner = &messageTokenUsageTracker{cost: cost}
	case llmtypes.DialectOpenAIResponses, llmtypes.DialectOpenResponses:
		inner = &responseTokenUsageTracker{cost: cost}
	case llmtypes.DialectOpenAIChatCompletions:
		inner = &chatCompletionsTokenUsageTracker{cost: cost}
	default:
		return nil
	}

	return &threadSafeTokenUsageTracker{
		inner:      inner,
		overridden: model.Spec.Manifest.OverrideCost != nil,
	}
}

func (t *messageTokenUsageTracker) setCost(cost types2.ModelCost)         { t.cost = cost }
func (t *responseTokenUsageTracker) setCost(cost types2.ModelCost)        { t.cost = cost }
func (t *chatCompletionsTokenUsageTracker) setCost(cost types2.ModelCost) { t.cost = cost }

func (t *messageTokenUsageTracker) modelCost() types2.ModelCost         { return t.cost }
func (t *responseTokenUsageTracker) modelCost() types2.ModelCost        { return t.cost }
func (t *chatCompletionsTokenUsageTracker) modelCost() types2.ModelCost { return t.cost }

// setModelCost changes the prices usage is costed at, for a response served by another model
// than the requested one.
func (c *threadSafeTokenUsageTracker) setModelCost(model v1.Model) {
	if c == nil || c.inner == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.inner.setCost(model.GetCost())
	c.overridden = model.Spec.Manifest.OverrideCost != nil
}

func (c *threadSafeTokenUsageTracker) costOverridden() bool {
	if c == nil {
		return false
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.overridden
}

// gatewayTokenUsage is the token usage the gateway priced a response at.
func gatewayTokenUsage(cost guardrails.Cost) types.TokenUsage {
	return types.TokenUsage{
		InputTokens:      cost.InputTokens,
		CacheReadTokens:  cost.CacheReadTokens,
		CacheWriteTokens: cost.CacheWriteTokens,
		OutputTokens:     cost.OutputTokens,
		ThinkingTokens:   cost.ReasoningTokens,
		TotalTokens:      cost.InputTokens + cost.OutputTokens,
	}
}

// price costs usage at the prices of the model usage is costed for.
func (c *threadSafeTokenUsageTracker) price(usage types.TokenUsage) types.TokenUsage {
	if c == nil || c.inner == nil {
		return usage
	}
	c.lock.Lock()
	cost := costForTier(c.inner.modelCost(), usage.InputTokens)
	c.lock.Unlock()
	uncached := max(usage.InputTokens-usage.CacheReadTokens-usage.CacheWriteTokens, 0)
	usage.CacheReadSpend = spendUSD(usage.CacheReadTokens, cost.CacheRead)
	usage.CacheWriteSpend = spendUSD(usage.CacheWriteTokens, cost.CacheWrite)
	usage.InputSpend = spendUSD(uncached, cost.Input) + usage.CacheReadSpend + usage.CacheWriteSpend
	usage.OutputSpend = spendUSD(usage.OutputTokens, cost.Output)
	usage.TotalSpend = usage.InputSpend + usage.OutputSpend
	return usage
}

// withGatewayCost replaces usage's spend with what the gateway priced the response at.
func withGatewayCost(usage types.TokenUsage, cost guardrails.Cost) types.TokenUsage {
	usage.CacheReadSpend = cost.CacheRead
	usage.CacheWriteSpend = cost.CacheWrite
	usage.InputSpend = cost.Input
	usage.OutputSpend = cost.Output
	usage.TotalSpend = cost.Input + cost.Output
	return usage
}

func (c *threadSafeTokenUsageTracker) addTokenUsage(line []byte) {
	if c == nil || c.inner == nil {
		return
	}
	// The response body and Close path can both touch usage while streaming.
	c.lock.Lock()
	defer c.lock.Unlock()
	c.inner.addTokenUsage(line)
}

func (c *threadSafeTokenUsageTracker) getTokenUsage() types.TokenUsage {
	if c == nil || c.inner == nil {
		return types.TokenUsage{}
	}
	// Return a stable snapshot for logging even if more stream chunks arrive.
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.inner.getTokenUsage()
}

func (t *messageTokenUsageTracker) addTokenUsage(line []byte) {
	// Anthropic sends usage on message_start, message_delta, and
	// non-streaming responses, depending on response mode.
	for _, u := range []gjson.Result{
		gjson.GetBytes(line, "usage"),
		gjson.GetBytes(line, "message.usage"),
	} {
		if !u.Exists() {
			continue
		}
		// Anthropic stream usage is cumulative, and later events may add input
		// tokens for tool/web-search work, so keep the largest count observed.
		t.inputTokens = max(t.inputTokens, int(u.Get("input_tokens").Int()))
		t.cacheRead = max(t.cacheRead, int(u.Get("cache_read_input_tokens").Int()))
		t.output = max(t.output, int(u.Get("output_tokens").Int()))
		t.thinking = max(t.thinking, int(u.Get("output_tokens_details.thinking_tokens").Int()))

		c5m := u.Get("cache_creation.ephemeral_5m_input_tokens")
		c1h := u.Get("cache_creation.ephemeral_1h_input_tokens")
		if c5m.Exists() || c1h.Exists() {
			// Prefer TTL-specific cache writes when present so 5m and 1h writes
			// can use their separate prices.
			t.cacheWrite5m = max(t.cacheWrite5m, int(c5m.Int()))
			t.cacheWrite1h = max(t.cacheWrite1h, int(c1h.Int()))
		} else {
			// Older Anthropic payloads only report aggregate cache creation.
			t.cacheWrite5m = max(t.cacheWrite5m, int(u.Get("cache_creation_input_tokens").Int()))
		}
	}
}

func (t *messageTokenUsageTracker) getTokenUsage() types.TokenUsage {
	cacheWrite := t.cacheWrite5m + t.cacheWrite1h
	// Anthropic reports uncached input separately from cache read/write, but
	// Obot's normalized input total includes all prompt-side token buckets.
	input := t.inputTokens + t.cacheRead + cacheWrite
	u := types.TokenUsage{
		InputTokens:      input,
		CacheReadTokens:  t.cacheRead,
		CacheWriteTokens: cacheWrite,
		OutputTokens:     t.output,
		ThinkingTokens:   t.thinking,
		TotalTokens:      input + t.output,
	}
	cost := costForTier(t.cost, input)
	// Cache read/write spend is folded into InputSpend because it is still
	// prompt-side spend, while retaining separate buckets for reporting.
	u.CacheReadSpend = spendUSD(t.cacheRead, cost.CacheRead)
	u.CacheWriteSpend = spendUSD(t.cacheWrite5m, cost.CacheWrite) + spendUSD(t.cacheWrite1h, cost.CacheWrite1h)
	u.InputSpend = spendUSD(t.inputTokens, cost.Input) + u.CacheReadSpend + u.CacheWriteSpend
	u.OutputSpend = spendUSD(t.output, cost.Output)
	u.TotalSpend = u.InputSpend + u.OutputSpend
	return u
}

func (t *responseTokenUsageTracker) addTokenUsage(line []byte) {
	// Responses sends usage on terminal stream events or on the top-level
	// non-streaming response.
	for _, u := range []gjson.Result{
		gjson.GetBytes(line, "response.usage"),
		gjson.GetBytes(line, "usage"),
	} {
		if !u.Exists() {
			continue
		}
		// Responses usage can appear only on terminal stream events; max keeps
		// retries or duplicate terminal events from double counting.
		t.inputTokens = max(t.inputTokens, int(u.Get("input_tokens").Int()))
		t.cachedTokens = max(t.cachedTokens, int(u.Get("input_tokens_details.cached_tokens").Int()))
		t.output = max(t.output, int(u.Get("output_tokens").Int()))
		t.reasoning = max(t.reasoning, int(u.Get("output_tokens_details.reasoning_tokens").Int()))
	}
}

func (t *responseTokenUsageTracker) getTokenUsage() types.TokenUsage {
	// OpenAI input_tokens is cache-inclusive; cached_tokens is split out only for
	// pricing the cached portion at CacheRead.
	u := types.TokenUsage{
		InputTokens:     t.inputTokens,
		CacheReadTokens: t.cachedTokens,
		OutputTokens:    t.output,
		ThinkingTokens:  t.reasoning,
		TotalTokens:     t.inputTokens + t.output,
	}
	cost := costForTier(t.cost, t.inputTokens)
	// Only the non-cached portion should use the normal input rate.
	uncachedInput := max(t.inputTokens-t.cachedTokens, 0)
	u.CacheReadSpend = spendUSD(t.cachedTokens, cost.CacheRead)
	u.InputSpend = spendUSD(uncachedInput, cost.Input) + u.CacheReadSpend
	u.OutputSpend = spendUSD(t.output, cost.Output)
	u.TotalSpend = u.InputSpend + u.OutputSpend
	return u
}

func (t *chatCompletionsTokenUsageTracker) addTokenUsage(line []byte) {
	u := gjson.GetBytes(line, "usage")
	if !u.Exists() || u.Type == gjson.Null {
		return
	}
	t.inputTokens = max(t.inputTokens, int(u.Get("prompt_tokens").Int()))
	t.cachedTokens = max(t.cachedTokens, int(u.Get("prompt_tokens_details.cached_tokens").Int()))
	t.output = max(t.output, int(u.Get("completion_tokens").Int()))
	t.reasoning = max(t.reasoning, int(u.Get("completion_tokens_details.reasoning_tokens").Int()))
}

func (t *chatCompletionsTokenUsageTracker) getTokenUsage() types.TokenUsage {
	// prompt_tokens is cache-inclusive, like Responses input_tokens.
	u := types.TokenUsage{
		InputTokens:     t.inputTokens,
		CacheReadTokens: t.cachedTokens,
		OutputTokens:    t.output,
		ThinkingTokens:  t.reasoning,
		TotalTokens:     t.inputTokens + t.output,
	}
	cost := costForTier(t.cost, t.inputTokens)
	uncachedInput := max(t.inputTokens-t.cachedTokens, 0)
	u.CacheReadSpend = spendUSD(t.cachedTokens, cost.CacheRead)
	u.InputSpend = spendUSD(uncachedInput, cost.Input) + u.CacheReadSpend
	u.OutputSpend = spendUSD(t.output, cost.Output)
	u.TotalSpend = u.InputSpend + u.OutputSpend
	return u
}

// costForTier selects the largest applicable context-tier rate.
func costForTier(cost types2.ModelCost, contextSize int) types2.TokenUsageCost {
	result := cost.TokenUsageCost
	best := -1
	for _, tier := range cost.Tiers {
		if tier.Type != types2.ModelCostTierTypeContext || tier.Size == nil {
			continue
		}
		if *tier.Size <= contextSize && *tier.Size > best {
			best = *tier.Size
			result = tier.TokenUsageCost
		}
	}
	return result
}

func spendUSD(tokens int, ratePerMTok float64) float64 {
	// Model cost rates are stored as dollars per million tokens.
	return float64(tokens) * ratePerMTok / 1e6
}
