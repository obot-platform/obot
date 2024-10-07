package types

import (
	"errors"
	"fmt"
	"time"
)

type Model struct {
	ID                string    `json:"id" gorm:"unique"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	LLMProviderID     uint      `json:"llmProviderID" gorm:"foreignKey:id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ProviderModelName string    `json:"providerModelName"`
	Disabled          bool      `json:"disabled"`
}

func (m *Model) Validate() error {
	var errs []error
	if m.ID == "" {
		errs = append(errs, fmt.Errorf("field ID is required"))
	}
	if m.LLMProviderID < 1 {
		errs = append(errs, fmt.Errorf("%d is an invalid LLM Provider id", m.LLMProviderID))
	}
	if m.ProviderModelName == "" {
		errs = append(errs, fmt.Errorf("field ProviderModelName is required"))
	}

	return errors.Join(errs...)
}
