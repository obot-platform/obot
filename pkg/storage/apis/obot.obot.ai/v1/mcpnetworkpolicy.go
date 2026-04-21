package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*MCPNetworkPolicy)(nil)
	_ DeleteRefs    = (*MCPNetworkPolicy)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPNetworkPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPNetworkPolicySpec   `json:"spec,omitempty"`
	Status MCPNetworkPolicyStatus `json:"status,omitempty"`
}

func (in *MCPNetworkPolicy) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPNetworkPolicy) Get(field string) string {
	switch field {
	case "spec.mcpServerName":
		return in.Spec.MCPServerName
	}
	return ""
}

func (*MCPNetworkPolicy) FieldNames() []string {
	return []string{"spec.mcpServerName"}
}

func (in *MCPNetworkPolicy) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &MCPServer{}, Name: in.Spec.MCPServerName},
	}
}

type MCPNetworkPolicySpec struct {
	MCPServerName string            `json:"mcpServerName,omitempty"`
	PodSelector   map[string]string `json:"podSelector,omitempty"`
	EgressDomains []string          `json:"egressDomains,omitempty"`
	DenyAllEgress bool              `json:"denyAllEgress,omitempty"`
}

type MCPNetworkPolicyStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPNetworkPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPNetworkPolicy `json:"items"`
}
