package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserRoleChange struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserRoleChangeSpec `json:"spec,omitempty"`
	Status EmptyStatus        `json:"status,omitempty"`
}

type UserRoleChangeSpec struct {
	UserID  uint       `json:"userID,omitempty"`
	OldRole types.Role `json:"oldRole,omitempty"` // deprecated and unused
	NewRole types.Role `json:"newRole,omitempty"` // deprecated and unused
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserRoleChangeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserRoleChange `json:"items"`
}
