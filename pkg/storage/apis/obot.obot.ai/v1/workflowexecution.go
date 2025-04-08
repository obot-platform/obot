package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*WorkflowExecution)(nil)
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
		case "spec.threadName":
			return in.Spec.ThreadName
		case "spec.webhookName":
			return in.Spec.WebhookName
		case "spec.cronJobName":
			return in.Spec.CronJobName
		case "spec.workflowName":
			return in.Spec.WorkflowName
		}
	}

	return ""
}

func (in *WorkflowExecution) FieldNames() []string {
	return []string{
		"spec.threadName",
		"spec.webhookName",
		"spec.cronJobName",
		"spec.workflowName",
		"spec.parentRunName",
	}
}

func (in *WorkflowExecution) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"State", "Status.State"},
		{"Thread", "Status.ThreadName"},
		{"Workflow", "Spec.WorkflowName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type WorkflowExecutionSpec struct {
	Input string `json:"input,omitempty"`
	// ThreadName is the name of the thread that owns this execution, which is the same as the owning thread of the workflow.
	ThreadName         string `json:"threadName,omitempty"`
	WorkflowName       string `json:"workflowName,omitempty"`
	WebhookName        string `json:"webhookName,omitempty"`
	EmailReceiverName  string `json:"emailReceiverName,omitempty"`
	CronJobName        string `json:"cronJobName,omitempty"`
	WorkflowGeneration int64  `json:"workflowGeneration,omitempty"`
	RunUntilStep       string `json:"runUntilStep,omitempty"`
	// The Run that started this execution
	RunName string `json:"runName,omitempty"`
	// TaskBreadCrumb is a comma-delimited list of taskID calls made to execute this task.
	// This helps to prevent cycles when tasks call tasks.
	TaskBreakCrumb string `json:"taskBreakCrumb,omitempty"`
}

func (in *WorkflowExecution) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
		{ObjType: &Thread{}, Name: in.Status.ThreadName},
		{ObjType: &Run{}, Name: in.Spec.RunName},
	}
}

type WorkflowExecutionStatus struct {
	State              types.WorkflowState     `json:"state,omitempty"`
	Output             string                  `json:"output,omitempty"`
	Error              string                  `json:"error,omitempty"`
	ThreadName         string                  `json:"threadName,omitempty"`
	WorkflowManifest   *types.WorkflowManifest `json:"workflowManifest,omitempty"`
	EndTime            *metav1.Time            `json:"endTime,omitempty"`
	WorkflowGeneration int64                   `json:"workflowGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []WorkflowExecution `json:"items"`
}
