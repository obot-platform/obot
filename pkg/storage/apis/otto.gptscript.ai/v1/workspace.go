package v1

import (
	"github.com/acorn-io/baaah/pkg/fields"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkspaceSpec   `json:"spec,omitempty"`
	Status            WorkspaceStatus `json:"status,omitempty"`
}

func (in *Workspace) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *Workspace) Get(field string) string {
	if in == nil {
		return ""
	}

	switch field {
	case "spec.agentName":
		return in.Spec.AgentName
	case "spec.workflowName":
		return in.Spec.WorkflowName
	case "spec.threadName":
		return in.Spec.ThreadName
	case "spec.knowledgeSetName":
		return in.Spec.KnowledgeSetName
	}

	return ""
}

func (*Workspace) FieldNames() []string {
	return []string{"spec.agentName", "spec.workflowName", "spec.threadName", "spec.knowledgeSetName"}
}

var _ fields.Fields = (*Workspace)(nil)

func (in *Workspace) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(Thread), Name: in.Spec.ThreadName},
		{ObjType: new(Agent), Name: in.Spec.AgentName},
		{ObjType: new(Workflow), Name: in.Spec.WorkflowName},
		{ObjType: new(KnowledgeSet), Name: in.Spec.KnowledgeSetName},
	}
}

type WorkspaceSpec struct {
	AgentName        string   `json:"agentName,omitempty"`
	WorkflowName     string   `json:"workflowName,omitempty"`
	ThreadName       string   `json:"threadName,omitempty"`
	KnowledgeSetName string   `json:"knowledgeSetName,omitempty"`
	IsKnowledge      bool     `json:"isKnowledge,omitempty"`
	FromWorkspaces   []string `json:"fromWorkspaces,omitempty"`
	WorkspaceID      string   `json:"workspaceID,omitempty"`
}

type WorkspaceStatus struct {
	WorkspaceID          string                `json:"workspaceID,omitempty"`
	IngestionGeneration  int64                 `json:"ingestionGeneration,omitempty"`
	IngestionRunHash     string                `json:"ingestionRunHash,omitempty"`
	IngestionRunName     string                `json:"ingestionRunName,omitempty"`
	IngestionLastRunTime metav1.Time           `json:"ingestionLastRunTime,omitempty"`
	LastNotFinished      map[string]types.Item `json:"lastNotFinished,omitempty"`
	NotFinished          map[string]types.Item `json:"notFinished,omitempty"`
	RetryCount           int                   `json:"retryCount,omitempty"`
	PendingApproval      []string              `json:"pendingApproval,omitempty"`
	PendingRejections    []string              `json:"pendingRejections,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}
