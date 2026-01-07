package types

import (
	"fmt"
	"strings"
)

const DefaultModelAliasRefPrefix = "obot://"

type ModelAccessPolicy struct {
	Metadata                  `json:",inline"`
	ModelAccessPolicyManifest `json:",inline"`
}

type ModelAccessPolicyManifest struct {
	DisplayName string          `json:"displayName,omitempty"`
	Subjects    []Subject       `json:"subjects,omitempty"`
	Models      []ModelResource `json:"models,omitempty"`
}

func (m ModelAccessPolicyManifest) Validate() error {
	if len(m.Subjects) == 0 {
		return fmt.Errorf("at least one subject is required")
	}

	subjects := make(map[Subject]struct{}, len(m.Subjects))
	for _, subject := range m.Subjects {
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("invalid subject: %w", err)
		}

		if subject.ID == "*" && len(m.Subjects) > 1 {
			return fmt.Errorf("wildcard subject (*) must be the only subject")
		}

		if _, ok := subjects[subject]; ok {
			return fmt.Errorf("duplicate subject: %s/%s", subject.Type, subject.ID)
		}
		subjects[subject] = struct{}{}
	}

	if len(m.Models) == 0 {
		return fmt.Errorf("at least one model resource is required")
	}

	models := make(map[ModelResource]struct{}, len(m.Models))
	for _, model := range m.Models {
		if err := model.Validate(); err != nil {
			return fmt.Errorf("invalid model: %w", err)
		}

		if model.IsWildcard() && len(m.Models) > 1 {
			return fmt.Errorf("wildcard model (*) must be the only model")
		}

		if _, ok := models[model]; ok {
			return fmt.Errorf("duplicate model %s", model.ID)
		}
		models[model] = struct{}{}
	}

	return nil
}

type ModelResource struct {
	// ID is the unique identifier of the model resource.
	// It either be:
	// - the wildcard '*', which selects all available models
	// - the model ID of a specific model
	// - an Obot default model alias in the form "obot://<alias>"
	//
	// When a model ID is provided, it must match the ID field of an existing referenced model.
	ID string `json:"id"`
}

func (m ModelResource) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("model resource ID is required")
	}
	if alias, isAlias := m.IsDefaultModelAliasRef(); isAlias {
		if DefaultModelAliasTypeFromString(alias) == DefaultModelAliasTypeUnknown {
			return fmt.Errorf("unknown model alias type reference: %s", alias)
		}
	}
	return nil
}

// IsWildcard returns true if this model resource selects all models
func (m ModelResource) IsWildcard() bool {
	return m.ID == "*"
}

// IsModelAlias returns true if the given model references a DefaultModelAlias.
func (m ModelResource) IsDefaultModelAliasRef() (string, bool) {
	return strings.CutPrefix(m.ID, DefaultModelAliasRefPrefix)
}

type ModelAccessPolicyList List[ModelAccessPolicy]
