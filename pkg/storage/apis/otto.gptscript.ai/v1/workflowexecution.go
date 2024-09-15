package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*WorkflowExecution)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
	Status WorkflowExecutionStatus `json:"status,omitempty"`
}

func (in *WorkflowExecution) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type WorkflowExecutionSpec struct {
	Input                 string `json:"input,omitempty"`
	WorkflowName          string `json:"workflowName,omitempty"`
	AfterWorkflowStepName string `json:"afterWorkflowStepName,omitempty"`
	WorkspaceID           string `json:"workspaceID,omitempty"`
}

type WorkflowState string

const (
	WorkflowStatePending  WorkflowState = "Pending"
	WorkflowStateRunning  WorkflowState = "Running"
	WorkflowStateError    WorkflowState = "Error"
	WorkflowStateComplete WorkflowState = "Complete"
)

type WorkflowExecutionExternalStatus struct {
	State    WorkflowState `json:"state,omitempty"`
	Output   string        `json:"output,omitempty"`
	ThreadID string        `json:"threadID,omitempty"`
}

type WorkflowExecutionStatus struct {
	External         WorkflowExecutionExternalStatus `json:"external,omitempty"`
	WorkflowManifest *WorkflowManifest               `json:"workflowManifest,omitempty"`
	Conditions       []metav1.Condition              `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []WorkflowExecution `json:"items"`
}
