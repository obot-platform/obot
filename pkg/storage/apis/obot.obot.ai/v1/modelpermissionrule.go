package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ModelAccessPolicy)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelAccessPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelAccessPolicySpec `json:"spec,omitempty"`
}

type ModelAccessPolicySpec struct {
	Manifest types.ModelAccessPolicyManifest `json:"manifest"`
}

func (in *ModelAccessPolicy) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.DisplayName"},
		{"Subjects", "{{len .Spec.Manifest.Subjects}}"},
		{"Models", "{{len .Spec.Manifest.Models}}"},
	}
}

func (in *ModelAccessPolicy) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ModelAccessPolicy) Get(_ string) (value string) {
	return ""
}

func (in *ModelAccessPolicy) FieldNames() []string {
	return []string{}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelAccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelAccessPolicy `json:"items"`
}
