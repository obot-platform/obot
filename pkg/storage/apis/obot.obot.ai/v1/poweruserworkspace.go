package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*PowerUserWorkspace)(nil)
	_ DeleteRefs    = (*PowerUserWorkspace)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PowerUserWorkspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PowerUserWorkspaceSpec   `json:"spec,omitempty"`
	Status PowerUserWorkspaceStatus `json:"status,omitempty"`
}

func (in *PowerUserWorkspace) Has(field string) (exists bool) {
	return in.Get(field) != ""
}

func (in *PowerUserWorkspace) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.role":
		return string(in.Spec.Role)
	}
	return ""
}

func (in *PowerUserWorkspace) FieldNames() []string {
	return []string{
		"spec.userID",
		"spec.role",
	}
}

func (in *PowerUserWorkspace) DeleteRefs() []Ref {
	// When a PowerUserWorkspace is deleted, it should clean up all resources owned by it
	return []Ref{
		// Note: We can't specify exact names here since we don't know them at compile time.
		// The controller will need to handle finding and deleting workspace-scoped resources
		// by querying for resources with PowerUserWorkspaceName field matching this workspace's name.
	}
}

type PowerUserWorkspaceSpec struct {
	// UserID is the ID of the user who owns this workspace
	UserID string `json:"userID,omitempty"`
	// Role is the role of the user (PowerUser, PowerUserPlus, or Admin)
	Role types.Role `json:"role,omitempty"`
}

type PowerUserWorkspaceStatus struct {
	// Ready indicates if the workspace is ready for use
	Ready bool `json:"ready,omitempty"`
	// ResourceCount tracks the number of resources owned by this workspace
	ResourceCount *PowerUserWorkspaceResourceCount `json:"resourceCount,omitempty"`
}

type PowerUserWorkspaceResourceCount struct {
	MCPServers               int `json:"mcpServers,omitempty"`
	MCPServerCatalogEntries  int `json:"mcpServerCatalogEntries,omitempty"`
	AccessControlRules       int `json:"accessControlRules,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PowerUserWorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PowerUserWorkspace `json:"items"`
}