package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/acorn-io/baaah/pkg/fields"
	"github.com/gptscript-ai/otto/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*WorkflowExecution)(nil)
	_ fields.Fields         = (*WorkflowExecution)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
	Status WorkflowExecutionStatus `json:"status,omitempty"`
}

func (in *WorkflowExecution) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *WorkflowExecution) Get(field string) string {
	if in != nil {
		switch field {
		case "spec.webhookName":
			return in.Spec.WebhookName
		case "spec.cronJobName":
			return in.Spec.CronJobName
		}
	}

	return ""
}

func (in *WorkflowExecution) FieldNames() []string {
	return []string{"spec.webhookName", "spec.cronJobName"}
}

func (in *WorkflowExecution) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"State", "Status.State"},
		{"Thread", "Status.ThreadName"},
		{"Workflow", "Spec.WorkflowName"},
		{"After", "Spec.AfterWorkflowStepName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *WorkflowExecution) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type WorkflowExecutionSpec struct {
	Input                 string `json:"input,omitempty"`
	WorkflowName          string `json:"workflowName,omitempty"`
	WebhookName           string `json:"webhookName,omitempty"`
	CronJobName           string `json:"cronJobName,omitempty"`
	AfterWorkflowStepName string `json:"afterWorkflowStepName,omitempty"`
	WorkspaceName         string `json:"workspaceName,omitempty"`
}

func (in *WorkflowExecution) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Status.ThreadName},
	}
}

type WorkflowState string

const (
	WorkflowStatePending  WorkflowState = "Pending"
	WorkflowStateRunning  WorkflowState = "Running"
	WorkflowStateError    WorkflowState = "Error"
	WorkflowStateComplete WorkflowState = "Complete"
)

type WorkflowExecutionStatus struct {
	State            WorkflowState           `json:"state,omitempty"`
	Output           string                  `json:"output,omitempty"`
	ThreadName       string                  `json:"threadName,omitempty"`
	WorkflowManifest *types.WorkflowManifest `json:"workflowManifest,omitempty"`
	EndTime          *metav1.Time            `json:"endTime,omitempty"`
	Conditions       []metav1.Condition      `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []WorkflowExecution `json:"items"`
}
