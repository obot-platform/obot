package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelAccessPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelAccessPolicySpec `json:"spec,omitempty"`
	Status EmptyStatus           `json:"status,omitempty"`
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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelAccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelAccessPolicy `json:"items"`
}
