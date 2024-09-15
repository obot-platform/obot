package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	gptscriptclient "github.com/gptscript-ai/go-gptscript"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*Run)(nil)
)

const (
	AgentFinalizer             = "otto.gptscript.ai/agent"
	RunFinalizer               = "otto.gptscript.ai/run"
	ThreadFinalizer            = "otto.gptscript.ai/thread"
	WorkflowExecutionFinalizer = "otto.gptscript.ai/workflow-execution"
	WorkflowFinalizer          = "otto.gptscript.ai/workflow"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Run struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunSpec   `json:"spec,omitempty"`
	Status RunStatus `json:"status,omitempty"`
}

func (in *Run) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type Progress struct {
	Content        string       `json:"content"`
	Tool           ToolProgress `json:"tool"`
	WaitingOnModel bool         `json:"waitingOnModel,omitempty"`
	Error          string       `json:"error,omitempty"`
}

type ToolProgress struct {
	Name                   string `json:"name,omitempty"`
	Description            string `json:"description,omitempty"`
	Input                  string `json:"input,omitempty"`
	PartialInput           string `json:"partialInput,omitempty"`
	GeneratingInputForName string `json:"generatingInputForName,omitempty"`
	GeneratingInput        bool   `json:"generatingInput,omitempty"`
}

type RunSpec struct {
	Background       bool     `json:"background,omitempty"`
	ThreadName       string   `json:"threadName,omitempty"`
	AgentName        string   `json:"agentName,omitempty"`
	WorkflowName     string   `json:"workflowName,omitempty"`
	WorkflowStepName string   `json:"workflowStepName,omitempty"`
	PreviousRunName  string   `json:"previousRunName,omitempty"`
	Input            string   `json:"input"`
	Env              []string `json:"env,omitempty"`
	Tool             string   `json:"tool,omitempty"`
}

type RunStatus struct {
	Conditions []metav1.Condition       `json:"conditions,omitempty"`
	State      gptscriptclient.RunState `json:"state,omitempty"`
	Output     string                   `json:"output"`
	Error      string                   `json:"error,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Run `json:"items"`
}
