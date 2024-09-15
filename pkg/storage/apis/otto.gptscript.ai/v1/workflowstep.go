package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*WorkflowStep)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowStep struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowStepSpec   `json:"spec,omitempty"`
	Status WorkflowStepStatus `json:"status,omitempty"`
}

func (in *WorkflowStep) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type WorkflowStepSpec struct {
	ParentWorkflowStepName string   `json:"parentWorkflowStepName,omitempty"`
	AfterWorkflowStepName  string   `json:"afterWorkflowStepName,omitempty"`
	NoWaitForAfterComplete bool     `json:"noWaitForAfterComplete,omitempty"`
	Step                   Step     `json:"step,omitempty"`
	Path                   []string `json:"path,omitempty"`
	WorkflowName           string   `json:"workflowName,omitempty"`
	WorkflowExecutionName  string   `json:"workflowExecutionName,omitempty"`
	ThreadName             string   `json:"threadName,omitempty"`
	SubFlow                *SubFlow `json:"subFlow,omitempty"`
}

type WorkflowStepState string

const (
	WorkflowStepStatePending  WorkflowStepState = "Pending"
	WorkflowStepStateRunning  WorkflowStepState = "Running"
	WorkflowStepStateError    WorkflowStepState = "Error"
	WorkflowStepStateComplete WorkflowStepState = "Complete"
)

type WorkflowStepStatus struct {
	State        WorkflowStepState  `json:"state,omitempty"`
	Error        string             `json:"message,omitempty"`
	ThreadName   string             `json:"threadName,omitempty"`
	FirstRunName string             `json:"firstRunName,omitempty"`
	LastRunName  string             `json:"lastRunName,omitempty"`
	Conditions   []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowStepList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []WorkflowStep `json:"items"`
}
