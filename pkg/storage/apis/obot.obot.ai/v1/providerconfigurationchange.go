package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ProviderTypeAuth                 ProviderType         = "auth"
	ProviderTypeModel                ProviderType         = "model"
	ProviderTypeLicense              ProviderType         = "license"
	ProviderDesiredStateConfigured   ProviderDesiredState = "configured"
	ProviderDesiredStateDeconfigured ProviderDesiredState = "deconfigured"
)

type ProviderType string

type ProviderDesiredState string

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProviderConfigurationChange is an internal desired-state task for changing a
// provider's active credential and all state derived from it.
type ProviderConfigurationChange struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   ProviderConfigurationChangeSpec   `json:"spec"`
	Status ProviderConfigurationChangeStatus `json:"status"`
}

type ProviderConfigurationChangeSpec struct {
	ProviderType         ProviderType         `json:"providerType"`
	ProviderName         string               `json:"providerName"`
	DesiredState         ProviderDesiredState `json:"desiredState"`
	StagedCredentialName string               `json:"stagedCredentialName,omitempty"`
}

type ProviderConfigurationChangeStatus struct {
	// Applied means all externally visible provider state has committed. The
	// remaining reconciliation only removes the staged credential and this task.
	Applied bool `json:"applied,omitempty"`
	// Error describes a terminal rejection. The remaining reconciliation only
	// removes the staged credential and this task.
	Error string `json:"error,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProviderConfigurationChangeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ProviderConfigurationChange `json:"items"`
}
