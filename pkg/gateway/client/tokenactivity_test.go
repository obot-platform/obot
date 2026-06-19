package client

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// TestTokenActivityRoundTrip verifies TokenUsage persistence and aggregation.
func TestTokenActivityRoundTrip(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	now := time.Now()

	rows := []types.RunTokenActivity{
		{
			UserID:    "u1",
			Model:     "claude-opus-4-5",
			CreatedAt: now,
			Usage: types.TokenUsage{
				InputTokens:      100050,
				CacheReadTokens:  100000,
				CacheWriteTokens: 500,
				OutputTokens:     800,
				ThinkingTokens:   600,
				TotalTokens:      100050 + 500 + 800,
				InputSpend:       0.00025,
				CacheReadSpend:   0.05,
				CacheWriteSpend:  0.0,
				OutputSpend:      0.02,
				TotalSpend:       0.07025,
			},
		},
		{
			UserID:    "u1",
			Model:     "gpt-5",
			CreatedAt: now,
			Usage: types.TokenUsage{
				InputTokens:  2006,
				OutputTokens: 300,
				TotalTokens:  2306,
				TotalSpend:   0.0033475,
			},
		},
	}
	for i := range rows {
		if err := c.InsertTokenUsage(ctx, &rows[i]); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	got, err := c.TokenUsageForUser(ctx, "u1", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("TokenUsageForUser: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	var opus types.RunTokenActivity
	for _, r := range got {
		if r.Model == "claude-opus-4-5" {
			opus = r
		}
	}
	if opus.Usage.CacheReadTokens != 100000 || opus.Usage.CacheWriteTokens != 500 {
		t.Errorf("cache buckets wrong: %+v", opus.Usage)
	}
	if opus.Usage.ThinkingTokens != 600 {
		t.Errorf("thinking = %d, want 600", opus.Usage.ThinkingTokens)
	}
	if math.Abs(opus.Usage.TotalSpend-0.07025) > 1e-9 {
		t.Errorf("total spend = %v, want 0.07025", opus.Usage.TotalSpend)
	}

	total, err := c.TotalTokenUsageForUser(ctx, "u1", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("TotalTokenUsageForUser: %v", err)
	}
	if total.Usage.InputTokens != 100050+2006 {
		t.Errorf("summed input = %d, want %d", total.Usage.InputTokens, 100050+2006)
	}
	if total.Usage.OutputTokens != 800+300 {
		t.Errorf("summed output = %d, want %d", total.Usage.OutputTokens, 800+300)
	}
	if total.Usage.CacheReadTokens != 100000 {
		t.Errorf("summed cache read = %d, want 100000", total.Usage.CacheReadTokens)
	}
	if want := 0.07025 + 0.0033475; math.Abs(total.Usage.TotalSpend-want) > 1e-9 {
		t.Errorf("summed total spend = %v, want %v", total.Usage.TotalSpend, want)
	}
}
