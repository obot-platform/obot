package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProviderDaemonSync broadcasts provider credential changes to every replica.
type ProviderDaemonSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec ProviderDaemonSyncSpec `json:"spec"`
}

type ProviderDaemonSyncSpec struct {
	Revisions map[string]ProviderDaemonRevision `json:"revisions,omitempty"`
}

// ProviderDaemonRevision identifies a provider whose cached daemons must be
// restarted whenever Revision advances.
type ProviderDaemonRevision struct {
	ProviderType      ProviderType `json:"providerType"`
	ProviderNamespace string       `json:"providerNamespace"`
	ProviderName      string       `json:"providerName"`
	Revision          int64        `json:"revision"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProviderDaemonSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ProviderDaemonSync `json:"items"`
}
