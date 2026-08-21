package types

import (
	"fmt"
)

const (
	SubjectTypeGroup    SubjectType = "group"
	SubjectTypeUser     SubjectType = "user"
	SubjectTypeSelector SubjectType = "selector"

	ResourceTypeMCPServerCatalogEntry ResourceType = "mcpServerCatalogEntry"
	ResourceTypeMCPServer             ResourceType = "mcpServer"
	ResourceTypeMcpCatalog            ResourceType = "mcpCatalog"
	ResourceTypeSelector              ResourceType = "selector"
)

type AccessControlRule struct {
	Metadata                  `json:",inline"`
	MCPCatalogID              string `json:"mcpCatalogID"`
	PowerUserWorkspaceID      string `json:"powerUserWorkspaceID,omitempty"`
	PowerUserID               string `json:"powerUserID,omitempty"`
	Generated                 bool   `json:"generated,omitempty"`
	AccessControlRuleManifest `json:",inline"`
}

type AccessControlRuleManifest struct {
	DisplayName string     `json:"displayName,omitempty"`
	Subjects    []Subject  `json:"subjects,omitempty"`
	Resources   []Resource `json:"resources,omitempty"`
}

type Subject struct {
	Type SubjectType `json:"type"`
	ID   string      `json:"id"`
}

type SubjectType string

type Resource struct {
	Type ResourceType `json:"type"`
	ID   string       `json:"id"`
}

type ResourceType string

type AccessControlRuleList List[AccessControlRule]

func (a AccessControlRuleManifest) Validate() error {
	for _, resource := range a.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("invalid resource: %v", err)
		}
	}
	for _, subject := range a.Subjects {
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("invalid subject: %v", err)
		}
	}
	return nil
}

func (s Subject) Validate() error {
	switch s.Type {
	case SubjectTypeUser, SubjectTypeGroup:
		if s.ID == "" {
			return fmt.Errorf("user ID is required")
		}
		return nil
	case SubjectTypeSelector:
		if s.ID != "*" {
			return fmt.Errorf("selector subject ID must be '*'")
		}
		return nil
	}
	return fmt.Errorf("invalid subject type: %s", s.Type)
}

func (r Resource) Validate() error {
	switch r.Type {
	case ResourceTypeMCPServerCatalogEntry, ResourceTypeMCPServer, ResourceTypeMcpCatalog:
		if r.ID == "" {
			return fmt.Errorf("resource ID is required")
		}
		return nil
	case ResourceTypeSelector:
		if r.ID != "*" {
			// We will change this in the future when we support selectors.
			return fmt.Errorf("selector resource ID must be '*'")
		}
		return nil
	}
	return fmt.Errorf("invalid resource type: %s", r.Type)
}
