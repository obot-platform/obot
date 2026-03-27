package types

import "fmt"

type MessagePolicy struct {
	Metadata              `json:",inline"`
	MessagePolicyManifest `json:",inline"`
}

type MessagePolicyManifest struct {
	DisplayName string          `json:"displayName,omitempty"`
	Description string          `json:"description,omitempty"`
	Definition  string          `json:"definition"`
	Direction   PolicyDirection `json:"direction"`
	Subjects    []Subject       `json:"subjects,omitempty"`
}

type PolicyDirection string

const (
	PolicyDirectionUserMessage PolicyDirection = "user-message"
	PolicyDirectionLLMResponse PolicyDirection = "llm-response"
	PolicyDirectionBoth        PolicyDirection = "both"
)

func (m MessagePolicyManifest) Validate() error {
	if m.Definition == "" {
		return fmt.Errorf("definition is required")
	}

	switch m.Direction {
	case PolicyDirectionUserMessage, PolicyDirectionLLMResponse, PolicyDirectionBoth:
	default:
		return fmt.Errorf("invalid direction %q: must be one of %q, %q, %q",
			m.Direction, PolicyDirectionUserMessage, PolicyDirectionLLMResponse, PolicyDirectionBoth)
	}

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

	return nil
}

type MessagePolicyList List[MessagePolicy]
