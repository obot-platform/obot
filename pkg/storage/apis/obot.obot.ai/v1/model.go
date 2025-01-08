package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*Model)(nil)
	_ Aliasable     = (*Model)(nil)
	_ Generationed  = (*Model)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ModelSpec   `json:"spec,omitempty"`
	Status            ModelStatus `json:"status,omitempty"`
}

func (m *Model) Has(field string) (exists bool) {
	return m.Get(field) != ""
}

func (m *Model) Get(field string) (value string) {
	if m != nil {
		switch field {
		case "spec.manifest.modelProvider":
			return m.Spec.Manifest.ModelProvider
		}
	}

	return ""
}

func (m *Model) FieldNames() []string {
	return []string{"spec.manifest.modelProvider"}
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

func (m *Model) GetObservedGeneration() int64 {
	return m.Status.ObservedGeneration
}

func (m *Model) SetObservedGeneration(gen int64) {
	m.Status.ObservedGeneration = gen
}

type ModelSpec struct {
	Manifest types.ModelManifest `json:"manifest,omitempty"`
}

type ModelStatus struct {
	AliasAssigned      bool  `json:"aliasAssigned,omitempty"`
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}
