//go:build ignore

// TODO: MAKE SURE YOU ADD NEW TYPES TO scheme.go

package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*Template)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Template struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TemplateSpec   `json:"spec,omitempty"`
	Status TemplateStatus `json:"status,omitempty"`
}

func (in *Template) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type TemplateSpec struct {
	Description string `json:"description,omitempty"`
}

type TemplateStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type TemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Template `json:"items"`
}
