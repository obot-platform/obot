package v1

import (
	"github.com/otto8-ai/nah/pkg/fields"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ToolReference)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ToolReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ToolReferenceSpec   `json:"spec,omitempty"`
	Status ToolReferenceStatus `json:"status,omitempty"`
}

func (in *ToolReference) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *ToolReference) Get(field string) string {
	if in != nil {
		switch field {
		case "spec.type":
			return string(in.Spec.Type)
		}
	}

	return ""
}

func (in *ToolReference) FieldNames() []string {
	return []string{"spec.type"}
}

func (in *ToolReference) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Reference", "Spec.Reference"},
		{"Error", "Status.Error"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type ToolReferenceSpec struct {
	Type      types.ToolReferenceType `json:"type,omitempty"`
	Builtin   bool                    `json:"builtin,omitempty"`
	Reference string                  `json:"reference,omitempty"`
	Active    *bool                   `json:"active,omitempty"`
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
	LastReferenceCheck metav1.Time           `json:"lastReferenceCheck,omitempty"`
	Tool               *ToolShortDescription `json:"tool,omitempty"`
	Error              string                `json:"error,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ToolReferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ToolReference `json:"items"`
}
