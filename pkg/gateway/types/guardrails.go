package types

import (
	"encoding/json"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
)

const (
	// Guardrail targets.
	GuardrailTargetLLM = "llm"
	GuardrailTargetMCP = "mcp"

	// Guardrail stages a policy applies to.
	GuardrailApplyInput  = "input"
	GuardrailApplyOutput = "output"
	GuardrailApplyBoth   = "both"

	// Guardrail actions.
	GuardrailActionBlock  = "block"
	GuardrailActionRedact = "redact"
	GuardrailActionLog    = "log"

	// Guardrail input scopes.
	GuardrailScopeAll    = "all"
	GuardrailScopeLatest = "latest"
)

// GuardrailPolicy decides which model requests and responses are checked, by which
// guardrails, and what happens when a guardrail finds a problem. It applies to the users its
// gateway profile applies to. MCP guardrail filters are evaluated as policies too, in memory,
// with their own subjects.
type GuardrailPolicy struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	CreatedAt   time.Time `json:"created"`
	UpdatedAt   time.Time `json:"updated"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	Enabled     bool      `gorm:"not null" json:"enabled"`
	// Target is llm (model requests and responses) or mcp (tool call arguments and results).
	Target string `gorm:"type:text;not null" json:"target"`
	// ApplyTo is input, output or both.
	ApplyTo string `gorm:"type:text;not null" json:"applyTo"`
	// Action is block, redact or log.
	Action string `gorm:"type:text;not null" json:"action"`
	// Condition is an optional CEL expression that must be true for the policy to apply.
	Condition string `gorm:"type:text" json:"condition,omitempty"`
	// SamplingRate is the percentage (1-100) of matching requests the policy checks.
	SamplingRate int `gorm:"not null" json:"samplingRate"`
	// InputScope is all (every message in the request) or latest (only messages since the
	// model last responded). Redacting policies always check every message.
	InputScope string `gorm:"type:text" json:"inputScope,omitempty"`
	// FailClosed blocks the request when a guardrail can't be evaluated.
	FailClosed bool `gorm:"not null" json:"failClosed"`
	// BlockMessage replaces the default message returned when the policy blocks.
	BlockMessage string   `gorm:"type:text" json:"blockMessage,omitempty"`
	GuardrailIDs []string `gorm:"serializer:json" json:"guardrailIDs"`
	// ProfileID is the gateway profile (a model access policy) the policy belongs to.
	ProfileID string `gorm:"type:text;index" json:"profileID"`
	// Subjects limit an MCP guardrail filter's policy to users, agents and groups. They aren't stored:
	// stored policies apply through their profile.
	Subjects []apitypes.Subject `gorm:"-" json:"-"`
}

// Guardrail configures one checker. Sensitive settings are kept in the encrypted
// credential store, not here.
type Guardrail struct {
	ID          string          `gorm:"primaryKey;type:text" json:"id"`
	CreatedAt   time.Time       `json:"created"`
	UpdatedAt   time.Time       `json:"updated"`
	Name        string          `gorm:"type:text;not null" json:"name"`
	Description string          `gorm:"type:text" json:"description,omitempty"`
	Type        string          `gorm:"type:text;not null" json:"type"`
	Config      json.RawMessage `json:"config,omitempty"`
}

func (GuardrailPolicy) TableName() string {
	return "guardrail_policies"
}

func (Guardrail) TableName() string {
	return "guardrails"
}
