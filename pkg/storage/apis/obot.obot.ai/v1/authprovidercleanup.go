package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AuthProviderCleanup is an internal task that removes group references owned by a
// deconfigured authentication provider.
type AuthProviderCleanup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AuthProviderCleanupSpec `json:"spec"`
	Status EmptyStatus             `json:"status"`
}

type AuthProviderCleanupSpec struct {
	AuthProviderName string `json:"authProviderName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AuthProviderCleanupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []AuthProviderCleanup `json:"items"`
}
