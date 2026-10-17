//nolint:revive
package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

type APIActivity struct {
	ID     uint
	UserID string    `gorm:"index:idx_api_activity_date_user,priority:2"`
	Date   time.Time `gorm:"index:idx_api_activity_date_user,priority:1"`
}

// RunTokenActivity is one LLM request and its usage. Every request sent to the LLM gateway
// has one, failed ones too (with no usage), so request counts and success rates can be
// reported from it.
type RunTokenActivity struct {
	ID         uint
	CreatedAt  time.Time `gorm:"index:idx_run_token_activity_created;index:idx_run_token_activity_user_created,priority:2;index:idx_run_token_activity_api_key_created,priority:2"`
	Name       string
	UserID     string `gorm:"index:idx_run_token_activity_user_created,priority:1"`
	Model      string
	APIKeyID   *uint  `gorm:"index:idx_run_token_activity_api_key_created,priority:1"`
	APIKeyName string `gorm:"type:text"`
	// Provider is the provider that served the request.
	Provider string `gorm:"type:text"`
	// Outcome is success, error or canceled, as for LLM audit logs. It is empty on records
	// made before outcomes were recorded, which were all successful requests.
	Outcome    string `gorm:"type:text"`
	DurationMs int64
	// Fallback is true when a fallback served the request.
	Fallback bool `gorm:"not null;default:false"`

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

func ConvertAPIActivity(a APIActivity) types2.APIActivity {
	return types2.APIActivity{
		UserID: a.UserID,
		Date:   *types2.NewTime(a.Date),
	}
}

func ConvertTokenActivity(a RunTokenActivity) types2.TokenUsage {
	return types2.TokenUsage{
		UserID:           a.UserID,
		Model:            a.Model,
		APIKeyID:         a.APIKeyID,
		APIKeyName:       a.APIKeyName,
		Provider:         a.Provider,
		Outcome:          a.Outcome,
		DurationMs:       a.DurationMs,
		Fallback:         a.Fallback,
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
