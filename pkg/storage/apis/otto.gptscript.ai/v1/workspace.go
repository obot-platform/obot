package v1

import (
	"github.com/acorn-io/baaah/pkg/fields"
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
	}

	return ""
}

func (*Workspace) FieldNames() []string {
	return []string{"spec.agentName", "spec.workflowName", "spec.threadName"}
}

var _ fields.Fields = (*Workspace)(nil)

func (in *Workspace) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(Thread), Name: in.Spec.ThreadName},
		{ObjType: new(Agent), Name: in.Spec.AgentName},
		{ObjType: new(Workflow), Name: in.Spec.WorkflowName},
	}
}

type WorkspaceSpec struct {
	AgentName      string   `json:"agentName,omitempty"`
	WorkflowName   string   `json:"workflowName,omitempty"`
	ThreadName     string   `json:"threadName,omitempty"`
	IsKnowledge    bool     `json:"isKnowledge,omitempty"`
	FromWorkspaces []string `json:"fromWorkspaces,omitempty"`
	WorkspaceID    string   `json:"workspaceID,omitempty"`
}

type WorkspaceStatus struct {
	WorkspaceID             string      `json:"workspaceID,omitempty"`
	HasKnowledge            bool        `json:"hasKnowledge,omitempty"`
	LastIngestionRunStarted metav1.Time `json:"lastIngestionRunStarted,omitempty"`
	IngestionRunName        string      `json:"ingestionRunName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type IngestKnowledgeRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IngestKnowledgeRequestSpec   `json:"spec,omitempty"`
	Status            IngestKnowledgeRequestStatus `json:"status,omitempty"`
}

func (in *IngestKnowledgeRequest) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *IngestKnowledgeRequest) Get(field string) string {
	if in == nil {
		return ""
	}

	switch field {
	case "spec.workspaceName":
		return in.Spec.WorkspaceName
	}

	return ""
}

func (*IngestKnowledgeRequest) FieldNames() []string {
	return []string{"spec.workspaceName"}
}

var _ fields.Fields = (*IngestKnowledgeRequest)(nil)

type IngestKnowledgeRequestSpec struct {
	WorkspaceName string `json:"workspaceName,omitempty"`
	HasKnowledge  bool   `json:"hasKnowledge,omitempty"`
}

type IngestKnowledgeRequestStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type IngestKnowledgeRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IngestKnowledgeRequest `json:"items"`
}
