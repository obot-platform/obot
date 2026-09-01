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
	// ProviderDesiredStateSwitched replaces the configured auth provider with another in one
	// reconcile. It is separate so ProviderDesiredStateConfigured keeps its empty-slot conflict check.
	ProviderDesiredStateSwitched ProviderDesiredState = "switched"
	// ProviderDesiredStateStaged saves a replacement auth provider's settings while the configured
	// one keeps serving logins. It goes through a change so the one-staged rule is serialized.
	ProviderDesiredStateStaged ProviderDesiredState = "staged"
	// ProviderDesiredStateUnstaged discards a staged replacement, leaving the configured provider
	// untouched. It shares the switch's serialization so a discard cannot interleave with one.
	ProviderDesiredStateUnstaged ProviderDesiredState = "unstaged"
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
	// ReplacesProviderName names the auth provider a switch takes over from. It must be the
	// currently configured one, and is deconfigured in the reconcile that promotes the replacement.
	ReplacesProviderName string `json:"replacesProviderName,omitempty"`
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
