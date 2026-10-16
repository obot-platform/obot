package llmengine

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestCostFromMetadata(t *testing.T) {
	cost, ok := CostFromMetadata(map[string]string{
		MetadataInputCost:      "1e-07",
		MetadataOutputCost:     "5e-07",
		MetadataCacheReadCost:  "1e-08",
		MetadataCacheWriteCost: "1.25e-07",
	})
	// Exact per-million prices, without float error from the conversion (1e-07 * 1e6 is
	// 0.09999999999999999).
	if want := (types.ModelCost{Input: 0.1, Output: 0.5, CacheRead: 0.01, CacheWrite: 0.125}); !ok || cost.TokenUsageCost != want.TokenUsageCost {
		t.Errorf("cost = %+v, %v; want %+v", cost, ok, want)
	}
	if _, ok := CostFromMetadata(map[string]string{}); ok {
		t.Error("a model without prices has a cost")
	}
}
