package types

import (
	"time"
)

const (
	// Subject types of MCP guardrail filters.
	LLMGatewayBudgetSubjectUser     = "user"
	LLMGatewayBudgetSubjectGroup    = "group"
	LLMGatewayBudgetSubjectEveryone = "everyone"

	// LLM gateway budget limit metrics.
	LLMGatewayBudgetMetricSpend        = "spend"
	LLMGatewayBudgetMetricTokens       = "tokens"
	LLMGatewayBudgetMetricInputTokens  = "input_tokens"
	LLMGatewayBudgetMetricOutputTokens = "output_tokens"
	LLMGatewayBudgetMetricRequests     = "requests"
)

// LLMGatewayBudget limits LLM gateway usage for the users its profile applies to. Obot enforces
// budgets itself from usage it records in its own database, so limits hold exactly across
// every Obot replica.
type LLMGatewayBudget struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	CreatedAt   time.Time `json:"created"`
	UpdatedAt   time.Time `json:"updated"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	Enabled     bool      `gorm:"not null" json:"enabled"`
	// ProfileID is the gateway profile (a model access policy) the budget belongs to. It
	// applies to everyone the profile applies to.
	ProfileID string `gorm:"type:text;index" json:"profileID"`
	// PerUser gives every user of the profile their own allowance. Otherwise the limits are
	// one pool shared by all of them.
	PerUser bool `gorm:"not null" json:"perUser"`
	// ExemptAdmins leaves administrators unlimited by this budget.
	ExemptAdmins bool `gorm:"not null" json:"exemptAdmins"`
	// UserTokenLimits lets a user's personal daily token limits (set on the user) replace
	// this per-user budget's input and output token limits: a positive personal limit
	// replaces the amount, and a negative one removes the limit. This is how the daily
	// token quota treated personal limits, and the budget migrated from it keeps that.
	UserTokenLimits bool                    `gorm:"not null;default:false" json:"userTokenLimits"`
	Limits          []LLMGatewayBudgetLimit `gorm:"serializer:json" json:"limits"`
	// Models restricts the budget to these Model IDs. Empty means every model.
	Models []string `gorm:"serializer:json" json:"models,omitempty"`
}

// LLMGatewayBudgetLimit caps one metric over a window.
type LLMGatewayBudgetLimit struct {
	// Metric is spend (USD), tokens, input_tokens, output_tokens or requests.
	Metric string  `json:"metric"`
	Amount float64 `json:"amount"`
	// Window is a count and unit: m (minutes), h, d, w or M (months), e.g. "1d" or "6h".
	Window string `json:"window"`
	// Calendar resets the window at calendar boundaries (the start of the hour, day, week
	// or month, in UTC) instead of rolling. Only single-unit windows can be calendar aligned.
	Calendar bool `json:"calendar,omitempty"`
}

// LLMGatewayBudgetUsage is one request's usage counted against one budget.
type LLMGatewayBudgetUsage struct {
	ID           uint      `gorm:"primaryKey"`
	BudgetID     string    `gorm:"type:text;not null;index:idx_llm_gateway_budget_usage_budget_created,priority:1;index:idx_llm_gateway_budget_usage_budget_user_created,priority:1"`
	UserID       string    `gorm:"type:text;not null;index:idx_llm_gateway_budget_usage_budget_user_created,priority:2"`
	CreatedAt    time.Time `gorm:"index;index:idx_llm_gateway_budget_usage_budget_created,priority:2;index:idx_llm_gateway_budget_usage_budget_user_created,priority:3"`
	Model        string    `gorm:"type:text"`
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Spend        float64
}

// LLMGatewaySettings are the LLM gateway settings Obot enforces itself, as opposed to the
// engine's own configuration.
type LLMGatewaySettings struct {
	// SubscriptionPassthrough lets Claude Code users signed in with a Claude subscription use
	// it through the gateway: their Anthropic OAuth token is forwarded to Anthropic for Claude
	// models instead of a configured provider key.
	SubscriptionPassthrough bool `json:"subscriptionPassthrough"`
}

func (LLMGatewayBudget) TableName() string {
	return "llm_gateway_budgets"
}

func (LLMGatewayBudgetUsage) TableName() string {
	return "llm_gateway_budget_usage"
}
