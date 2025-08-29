package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PowerUserWorkspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PowerUserWorkspaceSpec   `json:"spec,omitempty"`
	Status PowerUserWorkspaceStatus `json:"status,omitempty"`
}

func (in *PowerUserWorkspace) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"User ID", "Spec.UserID"},
		{"Role", "Spec.Role"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

// DeleteRefs returns references to resources that should be deleted when this workspace is deleted
// Note: The cascade deletion system in Obot works differently - it uses controllers to handle workspace cleanup
func (in *PowerUserWorkspace) DeleteRefs() []Ref {
	return []Ref{
		// Since we can't specify field paths in Refs, the cleanup will be handled
		// by the PowerUserWorkspace controller which will query and delete resources
		// that have spec.powerUserWorkspaceID matching this workspace's name
	}
}

type PowerUserWorkspaceSpec struct {
	// UserID is the ID of the user who owns this workspace
	UserID string `json:"userID"`
	// Role is the user's role (admin, power-user, power-user-plus)
	Role types.Role `json:"role"`
}

type PowerUserWorkspaceStatus struct {
	// ResourceCounts tracks the number of resources in this workspace
	ResourceCounts PowerUserWorkspaceResourceCounts `json:"resourceCounts,omitempty"`
}

type PowerUserWorkspaceResourceCounts struct {
	AccessControlRules       int `json:"accessControlRules,omitempty"`
	MCPServers              int `json:"mcpServers,omitempty"`
	MCPServerCatalogEntries int `json:"mcpServerCatalogEntries,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PowerUserWorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PowerUserWorkspace `json:"items"`
}