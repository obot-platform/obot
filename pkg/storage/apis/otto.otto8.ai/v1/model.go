package v1

import (
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ModelSpec   `json:"spec,omitempty"`
	Status            ModelStatus `json:"status,omitempty"`
}

func (m *Model) IsAssigned() bool {
	return m.Status.AliasAssigned
}

func (m *Model) GetAliasName() string {
	return m.Spec.Manifest.Alias
}

func (m *Model) SetAssigned(assigned bool) {
	m.Status.AliasAssigned = assigned
}

type ModelSpec struct {
	Manifest types.ModelManifest `json:"manifest,omitempty"`
}

type ModelStatus struct {
	AliasAssigned bool `json:"aliasAssigned,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}
