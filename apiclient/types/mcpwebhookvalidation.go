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
}

type MCPWebhookValidationList List[MCPWebhookValidation]

type MCPSelectors []MCPSelector

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

type MCPSelector struct {
	Method      string   `json:"method,omitempty"`
	Identifiers []string `json:"identifiers,omitempty"`
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
