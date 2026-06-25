package server

import (
	"math"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-12
}

// assertInvariants checks normalized usage buckets.
func assertInvariants(t *testing.T, u types.TokenUsage, openAI bool) {
	t.Helper()
	if u.CacheReadTokens > u.InputTokens {
		t.Errorf("invariant: CacheReadTokens (%d) > InputTokens (%d)", u.CacheReadTokens, u.InputTokens)
	}
	if u.ThinkingTokens > u.OutputTokens {
		t.Errorf("invariant: ThinkingTokens (%d) > OutputTokens (%d)", u.ThinkingTokens, u.OutputTokens)
	}
	if u.InputTokens-u.CacheReadTokens < 0 {
		t.Errorf("invariant: uncached input negative (input=%d cacheRead=%d)", u.InputTokens, u.CacheReadTokens)
	}
	if openAI && u.CacheWriteTokens != 0 {
		t.Errorf("invariant: OpenAI must have zero cache writes, got %d", u.CacheWriteTokens)
	}
}

// observeAll feeds lines to a tracker and returns the computed usage.
func observeAll(c tokenUsageTracker, lines ...string) types.TokenUsage {
	for _, l := range lines {
		c.addTokenUsage([]byte(l))
	}
	return c.getTokenUsage()
}

func TestMessageTokenUsageTracker(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  types.TokenUsage
	}{
		{
			name: "stream with cache read + ttl breakdown + thinking",
			lines: []string{
				`{"type":"message_start","message":{"usage":{"input_tokens":50,"cache_read_input_tokens":100000,"cache_creation":{"ephemeral_5m_input_tokens":200,"ephemeral_1h_input_tokens":300},"output_tokens":1}}}`,
				`{"type":"message_delta","usage":{"output_tokens":800,"output_tokens_details":{"thinking_tokens":600}}}`,
			},
			want: types.TokenUsage{InputTokens: 100550, CacheReadTokens: 100000, CacheWriteTokens: 500, OutputTokens: 800, ThinkingTokens: 600, TotalTokens: 101350},
		},
		{
			name: "web-search growth: delta input_tokens supersedes start",
			lines: []string{
				`{"type":"message_start","message":{"usage":{"input_tokens":2679,"output_tokens":3}}}`,
				`{"type":"message_delta","usage":{"input_tokens":10682,"output_tokens":510}}`,
			},
			want: types.TokenUsage{InputTokens: 10682, OutputTokens: 510, TotalTokens: 11192},
		},
		{
			name: "non-streaming body, no ttl breakdown → aggregate counts as cache write",
			lines: []string{
				`{"usage":{"input_tokens":10,"cache_read_input_tokens":40,"cache_creation_input_tokens":500,"output_tokens":20}}`,
			},
			want: types.TokenUsage{InputTokens: 550, CacheReadTokens: 40, CacheWriteTokens: 500, OutputTokens: 20, TotalTokens: 570},
		},
		{
			name: "ttl breakdown present is authoritative (no double count of aggregate)",
			lines: []string{
				`{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_creation_input_tokens":500,"cache_creation":{"ephemeral_5m_input_tokens":500,"ephemeral_1h_input_tokens":0},"output_tokens":1}}}`,
				`{"type":"message_delta","usage":{"output_tokens":5}}`,
			},
			want: types.TokenUsage{InputTokens: 510, CacheWriteTokens: 500, OutputTokens: 5, TotalTokens: 515},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := observeAll(&messageTokenUsageTracker{}, tt.lines...)
			if got != tt.want {
				t.Errorf("getTokenUsage() = %+v, want %+v", got, tt.want)
			}
			assertInvariants(t, got, false)
		})
	}
}

func TestResponseTokenUsageTracker(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  types.TokenUsage
	}{
		{
			name: "stream response.completed with cached + reasoning",
			lines: []string{
				`{"type":"response.completed","response":{"usage":{"input_tokens":2006,"input_tokens_details":{"cached_tokens":1920},"output_tokens":300,"output_tokens_details":{"reasoning_tokens":120},"total_tokens":2306}}}`,
			},
			want: types.TokenUsage{InputTokens: 2006, CacheReadTokens: 1920, OutputTokens: 300, ThinkingTokens: 120, TotalTokens: 2306},
		},
		{
			name: "non-streaming top-level usage",
			lines: []string{
				`{"usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15}}`,
			},
			want: types.TokenUsage{InputTokens: 5, OutputTokens: 10, TotalTokens: 15},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := observeAll(&responseTokenUsageTracker{}, tt.lines...)
			if got != tt.want {
				t.Errorf("getTokenUsage() = %+v, want %+v", got, tt.want)
			}
			assertInvariants(t, got, true)
		})
	}
}

// TestTokenUsageTrackerCosts_WorkedExamples verifies tracker cost math.
func TestTokenUsageTrackerCosts_WorkedExamples(t *testing.T) {
	t.Run("anthropic cached + thinking (Opus 4.8)", func(t *testing.T) {
		c := &messageTokenUsageTracker{cost: types2.ModelCost{TokenUsageCost: types2.TokenUsageCost{Input: 5, Output: 25, CacheRead: 0.5, CacheWrite: 6.25}}}
		c.addTokenUsage([]byte(`{"usage":{"input_tokens":50,"cache_read_input_tokens":100000,"output_tokens":800,"output_tokens_details":{"thinking_tokens":600}}}`))
		u := c.getTokenUsage()

		if !approx(u.CacheReadSpend, 0.05) { // 100000 * 0.5/1e6
			t.Errorf("CacheReadSpend = %v, want 0.05", u.CacheReadSpend)
		}
		if !approx(u.CacheWriteSpend, 0) {
			t.Errorf("CacheWriteSpend = %v, want 0", u.CacheWriteSpend)
		}
		if !approx(u.InputSpend, 0.05025) { // uncached(50*5/1e6=0.00025) + cacheRead(0.05) + cacheWrite(0)
			t.Errorf("InputSpend = %v, want 0.05025", u.InputSpend)
		}
		if !approx(u.OutputSpend, 0.02) { // 800 * 25/1e6 (thinking is a sub-slice of output, not added)
			t.Errorf("OutputSpend = %v, want 0.02", u.OutputSpend)
		}
		if !approx(u.TotalSpend, 0.07025) {
			t.Errorf("TotalSpend = %v, want 0.07025", u.TotalSpend)
		}
		if !approx(u.TotalSpend, u.InputSpend+u.OutputSpend) {
			t.Error("TotalSpend should equal InputSpend + OutputSpend")
		}
	})

	t.Run("openai responses (gpt-5)", func(t *testing.T) {
		c := &responseTokenUsageTracker{cost: types2.ModelCost{TokenUsageCost: types2.TokenUsageCost{Input: 1.25, Output: 10, CacheRead: 0.125}}}
		c.addTokenUsage([]byte(`{"usage":{"input_tokens":2006,"input_tokens_details":{"cached_tokens":1920},"output_tokens":300,"output_tokens_details":{"reasoning_tokens":120},"total_tokens":2306}}`))
		u := c.getTokenUsage()

		if !approx(u.CacheReadSpend, 0.00024) { // 1920 * 0.125/1e6
			t.Errorf("CacheReadSpend = %v, want 0.00024", u.CacheReadSpend)
		}
		if !approx(u.InputSpend, 0.0003475) { // uncached(86*1.25/1e6=0.0001075) + cacheRead(0.00024)
			t.Errorf("InputSpend = %v, want 0.0003475", u.InputSpend)
		}
		if !approx(u.OutputSpend, 0.003) { // 300 * 10/1e6
			t.Errorf("OutputSpend = %v, want 0.003", u.OutputSpend)
		}
		if !approx(u.TotalSpend, 0.0033475) {
			t.Errorf("TotalSpend = %v, want 0.0033475", u.TotalSpend)
		}
	})

	t.Run("unpriced model leaves spend zero", func(t *testing.T) {
		c := &messageTokenUsageTracker{} // zero-value cost → unpriced
		c.addTokenUsage([]byte(`{"usage":{"input_tokens":100,"output_tokens":40}}`))
		u := c.getTokenUsage()
		if u.TotalSpend != 0 || u.InputSpend != 0 || u.OutputSpend != 0 {
			t.Errorf("unpriced model must leave spend 0, got total=%v", u.TotalSpend)
		}
		if u.InputTokens != 100 || u.OutputTokens != 40 {
			t.Errorf("tokens must be intact when unpriced: %+v", u)
		}
	})
}

// TestMessageTokenUsageTracker_CacheWriteSplitPricing verifies cache-write rates.
func TestMessageTokenUsageTracker_CacheWriteSplitPricing(t *testing.T) {
	c := &messageTokenUsageTracker{cost: types2.ModelCost{TokenUsageCost: types2.TokenUsageCost{CacheWrite: 6.25, CacheWrite1h: 10}}}
	c.addTokenUsage([]byte(`{"usage":{"input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":400,"ephemeral_1h_input_tokens":1000},"output_tokens":0}}`))
	u := c.getTokenUsage()

	if u.CacheWriteTokens != 1400 {
		t.Fatalf("CacheWriteTokens = %d, want 1400", u.CacheWriteTokens)
	}
	if !approx(u.CacheWriteSpend, 0.0125) {
		t.Errorf("CacheWriteSpend = %v, want 0.0125", u.CacheWriteSpend)
	}
	if !approx(u.InputSpend, 0.0125) {
		t.Errorf("InputSpend = %v, want 0.0125 (cache-write spend folds into InputSpend)", u.InputSpend)
	}
}

// TestEffectiveTokenCost_ContextTier verifies context tier selection.
func TestEffectiveTokenCost_ContextTier(t *testing.T) {
	size := 200000
	cost := types2.ModelCost{
		TokenUsageCost: types2.TokenUsageCost{Input: 2.5, Output: 15, CacheRead: 0.25},
		Tiers: []types2.ModelCostTier{{
			TokenUsageCost: types2.TokenUsageCost{Input: 5, Output: 22.5, CacheRead: 0.5},
			Type:           types2.ModelCostTierTypeContext,
			Size:           &size,
		}},
	}

	if got := costForTier(cost, 199999); got.Input != 2.5 {
		t.Errorf("below threshold: Input = %v, want base 2.5", got.Input)
	}
	if got := costForTier(cost, 200000); got.Input != 5 {
		t.Errorf("at threshold: Input = %v, want tier 5", got.Input)
	}
	if got := costForTier(cost, 500000); got.Output != 22.5 {
		t.Errorf("above threshold: Output = %v, want tier 22.5", got.Output)
	}
}

// TestMessageTokenUsageTracker_TierByContextSize verifies tiered message pricing.
func TestMessageTokenUsageTracker_TierByContextSize(t *testing.T) {
	size := 200000
	cost := types2.ModelCost{
		TokenUsageCost: types2.TokenUsageCost{Input: 5},
		Tiers:          []types2.ModelCostTier{{TokenUsageCost: types2.TokenUsageCost{Input: 10}, Type: types2.ModelCostTierTypeContext, Size: &size}},
	}
	c := &messageTokenUsageTracker{cost: cost}
	c.addTokenUsage([]byte(`{"usage":{"input_tokens":150000,"cache_read_input_tokens":60000,"output_tokens":0}}`))
	u := c.getTokenUsage()
	if !approx(u.InputSpend, 1.5) {
		t.Errorf("InputSpend = %v, want 1.5 (tier rate applied via context size)", u.InputSpend)
	}
}
