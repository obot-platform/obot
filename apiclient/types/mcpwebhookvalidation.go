package types

import (
	"fmt"
)

type MCPWebhookValidation struct {
	Metadata                     `json:",inline"`
	MCPWebhookValidationManifest `json:",inline"`
	HasSecret                    bool     `json:"hasSecret,omitempty"`
	Configured                   bool     `json:"configured"`
	MissingRequiredEnvVars       []string `json:"missingRequiredEnvVars,omitempty"`
}

type MCPWebhookValidationManifest struct {
	Name                          string                   `json:"name,omitempty"`
	Resources                     []Resource               `json:"resources,omitempty"`
	URL                           string                   `json:"url,omitempty"`
	Secret                        string                   `json:"secret,omitempty"`
	SystemMCPServerManifest       *SystemMCPServerManifest `json:"mcpServerManifest,omitempty"`
	SystemMCPServerCatalogEntryID string                   `json:"systemMCPServerCatalogEntryID,omitempty"`
	ToolName                      string                   `json:"toolName,omitempty"`
	Selectors                     MCPSelectors             `json:"selectors,omitempty"`
	AllowedToMutate               bool                     `json:"allowedToMutate,omitempty"`
	Disabled                      bool                     `json:"disabled,omitempty"`
	// Guardrail makes this a guardrail filter: Obot checks matching traffic in-process with
	// guardrails instead of calling a filter MCP server. It is mutually exclusive
	// with URL, SystemMCPServerManifest and SystemMCPServerCatalogEntryID.
	Guardrail *MCPGuardrailFilterConfig `json:"guardrail,omitempty"`
}

const (
	// Guardrail filter actions.
	MCPGuardrailActionBlock  = "block"
	MCPGuardrailActionRedact = "redact"

	// Guardrail filter directions.
	MCPGuardrailDirectionRequest  = "request"
	MCPGuardrailDirectionResponse = "response"
	MCPGuardrailDirectionBoth     = "both"

	// Guardrail filter subject types.
	MCPGuardrailSubjectUser     = "user"
	MCPGuardrailSubjectGroup    = "group"
	MCPGuardrailSubjectEveryone = "everyone"
)

// MCPGuardrailFilterConfig configures a guardrail filter.
type MCPGuardrailFilterConfig struct {
	// GuardrailIDs are the guardrails that check the traffic.
	GuardrailIDs []string `json:"guardrailIDs"`
	// Action is block or redact.
	Action string `json:"action"`
	// Direction is request, response or both. It defaults to both.
	Direction string `json:"direction,omitempty"`
	// FailClosed blocks the message when a guardrail can't be evaluated.
	FailClosed bool `json:"failClosed,omitempty"`
	// SamplingRate is the percentage (1-100) of matching messages that are checked. It
	// defaults to 100.
	SamplingRate int `json:"samplingRate,omitempty"`
	// Subjects limits the filter to users and groups. Empty means everyone.
	Subjects []MCPGuardrailFilterSubject `json:"subjects,omitempty"`
	// Condition is an optional CEL expression that must be true for the filter to apply.
	Condition string `json:"condition,omitempty"`
	// BlockMessage replaces the default message returned when the filter blocks.
	BlockMessage string `json:"blockMessage,omitempty"`
}

// MCPGuardrailFilterSubject is a user, a group or everyone.
type MCPGuardrailFilterSubject struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

type MCPWebhookValidationList List[MCPWebhookValidation]

type MCPSelectors []MCPSelector

type MCPSelector struct {
	Method      string   `json:"method,omitempty"`
	Identifiers []string `json:"identifiers,omitempty"`
}

func (f MCPSelectors) Matches(method, identifier string) bool {
	for _, filter := range f {
		if filter.Matches(method, identifier) {
			return true
		}
	}

	// Empty filter means everything.
	return f == nil
}

func (f MCPSelectors) Strings() []string {
	if len(f) == 0 {
		return []string{"*"}
	}

	var result []string
	for _, filter := range f {
		result = append(result, filter.Strings()...)
	}
	return result
}

func (f *MCPSelector) Matches(method, identifier string) bool {
	if f.Method != "*" && f.Method != method {
		return false
	}

	for _, id := range f.Identifiers {
		if id == "*" || identifier == "" || id == identifier {
			return true
		}
	}

	// Empty identifiers means everything.
	return f.Identifiers == nil
}

func (f MCPSelector) Strings() []string {
	s := "*"
	if f.Method != "" {
		s = f.Method
	}

	if f.Identifiers == nil {
		return []string{s}
	}

	result := make([]string, 0, len(f.Identifiers))
	for _, id := range f.Identifiers {
		result = append(result, fmt.Sprintf("%s?name=%s", s, id))
	}

	return result
}
