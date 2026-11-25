package types

import (
	"fmt"
)

const (
	SkillResourceTypeSkill           SkillResourceType = "skill"
	SkillResourceTypeSkillRepository SkillResourceType = "skillRepository"
	SkillResourceTypeSelector        SkillResourceType = "selector"
)

type SkillAccessRule struct {
	Metadata
	SkillAccessRuleManifest `json:",inline"`
}

type SkillAccessRuleManifest struct {
	DisplayName string          `json:"displayName,omitempty"`
	Subjects    []Subject       `json:"subjects,omitempty"`
	Resources   []SkillResource `json:"resources,omitempty"`
}

type SkillResource struct {
	Type SkillResourceType `json:"type"`
	ID   string            `json:"id"`
}

type SkillResourceType string

type SkillAccessRuleList List[SkillAccessRule]

func (s SkillAccessRuleManifest) Validate() error {
	if err := validatePolicySubjects(PolicySystemSkillAccess, s.Subjects); err != nil {
		return err
	}

	if len(s.Resources) == 0 {
		return fmt.Errorf("at least one resource is required")
	}

	resources := make(map[SkillResource]struct{}, len(s.Resources))
	for _, resource := range s.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("invalid resource: %w", err)
		}

		if resource.IsWildcard() && len(s.Resources) > 1 {
			return fmt.Errorf("wildcard resource (*) must be the only resource")
		}

		if _, ok := resources[resource]; ok {
			return fmt.Errorf("duplicate resource: %s/%s", resource.Type, resource.ID)
		}
		resources[resource] = struct{}{}
	}

	return nil
}

func (s SkillResource) Validate() error {
	switch s.Type {
	case SkillResourceTypeSkill, SkillResourceTypeSkillRepository:
		if s.ID == "" {
			return fmt.Errorf("resource ID is required")
		}
		return nil
	case SkillResourceTypeSelector:
		if s.ID != "*" {
			return fmt.Errorf("selector resource ID must be '*'")
		}
		return nil
	}

	return fmt.Errorf("invalid resource type: %s", s.Type)
}

func (s SkillResource) IsWildcard() bool {
	return s.Type == SkillResourceTypeSelector && s.ID == "*"
}
