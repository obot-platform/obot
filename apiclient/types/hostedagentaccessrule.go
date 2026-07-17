package types

import "fmt"

type HostedAgentAccessRule struct {
	Metadata                      `json:",inline"`
	HostedAgentAccessRuleManifest `json:",inline"`
}

type HostedAgentAccessRuleManifest struct {
	DisplayName string                `json:"displayName,omitempty"`
	Subjects    []Subject             `json:"subjects,omitempty"`
	Resources   []HostedAgentResource `json:"resources,omitempty"`
}

func (h HostedAgentAccessRuleManifest) Validate() error {
	if len(h.Subjects) == 0 {
		return fmt.Errorf("at least one subject is required")
	}

	subjects := make(map[Subject]struct{}, len(h.Subjects))
	for _, subject := range h.Subjects {
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("invalid subject: %w", err)
		}

		if subject.ID == "*" && len(h.Subjects) > 1 {
			return fmt.Errorf("wildcard subject (*) must be the only subject")
		}

		if _, ok := subjects[subject]; ok {
			return fmt.Errorf("duplicate subject: %s/%s", subject.Type, subject.ID)
		}
		subjects[subject] = struct{}{}
	}

	if len(h.Resources) == 0 {
		return fmt.Errorf("at least one resource is required")
	}

	resources := make(map[HostedAgentResource]struct{}, len(h.Resources))
	for _, resource := range h.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("invalid resource: %w", err)
		}

		if resource.IsWildcard() && len(h.Resources) > 1 {
			return fmt.Errorf("wildcard resource (*) must be the only resource")
		}

		if _, ok := resources[resource]; ok {
			return fmt.Errorf("duplicate resource: %s/%s", resource.Type, resource.ID)
		}
		resources[resource] = struct{}{}
	}

	return nil
}

type HostedAgentResource struct {
	Type HostedAgentResourceType `json:"type"`
	ID   string                  `json:"id"`
}

func (h HostedAgentResource) Validate() error {
	switch h.Type {
	case HostedAgentResourceTypeHostedAgent:
		if h.ID == "" {
			return fmt.Errorf("resource ID is required")
		}
		return nil
	case HostedAgentResourceTypeSelector:
		if h.ID != "*" {
			return fmt.Errorf("selector resource ID must be '*'")
		}
		return nil
	}

	return fmt.Errorf("invalid resource type: %s", h.Type)
}

func (h HostedAgentResource) IsWildcard() bool {
	return h.Type == HostedAgentResourceTypeSelector && h.ID == "*"
}

type HostedAgentResourceType string

const (
	HostedAgentResourceTypeHostedAgent HostedAgentResourceType = "hostedAgent"
	HostedAgentResourceTypeSelector    HostedAgentResourceType = "selector"
)

type HostedAgentAccessRuleList List[HostedAgentAccessRule]
