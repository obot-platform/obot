package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*ToolReference)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ToolReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ToolReferenceSpec   `json:"spec,omitempty"`
	Status ToolReferenceStatus `json:"status,omitempty"`
}

func (in *ToolReference) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Reference", "Spec.Reference"},
		{"Error", "Status.Error"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *ToolReference) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type ToolReferenceSpec struct {
	Type      types.ToolReferenceType `json:"type,omitempty"`
	Reference string                  `json:"reference,omitempty"`
}

type ToolShortDescription struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Credential  string            `json:"credential,omitempty"`
}

type ToolReferenceStatus struct {
	Reference          string                `json:"reference,omitempty"`
	ObservedGeneration int64                 `json:"observedGeneration,omitempty"`
	Tool               *ToolShortDescription `json:"tool,omitempty"`
	Error              string                `json:"error,omitempty"`
	Conditions         []metav1.Condition    `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ToolReferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ToolReference `json:"items"`
}
