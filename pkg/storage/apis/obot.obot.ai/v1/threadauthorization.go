package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ThreadAuthorization)(nil)
	_ DeleteRefs    = (*ThreadAuthorization)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadAuthorization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadAuthorizationSpec   `json:"spec,omitempty"`
	Status ThreadAuthorizationStatus `json:"status,omitempty"`
}

func (in *ThreadAuthorization) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &Thread{},
			Name:    in.Spec.ThreadID,
		},
	}
}

func (in *ThreadAuthorization) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ThreadAuthorization) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.threadID":
		return in.Spec.ThreadID
	case "spec.accepted":
		return strconv.FormatBool(in.Spec.Accepted)
	}
	return ""
}

func (in *ThreadAuthorization) FieldNames() []string {
	return []string{"spec.userID", "spec.threadID", "spec.accepted"}
}

type ThreadAuthorizationSpec struct {
	types.ThreadAuthorizationManifest
	Accepted bool `json:"accepted,omitempty"`
}

type ThreadAuthorizationStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadAuthorizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ThreadAuthorization `json:"items"`
}
