package v1

import (
	gptscriptclient "github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RunFinalizer             = "obot.obot.ai/run"
	ThreadFinalizer          = "obot.obot.ai/thread"
	KnowledgeFileFinalizer   = "obot.obot.ai/knowledge-file"
	WorkspaceFinalizer       = "obot.obot.ai/workspace"
	KnowledgeSetFinalizer    = "obot.obot.ai/knowledge-set"
	KnowledgeSourceFinalizer = "obot.obot.ai/knowledge-source"
	ToolReferenceFinalizer   = "obot.obot.ai/tool-reference"
	AgentFinalizer           = "obot.obot.ai/agent"
	WorkflowFinalizer        = "obot.obot.ai/workflow"

	ModelProviderSyncAnnotation = "obot.ai/model-provider-sync"
	WorkflowSyncAnnotation      = "obot.ai/workflow-sync"
	AgentSyncAnnotation         = "obot.ai/agent-sync"
	AuthProviderSyncAnnotation  = "obot.ai/auth-provider-sync"
)

var (
	_ fields.Fields = (*Run)(nil)
	_ DeleteRefs    = (*Run)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Run struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunSpec   `json:"spec,omitempty"`
	Status RunStatus `json:"status,omitempty"`
}

func (in *Run) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *Run) Get(field string) string {
	if in != nil {
		switch field {
		case LabelInactive:
			return in.Labels[LabelInactive]
		case "spec.threadName":
			return in.Spec.ThreadName
		case "spec.previousRunName":
			return in.Spec.PreviousRunName
		}
	}

	return ""
}

func (in *Run) FieldNames() []string {
	return []string{"spec.threadName", "spec.previousRunName", LabelInactive}
}

func (in *Run) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"PreviousRun", "Spec.PreviousRunName"},
		{"State", "Status.State"},
		{"Thread", "Spec.ThreadName"},
		{"Agent", "Spec.AgentName"},
		{"Workflow", "Spec.WorkflowName"},
		{"Step", "Spec.WorkflowStepName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type RunSpec struct {
	Synchronous           bool                    `json:"synchronous,omitempty"`
	ThreadName            string                  `json:"threadName,omitempty"`
	AgentName             string                  `json:"agentName,omitempty"`
	WorkflowName          string                  `json:"workflowName,omitempty"`
	WorkflowExecutionName string                  `json:"workflowExecutionName,omitempty"`
	WorkflowStepName      string                  `json:"workflowStepName,omitempty"`
	WorkflowStepID        string                  `json:"workflowStepID,omitempty"`
	PreviousRunName       string                  `json:"previousRunName,omitempty"`
	Input                 string                  `json:"input"`
	Env                   []string                `json:"env,omitempty"`
	Tool                  string                  `json:"tool,omitempty"`
	ToolReferenceType     types.ToolReferenceType `json:"toolReferenceType,omitempty"`
	CredentialContextIDs  []string                `json:"credentialContextIDs,omitempty"`
	DefaultModel          string                  `json:"defaultModel,omitempty"`
	Timeout               metav1.Duration         `json:"timeout,omitempty"`
}

func (in *Run) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
		{ObjType: &WorkflowExecution{}, Name: in.Spec.WorkflowExecutionName},
		{ObjType: &WorkflowStep{}, Name: in.Spec.WorkflowStepName},
	}
}

type RunStatus struct {
	Conditions []metav1.Condition       `json:"conditions,omitempty"`
	State      gptscriptclient.RunState `json:"state,omitempty"`
	Output     string                   `json:"output"`
	EndTime    metav1.Time              `json:"endTime,omitempty"`
	Error      string                   `json:"error,omitempty"`
	SubCall    *SubCall                 `json:"subCall,omitempty"`
	TaskResult *TaskResult              `json:"taskResult,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Run `json:"items"`
}
