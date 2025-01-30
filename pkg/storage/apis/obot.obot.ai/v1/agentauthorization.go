package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*AgentAuthorization)(nil)
	_ DeleteRefs    = (*AgentAuthorization)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentAuthorization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentAuthorizationSpec   `json:"spec,omitempty"`
	Status AgentAuthorizationStatus `json:"status,omitempty"`
}

func (in *AgentAuthorization) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &Agent{},
			Name:    in.Spec.AgentID,
		},
	}
}

func (in *AgentAuthorization) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *AgentAuthorization) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.agentID":
		return in.Spec.AgentID
	}
	return ""
}

func (in *AgentAuthorization) FieldNames() []string {
	return []string{"spec.userID", "spec.agentID"}
}

type AgentAuthorizationSpec struct {
	types.AgentAuthorizationManifest
}

type AgentAuthorizationStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentAuthorizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AgentAuthorization `json:"items"`
}
