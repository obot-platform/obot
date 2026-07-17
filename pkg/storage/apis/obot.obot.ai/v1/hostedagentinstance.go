package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*HostedAgentInstance)(nil)
	_ DeleteRefs    = (*HostedAgentInstance)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostedAgentInstanceSpec   `json:"spec,omitempty"`
	Status HostedAgentInstanceStatus `json:"status,omitempty"`
}

func (in *HostedAgentInstance) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *HostedAgentInstance) Get(field string) string {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.hostedAgentName":
		return in.Spec.HostedAgentName
	}
	return ""
}

func (in *HostedAgentInstance) FieldNames() []string {
	return []string{"spec.userID", "spec.hostedAgentName"}
}

func (in *HostedAgentInstance) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &HostedAgent{}, Name: in.Spec.HostedAgentName},
	}
}

func (in *HostedAgentInstance) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.Name"},
		{"Hosted Agent", "Spec.HostedAgentName"},
		{"User", "Spec.UserID"},
		{"State", "Status.State"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type HostedAgentInstanceSpec struct {
	UserID          string                            `json:"userID,omitempty"`
	HostedAgentName string                            `json:"hostedAgentName,omitempty"`
	Manifest        types.HostedAgentInstanceManifest `json:"manifest,omitempty"`
}

type HostedAgentInstanceStatus struct {
	types.HostedAgentInstanceStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []HostedAgentInstance `json:"items"`
}
