package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ModelPermissionRule)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelPermissionRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelPermissionRuleSpec `json:"spec,omitempty"`
}

type ModelPermissionRuleSpec struct {
	Manifest types.ModelPermissionRuleManifest `json:"manifest"`
}

func (in *ModelPermissionRule) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.DisplayName"},
		{"Subjects", "{{len .Spec.Manifest.Subjects}}"},
		{"Models", "{{len .Spec.Manifest.Models}}"},
	}
}

func (in *ModelPermissionRule) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ModelPermissionRule) Get(field string) (value string) {
	return ""
}

func (in *ModelPermissionRule) FieldNames() []string {
	return []string{}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelPermissionRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelPermissionRule `json:"items"`
}
