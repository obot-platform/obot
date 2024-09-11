package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*Agent)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

func (in *Agent) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type AgentSpec struct {
	Manifest AgentManifest `json:"manifest,omitempty"`
}

type AgentStatus struct {
	Conditions           []metav1.Condition `json:"conditions,omitempty"`
	HasKnowledge         bool               `json:"hasKnowledge,omitempty"`
	IngestKnowledge      bool               `json:"ingestKnowledge,omitempty"`
	WorkspaceID          string             `json:"workspaceID,omitempty"`
	KnowledgeWorkspaceID string             `json:"knowledgeWorkspaceID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Agent `json:"items"`
}
