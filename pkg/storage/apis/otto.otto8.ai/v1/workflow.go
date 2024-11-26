package v1

import (
	"slices"

	"github.com/otto8-ai/nah/pkg/fields"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ Aliasable     = (*Workflow)(nil)
	_ AliasScoped   = (*Workflow)(nil)
	_ fields.Fields = (*Workflow)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec,omitempty"`
	Status WorkflowStatus `json:"status,omitempty"`
}

func (in *Workflow) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *Workflow) Get(field string) (value string) {
	switch field {
	case "spec.agentName":
		return in.Spec.AgentName
	case "spec.userID":
		return in.Spec.UserID
	}
	return ""
}

func (in *Workflow) FieldNames() []string {
	return []string{
		"spec.agentName",
		"spec.userID",
	}
}

func (in *Workflow) GetAliasName() string {
	return in.Spec.Manifest.Alias
}

func (in *Workflow) SetAssigned(assigned bool) {
	in.Status.AliasAssigned = assigned
}

func (in *Workflow) IsAssigned() bool {
	return in.Status.AliasAssigned
}

func (in *Workflow) GetAliasScope() string {
	return "Agent"
}

func (in *Workflow) GetAliasObservedGeneration() int64 {
	return in.Status.AliasObservedGeneration
}

func (in *Workflow) SetAliasObservedGeneration(gen int64) {
	in.Status.AliasObservedGeneration = gen
}

type WorkflowSpec struct {
	AgentName string                 `json:"agentName,omitempty"`
	UserID    string                 `json:"userID,omitempty"`
	Manifest  types.WorkflowManifest `json:"manifest,omitempty"`
}

type WorkflowStatus struct {
	WorkspaceName           string                                   `json:"workspaceName,omitempty"`
	KnowledgeSetNames       []string                                 `json:"knowledgeSetNames,omitempty"`
	AliasAssigned           bool                                     `json:"aliasAssigned,omitempty"`
	AuthStatus              map[string]types.OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
	AliasObservedGeneration int64                                    `json:"aliasProcessed,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Workflow `json:"items"`
}
