package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProviderSync broadcasts provider changes to every replica.
type ProviderSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec ProviderSyncSpec `json:"spec"`
}

type ProviderSyncSpec struct {
	Revisions map[string]ProviderRevision `json:"revisions,omitempty"`
}

// ProviderRevision identifies a provider whose configuration is out of date and
// must be reconfigured.
type ProviderRevision struct {
	ProviderType      ProviderType `json:"providerType"`
	ProviderNamespace string       `json:"providerNamespace"`
	ProviderName      string       `json:"providerName"`
	Revision          int64        `json:"revision"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProviderSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ProviderSync `json:"items"`
}
