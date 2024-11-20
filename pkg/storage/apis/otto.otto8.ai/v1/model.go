package v1

import (
	"fmt"

	"github.com/otto8-ai/nah/pkg/fields"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*Model)(nil)
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

func (m *Model) Has(field string) bool {
	return m.Get(field) != ""
}

func (m *Model) Get(field string) string {
	if m != nil {
		switch field {
		case "spec.manifest.default":
			return fmt.Sprintf("%v", m.Spec.Manifest.Default)
		}
	}

	return ""
}

func (m *Model) FieldNames() []string {
	return []string{"spec.manifest.default"}
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
