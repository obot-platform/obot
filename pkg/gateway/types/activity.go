//nolint:revive
package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

type APIActivity struct {
	ID     uint
	UserID string
	Date   time.Time
}

func ConvertAPIActivity(a APIActivity) types2.APIActivity {
	return types2.APIActivity{
		UserID: a.UserID,
		Date:   *types2.NewTime(a.Date),
	}
}

type RunTokenActivity struct {
	ID        uint
	CreatedAt time.Time
	Name      string
	UserID    string
	Model     string

	Usage TokenUsage `gorm:"embedded"`
}

// TokenUsage is normalized token usage and spend.
type TokenUsage struct {
	// InputTokens is the total input: CacheReadTokens + CacheWriteTokens + uncached input.
	InputTokens int
	// CacheReadTokens is the cache-hit input tokens; a subset of InputTokens.
	CacheReadTokens int
	// CacheWriteTokens is the cache-write input tokens (5m + 1h); a subset of InputTokens
	// (Anthropic only).
	CacheWriteTokens int
	// OutputTokens is the total output, including ThinkingTokens.
	OutputTokens int
	// ThinkingTokens is the thinking/reasoning output tokens; a subset of OutputTokens.
	ThinkingTokens int
	// TotalTokens is InputTokens + OutputTokens.
	TotalTokens int

	// InputSpend is the total USD on InputTokens (each bucket at its own rate):
	// CacheReadSpend + CacheWriteSpend + uncached-input spend. 0 when unpriced.
	InputSpend float64
	// CacheReadSpend is the USD on CacheReadTokens; a subset of InputSpend.
	CacheReadSpend float64
	// CacheWriteSpend is the USD on CacheWriteTokens; a subset of InputSpend.
	CacheWriteSpend float64
	// OutputSpend is the USD on OutputTokens.
	OutputSpend float64
	// TotalSpend is InputSpend + OutputSpend.
	TotalSpend float64
}

func ConvertTokenActivity(a RunTokenActivity) types2.TokenUsage {
	return types2.TokenUsage{
		UserID:           a.UserID,
		Model:            a.Model,
		Date:             *types2.NewTime(a.CreatedAt),
		InputTokens:      a.Usage.InputTokens,
		CacheReadTokens:  a.Usage.CacheReadTokens,
		CacheWriteTokens: a.Usage.CacheWriteTokens,
		OutputTokens:     a.Usage.OutputTokens,
		ThinkingTokens:   a.Usage.ThinkingTokens,
		TotalTokens:      a.Usage.TotalTokens,
		InputSpend:       a.Usage.InputSpend,
		CacheReadSpend:   a.Usage.CacheReadSpend,
		CacheWriteSpend:  a.Usage.CacheWriteSpend,
		OutputSpend:      a.Usage.OutputSpend,
		TotalSpend:       a.Usage.TotalSpend,
	}
}

type RemainingTokenUsage struct {
	InputTokens           int
	OutputTokens          int
	UnlimitedInputTokens  bool
	UnlimitedOutputTokens bool
}

func (r RemainingTokenUsage) IsDepleted() bool {
	return !r.UnlimitedInputTokens && r.InputTokens <= 0 ||
		!r.UnlimitedOutputTokens && r.OutputTokens <= 0
}

func ConvertRemainingTokenUsage(userID string, r *RemainingTokenUsage) types2.RemainingTokenUsage {
	return types2.RemainingTokenUsage{
		UserID:                userID,
		InputTokens:           r.InputTokens,
		OutputTokens:          r.OutputTokens,
		UnlimitedInputTokens:  r.UnlimitedInputTokens,
		UnlimitedOutputTokens: r.UnlimitedOutputTokens,
	}
}
