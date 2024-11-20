package v1

import (
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ Aliasable   = (*Workflow)(nil)
	_ AliasScoped = (*Workflow)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec,omitempty"`
	Status WorkflowStatus `json:"status,omitempty"`
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

type WorkflowSpec struct {
	Manifest types.WorkflowManifest `json:"manifest,omitempty"`
}

type WorkflowStatus struct {
	WorkspaceName     string                                   `json:"workspaceName,omitempty"`
	KnowledgeSetNames []string                                 `json:"knowledgeSetNames,omitempty"`
	AliasAssigned     bool                                     `json:"aliasAssigned,omitempty"`
	AuthStatus        map[string]types.OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Workflow `json:"items"`
}
