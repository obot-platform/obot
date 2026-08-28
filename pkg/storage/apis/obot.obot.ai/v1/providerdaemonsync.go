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
	Timestamp metav1.Time `json:"timestamp"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProviderDaemonSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ProviderDaemonSync `json:"items"`
}
