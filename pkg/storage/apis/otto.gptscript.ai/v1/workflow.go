package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*Workflow)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec,omitempty"`
	Status WorkflowStatus `json:"status,omitempty"`
}

func (in *Workflow) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type WorkflowSpec struct {
	Manifest types.WorkflowManifest `json:"manifest,omitempty"`
}

type WorkflowStatus struct {
	External      types.WorkflowExternalStatus `json:"external,omitempty"`
	WorkspaceName string                       `json:"workspaceName,omitempty"`
	Conditions    []metav1.Condition           `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Workflow `json:"items"`
}
