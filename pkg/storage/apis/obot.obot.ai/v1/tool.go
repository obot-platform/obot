package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*Tool)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Tool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ToolSpec   `json:"spec,omitempty"`
	Status ToolStatus `json:"status,omitempty"`
}

type ToolSpec struct {
	ThreadName string             `json:"threadName,omitempty"`
	Manifest   types.ToolManifest `json:"manifest,omitempty"`
	Envs       []string           `json:"envs,omitempty"`
}

func (in *Tool) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *Tool) Get(field string) string {
	if in == nil {
		return ""
	}

	switch field {
	case "spec.threadName":
		return in.Spec.ThreadName
	}

	return ""
}

func (*Tool) FieldNames() []string {
	return []string{"spec.threadName"}
}

type ToolStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ToolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Tool `json:"items"`
}

// +k8s:deepcopy-gen=false

type ToolUser interface {
	Generationed
	GetTools() []string
	GetToolInfos() map[string]types.ToolInfo
	SetToolInfos(map[string]types.ToolInfo)
}
