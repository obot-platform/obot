package v1

import (
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*ThreadShare)(nil)
var _ DeleteRefs = (*ThreadShare)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadShare struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadShareSpec   `json:"spec,omitempty"`
	Status ThreadShareStatus `json:"status,omitempty"`
}

func (in *ThreadShare) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &Thread{},
			Name:    in.Spec.ProjectThreadName,
		},
	}
}

func (in *ThreadShare) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ThreadShare) Get(field string) (value string) {
	switch field {
	case "spec.publicID":
		return in.Spec.PublicID
	case "spec.userID":
		return in.Spec.UserID
	case "spec.featured":
		return fmt.Sprint(in.Spec.Featured)
	default:
		return ""
	}
}

func (in *ThreadShare) FieldNames() []string {
	return []string{"spec.publicID", "spec.userID", "spec.featured"}
}

type ThreadShareSpec struct {
	Manifest          types.ProjectShareManifest `json:"manifest,omitempty"`
	PublicID          string                     `json:"publicID,omitempty"`
	UserID            string                     `json:"userID,omitempty"`
	ProjectThreadName string                     `json:"projectThreadName,omitempty"`
	Featured          bool                       `json:"featured,omitempty"`
}

type ThreadShareStatus struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Icons       *types.AgentIcons `json:"icons"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ThreadShare `json:"items"`
}
