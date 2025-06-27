package types

import "fmt"

type AccessControlRule struct {
	Metadata                  `json:",inline"`
	AccessControlRuleManifest `json:",inline"`
}

type AccessControlRuleManifest struct {
	DisplayName string     `json:"displayName,omitempty"`
	UserIDs     []string   `json:"userIDs,omitempty"`
	Resources   []Resource `json:"resources,omitempty"`
}

type Resource struct {
	Type ResourceType `json:"type"`
	ID   string       `json:"id"`
}

func (r Resource) Validate() error {
	switch r.Type {
	case ResourceTypeMCPServerCatalogEntry, ResourceTypeMCPServer:
		return nil
	case ResourceTypeSelector:
		if r.ID != "*" {
			// We will change this in the future when we support selectors.
			return fmt.Errorf("selector resource ID must be '*'")
		}
		return nil
	default:
		return fmt.Errorf("invalid resource type: %s", r.Type)
	}
}

type ResourceType string

const (
	ResourceTypeMCPServerCatalogEntry ResourceType = "mcpServerCatalogEntry"
	ResourceTypeMCPServer             ResourceType = "mcpServer"
	ResourceTypeSelector              ResourceType = "selector"
)

type AccessControlRuleList List[AccessControlRule]
