package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ProjectV2)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProjectV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectV2Spec   `json:"spec,omitempty"`
	Status ProjectV2Status `json:"status,omitempty"`
}

func (in *ProjectV2) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ProjectV2) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	}
	return ""
}

func (in *ProjectV2) FieldNames() []string {
	return []string{"spec.userID"}
}

type ProjectV2Spec struct {
	types.ProjectV2Manifest `json:",inline"`

	// UserID is the user that created this project
	UserID string `json:"userID,omitempty"`
}

type ProjectV2Status struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProjectV2List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ProjectV2 `json:"items"`
}
