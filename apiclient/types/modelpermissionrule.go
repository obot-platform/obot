package types

import "fmt"

type ModelPermissionRule struct {
	Metadata                    `json:",inline"`
	ModelPermissionRuleManifest `json:",inline"`
}

type ModelPermissionRuleManifest struct {
	DisplayName string          `json:"displayName,omitempty"`
	Subjects    []Subject       `json:"subjects,omitempty"`
	Models      []ModelResource `json:"models,omitempty"`
}

func (m ModelPermissionRuleManifest) Validate() error {
	for _, subject := range m.Subjects {
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("invalid subject: %v", err)
		}
	}
	for _, model := range m.Models {
		if err := model.Validate(); err != nil {
			return fmt.Errorf("invalid model: %v", err)
		}
	}
	return nil
}

type ModelResource struct {
	ModelID string `json:"modelID"`
}

func (m ModelResource) Validate() error {
	if m.ModelID == "" {
		return fmt.Errorf("model ID is required")
	}
	return nil
}

// IsWildcard returns true if this model resource represents all models
func (m ModelResource) IsWildcard() bool {
	return m.ModelID == "*"
}

type ModelPermissionRuleList List[ModelPermissionRule]
