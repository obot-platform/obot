package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	gptscriptclient "github.com/gptscript-ai/go-gptscript"
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

func (in *Thread) GetKnowledgeWorkspaceStatus() *KnowledgeWorkspaceStatus {
	// This is crazy hack and may cause issues in the future. So if it does, find a better way. That's your problem.
	if in.Spec.WorkspaceID != "" && in.Status.Workspace.WorkspaceID == "" {
		in.Status.Workspace.WorkspaceID = in.Spec.WorkspaceID
	}
	return &in.Status.KnowledgeWorkspace
}

func (in *Thread) GetWorkspaceStatus() *WorkspaceStatus {
	// This is crazy hack and may cause issues in the future. So if it does, find a better way. That's your problem.
	if in.Spec.KnowledgeWorkspaceID != "" && in.Status.KnowledgeWorkspace.KnowledgeWorkspaceID == "" {
		in.Status.KnowledgeWorkspace.KnowledgeWorkspaceID = in.Spec.KnowledgeWorkspaceID
	}
	return &in.Status.Workspace
}

func (in *Thread) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type ThreadSpec struct {
	AgentName             string `json:"agentName,omitempty"`
	WorkflowName          string `json:"workflowName,omitempty"`
	WorkflowExecutionName string `json:"workflowExecutionName,omitempty"`
	WorkflowStepName      string `json:"workflowStepName,omitempty"`
	WorkspaceID           string `json:"workspaceID,omitempty"`
	KnowledgeWorkspaceID  string `json:"knowledgeWorkspaceID,omitempty"`
}

type ThreadStatus struct {
	Description        string                   `json:"description,omitempty"`
	LastRunName        string                   `json:"lastRunName,omitempty"`
	LastRunState       gptscriptclient.RunState `json:"lastRunState,omitempty"`
	LastRunOutput      string                   `json:"lastRunOutput,omitempty"`
	LastRunError       string                   `json:"lastRunError,omitempty"`
	Conditions         []metav1.Condition       `json:"conditions,omitempty"`
	Workspace          WorkspaceStatus          `json:"workspace,omitempty"`
	KnowledgeWorkspace KnowledgeWorkspaceStatus `json:"knowledgeWorkspace,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Thread `json:"items"`
}
