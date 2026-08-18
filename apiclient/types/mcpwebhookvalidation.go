package types

import (
	"fmt"
	"slices"
)

type MCPWebhookValidation struct {
	Metadata                     `json:",inline"`
	MCPWebhookValidationManifest `json:",inline"`
	HasSecret                    bool     `json:"hasSecret,omitempty"`
	Configured                   bool     `json:"configured"`
	MissingRequiredEnvVars       []string `json:"missingRequiredEnvVars,omitempty"`
}

type MCPWebhookValidationManifest struct {
	Name                          string                         `json:"name,omitempty"`
	Resources                     []MCPWebhookValidationResource `json:"resources,omitempty"`
	URL                           string                         `json:"url,omitempty"`
	Secret                        string                         `json:"secret,omitempty"`
	SystemMCPServerManifest       *SystemMCPServerManifest       `json:"mcpServerManifest,omitempty"`
	SystemMCPServerCatalogEntryID string                         `json:"systemMCPServerCatalogEntryID,omitempty"`
	ToolName                      string                         `json:"toolName,omitempty"`
	Selectors                     MCPSelectors                   `json:"selectors,omitempty"`
	LocalAgentEvents              LocalAgentEvents               `json:"localAgentEvents,omitempty"`
	AllowedToMutate               bool                           `json:"allowedToMutate,omitempty"`
	Disabled                      bool                           `json:"disabled,omitempty"`
}

type MCPWebhookValidationList List[MCPWebhookValidation]

type MCPWebhookValidationResource struct {
	Type MCPWebhookValidationResourceType `json:"type"`
	ID   string                           `json:"id"`
}

func (r MCPWebhookValidationResource) Validate() error {
	switch r.Type {
	case MCPWebhookValidationResourceTypeMCPServerCatalogEntry,
		MCPWebhookValidationResourceTypeMCPServer,
		MCPWebhookValidationResourceTypeMCPCatalog:
		if r.ID == "" {
			return fmt.Errorf("resource ID is required")
		}
		return nil
	case MCPWebhookValidationResourceTypeSelector,
		MCPWebhookValidationResourceTypeDeviceSelector:
		if r.ID != "*" {
			return fmt.Errorf("%s resource ID must be '*'", r.Type)
		}
		return nil
	}

	return fmt.Errorf("invalid resource type: %s", r.Type)
}

type MCPWebhookValidationResourceType string

const (
	MCPWebhookValidationResourceTypeMCPServerCatalogEntry MCPWebhookValidationResourceType = "mcpServerCatalogEntry"
	MCPWebhookValidationResourceTypeMCPServer             MCPWebhookValidationResourceType = "mcpServer"
	MCPWebhookValidationResourceTypeMCPCatalog            MCPWebhookValidationResourceType = "mcpCatalog"
	MCPWebhookValidationResourceTypeSelector              MCPWebhookValidationResourceType = "selector"
	MCPWebhookValidationResourceTypeDeviceSelector        MCPWebhookValidationResourceType = "deviceSelector"
)

type LocalAgentEvent string

const (
	LocalAgentEventAll               LocalAgentEvent = "*"
	LocalAgentEventUserPrompt        LocalAgentEvent = "userPrompt"
	LocalAgentEventToolCallArguments LocalAgentEvent = "toolCallArguments"
	LocalAgentEventToolResponse      LocalAgentEvent = "toolResponse"
)

type LocalAgentEvents []LocalAgentEvent

func (m MCPWebhookValidationManifest) ValidateLocalAgentTargeting() error {
	hasDeviceTarget := false
	for _, resource := range m.Resources {
		if resource.Type == MCPWebhookValidationResourceTypeDeviceSelector {
			hasDeviceTarget = true
			break
		}
	}

	if hasDeviceTarget && len(m.LocalAgentEvents) == 0 {
		return fmt.Errorf("local agent events are required for a device selector resource")
	}
	if !hasDeviceTarget && len(m.LocalAgentEvents) > 0 {
		return fmt.Errorf("a device selector resource is required for local agent events")
	}

	for _, event := range m.LocalAgentEvents {
		switch event {
		case LocalAgentEventAll:
			if len(m.LocalAgentEvents) != 1 {
				return fmt.Errorf("local agent event '*' cannot be combined with explicit events")
			}
		case LocalAgentEventUserPrompt, LocalAgentEventToolCallArguments, LocalAgentEventToolResponse:
		default:
			return fmt.Errorf("invalid local agent event: %s", event)
		}
	}

	return nil
}

func (e LocalAgentEvents) Matches(event LocalAgentEvent) bool {
	return slices.Contains(e, LocalAgentEventAll) || slices.Contains(e, event)
}

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
