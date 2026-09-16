package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*UserRoleChange)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserRoleChange struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   UserRoleChangeSpec `json:"spec"`
	Status EmptyStatus        `json:"status"`
}

type UserRoleChangeSpec struct {
	UserID uint `json:"userID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserRoleChangeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []UserRoleChange `json:"items"`
}

func (in *UserRoleChange) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *UserRoleChange) Get(field string) string {
	switch field {
	case "spec.userID":
		return strconv.FormatUint(uint64(in.Spec.UserID), 10)
	}
	return ""
}

func (*UserRoleChange) FieldNames() []string {
	return []string{"spec.userID"}
}
