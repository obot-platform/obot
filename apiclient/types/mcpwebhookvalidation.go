package types

import (
	"fmt"
	"net/url"
	"slices"
)

type MCPWebhookValidation struct {
	Metadata                     `json:",inline"`
	MCPWebhookValidationManifest `json:",inline"`
}

type MCPWebhookValidationManifest struct {
	DisplayName string       `json:"displayName,omitempty"`
	Resources   []Resource   `json:"resources,omitempty"`
	Webhooks    []MCPWebhook `json:"webhooks,omitempty"`
	Disabled    bool         `json:"disabled,omitempty"`
}

func (m *MCPWebhookValidationManifest) Validate() error {
	for _, resource := range m.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("invalid resource: %v", err)
		}
	}

	for _, webhook := range m.Webhooks {
		if err := webhook.Validate(); err != nil {
			return fmt.Errorf("invalid webhook: %v", err)
		}
	}
	return nil
}

type MCPWebhook struct {
	URL     string     `json:"url"`
	Secret  string     `json:"secret,omitempty"`
	Filters MCPFilters `json:"filters,omitempty"`
}

func (w *MCPWebhook) Validate() error {
	if w.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	if _, err := url.Parse(w.URL); err != nil {
		return fmt.Errorf("invalid webhook URL: %v", err)
	}

	for _, filter := range w.Filters {
		if filter.Method == "*" {
			w.Filters = []MCPFilter{{Method: filter.Method}}
			break
		}
		if slices.Contains(filter.Identifiers, "*") {
			filter.Identifiers = []string{"*"}
		}
	}

	// Default to all filters.
	if len(w.Filters) == 0 {
		w.Filters = []MCPFilter{{Method: "*"}}
	}

	return nil
}

type MCPWebhookValidationList List[MCPWebhookValidation]

type MCPFilters []MCPFilter

func (f MCPFilters) Matches(method, identifier string) bool {
	for _, filter := range f {
		if filter.Matches(method, identifier) {
			return true
		}
	}

	// Empty filter means everything.
	return f == nil
}

type MCPFilter struct {
	Method      string   `json:"method,omitempty"`
	Identifiers []string `json:"identifiers,omitempty"`
}

func (f *MCPFilter) Matches(method, identifier string) bool {
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
