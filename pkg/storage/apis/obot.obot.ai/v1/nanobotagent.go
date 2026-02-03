package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*NanobotAgent)(nil)
	_ DeleteRefs    = (*NanobotAgent)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NanobotAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NanobotAgentSpec   `json:"spec,omitempty"`
	Status NanobotAgentStatus `json:"status,omitempty"`
}

func (in *NanobotAgent) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *NanobotAgent) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.projectV2ID":
		return in.Spec.ProjectV2ID
	}
	return ""
}

func (in *NanobotAgent) FieldNames() []string {
	return []string{"spec.userID", "spec.projectV2ID"}
}

func (in *NanobotAgent) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &ProjectV2{},
			Name:    in.Spec.ProjectV2ID,
		},
	}
}

type NanobotAgentSpec struct {
	types.NanobotAgentManifest `json:",inline"`

	// UserID is the user that created this nanobot workflow
	UserID string `json:"userID,omitempty"`

	// ProjectV2ID is the project this workflow belongs to
	ProjectV2ID string `json:"projectV2ID,omitempty"`
}

type NanobotAgentStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NanobotAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NanobotAgent `json:"items"`
}
