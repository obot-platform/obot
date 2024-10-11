package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/otto8-ai/otto8/apiclient/types"
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
	Manifest            types.AgentManifest `json:"manifest,omitempty"`
	InputFilters        []string            `json:"inputFilters,omitempty"`
	CredentialContextID string              `json:"credentialContextID,omitempty"`
}

type AgentStatus struct {
	Conditions        []metav1.Condition        `json:"conditions,omitempty"`
	External          types.AgentExternalStatus `json:"external,omitempty"`
	KnowledgeSetNames []string                  `json:"knowledgeSetNames,omitempty"`
	WorkspaceName     string                    `json:"workspaceName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Agent `json:"items"`
}
