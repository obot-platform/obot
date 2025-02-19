package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ThreadTemplateAuthorization)(nil)
	_ DeleteRefs    = (*ThreadTemplateAuthorization)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadTemplateAuthorization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadTemplateAuthorizationSpec   `json:"spec,omitempty"`
	Status ThreadTemplateAuthorizationStatus `json:"status,omitempty"`
}

func (in *ThreadTemplateAuthorization) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &ThreadTemplate{},
			Name:    in.Spec.TemplateID,
		},
	}
}

func (in *ThreadTemplateAuthorization) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ThreadTemplateAuthorization) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.templateID":
		return in.Spec.TemplateID
	}
	return ""
}

func (in *ThreadTemplateAuthorization) FieldNames() []string {
	return []string{"spec.userID", "spec.templateID"}
}

type ThreadTemplateAuthorizationSpec struct {
	types.TemplateAuthorizationManifest
}

type ThreadTemplateAuthorizationStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadTemplateAuthorizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ThreadTemplateAuthorization `json:"items"`
}
