package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkspaceSpec   `json:"spec,omitempty"`
	Status            WorkspaceStatus `json:"status,omitempty"`
}

func (in *Workspace) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Agent", "Spec.AgentName"},
		{"Workflow", "Spec.WorkflowName"},
		{"Thread", "Spec.ThreadName"},
		{"KnowledgeSet", "Spec.KnowledgeSetName"},
		{"KnowledgeSource", "Spec.KnowledgeSourceName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
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
		{ObjType: new(ThreadTemplate), Name: in.Spec.ThreadTemplateName},
		{ObjType: new(Agent), Name: in.Spec.AgentName},
		{ObjType: new(Workflow), Name: in.Spec.WorkflowName},
		{ObjType: new(KnowledgeSet), Name: in.Spec.KnowledgeSetName},
		{ObjType: new(KnowledgeSource), Name: in.Spec.KnowledgeSourceName},
	}
}

type WorkspaceSpec struct {
	AgentName           string   `json:"agentName,omitempty"`
	WorkflowName        string   `json:"workflowName,omitempty"`
	ThreadTemplateName  string   `json:"threadTemplateName,omitempty"`
	ThreadName          string   `json:"threadName,omitempty"`
	KnowledgeSetName    string   `json:"knowledgeSetName,omitempty"`
	KnowledgeSourceName string   `json:"knowledgeSourceName,omitempty"`
	FromWorkspaceNames  []string `json:"fromWorkspaceNames,omitempty"`
}

type WorkspaceStatus struct {
	WorkspaceID string `json:"workspaceID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}
