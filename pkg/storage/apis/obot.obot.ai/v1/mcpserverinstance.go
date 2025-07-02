package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*MCPServerInstance)(nil)
	_ DeleteRefs    = (*MCPServerInstance)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MCPServerInstanceSpec `json:"spec,omitempty"`
}

func (in *MCPServerInstance) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPServerInstance) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.mcpServerName":
		return in.Spec.MCPServerName
	case "spec.mcpCatalogName":
		return in.Spec.MCPCatalogName
	case "spec.mcpServerCatalogEntryName":
		return in.Spec.MCPServerCatalogEntryName
	}
	return ""
}

func (in *MCPServerInstance) FieldNames() []string {
	return []string{
		"spec.userID",
		"spec.mcpServerName",
		"spec.mcpCatalogName",
		"spec.mcpServerCatalogEntryName",
	}
}

func (in *MCPServerInstance) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &MCPServer{}, Name: in.Spec.MCPServerName},
	}
}

type MCPServerInstanceSpec struct {
	// UserID is the user that owns this MCP server instance.
	UserID string `json:"userID,omitempty"`
	// MCPServerName is the name of the MCP server this instance is associated with.
	MCPServerName string `json:"mcpServerName,omitempty"`
	// MCPCatalogName is the name of the MCP catalog that the server that this instance points to is shared within, if there is one.
	// If there is not one, then this field will be set to the catalog that the Spec.MCPServerCatalogEntryName is in.
	MCPCatalogName string `json:"mcpCatalogName,omitempty"`
	// MCPServerCatalogEntryName is the name of the MCP server catalog entry that the server that this instance points to is based on.
	MCPServerCatalogEntryName string `json:"mcpServerCatalogEntryName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPServerInstance `json:"items"`
}
