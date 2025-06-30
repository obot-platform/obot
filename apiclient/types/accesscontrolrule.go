package types

import (
	"fmt"
)

type AccessControlRule struct {
	Metadata                  `json:",inline"`
	AccessControlRuleManifest `json:",inline"`
}

type AccessControlRuleManifest struct {
	DisplayName string     `json:"displayName,omitempty"`
	Subjects    []Subject  `json:"subjects,omitempty"`
	Resources   []Resource `json:"resources,omitempty"`
}

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

type Subject struct {
	Type SubjectType `json:"type"`
	ID   string      `json:"id"`
}

type SubjectType string

const (
	SubjectTypeGroup    SubjectType = "group"
	SubjectTypeUser     SubjectType = "user"
	SubjectTypeSelector SubjectType = "selector"
)

func (s Subject) Validate() error {
	switch s.Type {
	case SubjectTypeUser, SubjectTypeGroup:
		if s.ID == "" {
			return fmt.Errorf("ID is required")
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

type Resource struct {
	Type ResourceType `json:"type"`
	ID   string       `json:"id"`
}

func (r Resource) Validate() error {
	switch r.Type {
	case ResourceTypeMCPServerCatalogEntry, ResourceTypeMCPServer:
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

type ResourceType string

const (
	ResourceTypeMCPServerCatalogEntry ResourceType = "mcpServerCatalogEntry"
	ResourceTypeMCPServer             ResourceType = "mcpServer"
	ResourceTypeSelector              ResourceType = "selector"
)

type AccessControlRuleList List[AccessControlRule]
