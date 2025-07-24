package types

import (
	"fmt"
	"slices"
)

type MCPWebhookValidation struct {
	Metadata                     `json:",inline"`
	MCPWebhookValidationManifest `json:",inline"`
	HasSecret                    bool `json:"hasSecret,omitempty"`
}

type MCPWebhookValidationManifest struct {
	Name      string       `json:"name,omitempty"`
	Resources []Resource   `json:"resources,omitempty"`
	URL       string       `json:"url,omitempty"`
	Secret    string       `json:"secret,omitempty"`
	Selectors MCPSelectors `json:"selectors,omitempty"`
	Disabled  bool         `json:"disabled,omitempty"`
}

func (m *MCPWebhookValidationManifest) Validate() error {
	if m.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	for _, resource := range m.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("invalid resource: %v", err)
		}
	}

	for _, filter := range m.Selectors {
		if filter.Method == "*" {
			m.Selectors = []MCPSelector{{Method: filter.Method}}
			break
		}
		if slices.Contains(filter.Identifiers, "*") {
			filter.Identifiers = []string{"*"}
		}
	}

	return nil
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
