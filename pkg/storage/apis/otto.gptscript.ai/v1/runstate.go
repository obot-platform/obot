package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*RunState)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunStateSpec   `json:"spec,omitempty"`
	Status RunStateStatus `json:"status,omitempty"`
}

func (in *RunState) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type RunStateSpec struct {
	ThreadName string `json:"threadName,omitempty"`
	Program    []byte `json:"program,omitempty"`
	ChatState  []byte `json:"chatState,omitempty"`
	CallFrame  []byte `json:"callFrame,omitempty"`
	Output     []byte `json:"output,omitempty"`
	Done       bool   `json:"done,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (in *RunState) DeleteRefs() []Ref {
	return []Ref{}
}

type RunStateStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []RunState `json:"items"`
}
