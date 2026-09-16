package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*UserGroupChange)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserGroupChange struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   UserGroupChangeSpec `json:"spec"`
	Status EmptyStatus         `json:"status"`
}

type UserGroupChangeSpec struct {
	UserID uint `json:"userID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserGroupChangeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []UserGroupChange `json:"items"`
}

func (in *UserGroupChange) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *UserGroupChange) Get(field string) string {
	switch field {
	case "spec.userID":
		return strconv.FormatUint(uint64(in.Spec.UserID), 10)
	}
	return ""
}

func (*UserGroupChange) FieldNames() []string {
	return []string{"spec.userID"}
}
