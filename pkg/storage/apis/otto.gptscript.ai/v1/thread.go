package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	gptscriptclient "github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*Thread)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Thread struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadSpec   `json:"spec,omitempty"`
	Status ThreadStatus `json:"status,omitempty"`
}

func (in *Thread) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"CurrentRun", "Status.CurrentRunName"},
		{"LastRun", "Status.LastRunName"},
		{"LastRunState", "Status.LastRunState"},
		{"WorkflowState", "Status.WorkflowState"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *Thread) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type ThreadSpec struct {
	Manifest                  types.ThreadManifest `json:"manifest,omitempty"`
	ParentThreadName          string               `json:"parentThreadName,omitempty"`
	AgentName                 string               `json:"agentName,omitempty"`
	WorkflowName              string               `json:"workflowName,omitempty"`
	WorkflowExecutionName     string               `json:"workflowExecutionName,omitempty"`
	RemoteKnowledgeSourceName string               `json:"remoteKnowledgeSourceName,omitempty"`
	WebhookName               string               `json:"webhookName,omitempty"`
	CronJobName               string               `json:"cronJobName,omitempty"`
	WorkspaceID               string               `json:"workspaceID,omitempty"`
}

func (in *Thread) DeleteRefs() []Ref {
	return []Ref{
		{&WorkflowExecution{}, in.Spec.WorkflowExecutionName},
		{&Workflow{}, in.Spec.WorkflowName},
		{&CronJob{}, in.Spec.CronJobName},
		{&Webhook{}, in.Spec.WebhookName},
		{&Thread{}, in.Status.PreviousThreadName},
		{&RemoteKnowledgeSource{}, in.Spec.RemoteKnowledgeSourceName},
	}
}

type ThreadStatus struct {
	LastRunName        string                   `json:"lastRunName,omitempty"`
	CurrentRunName     string                   `json:"currentRunName,omitempty"`
	LastRunState       gptscriptclient.RunState `json:"lastRunState,omitempty"`
	WorkflowState      types.WorkflowState      `json:"workflowState,omitempty"`
	PreviousThreadName string                   `json:"previousThreadName,omitempty"`
	KnowledgeSetNames  []string                 `json:"knowledgeSetNames,omitempty"`
	Conditions         []metav1.Condition       `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Thread `json:"items"`
}
