package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*PowerUserWorkspace)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PowerUserWorkspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PowerUserWorkspaceSpec   `json:"spec,omitempty"`
	Status PowerUserWorkspaceStatus `json:"status,omitempty"`
}

func (in *PowerUserWorkspace) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *PowerUserWorkspace) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.role":
		return strconv.Itoa(int(in.Spec.Role))
	}
	return ""
}

func (in *PowerUserWorkspace) FieldNames() []string {
	return []string{
		"spec.userID",
		"spec.role",
	}
}

type PowerUserWorkspaceSpec struct {
	// UserID is the ID of the user who owns this workspace
	UserID string `json:"userID,omitempty"`
	// Role is the role of the user (Admin, PowerUser, or PowerUserPlus)
	Role types.Role `json:"role,omitempty"`
}

type PowerUserWorkspaceStatus struct {
	// ResourceCount tracks the number of resources owned by this workspace
	ResourceCount int `json:"resourceCount,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PowerUserWorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PowerUserWorkspace `json:"items"`
}
