package v1

import (
	"slices"
	"strconv"

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
	case "spec.powerUserWorkspaceID":
		return in.Spec.PowerUserWorkspaceID
	case "spec.template":
		return strconv.FormatBool(in.Spec.Template)
	case "spec.compositeName":
		return in.Spec.CompositeName
	}
	return ""
}

func (in *MCPServerInstance) FieldNames() []string {
	return []string{
		"spec.userID",
		"spec.mcpServerName",
		"spec.mcpCatalogName",
		"spec.mcpServerCatalogEntryName",
		"spec.powerUserWorkspaceID",
		"spec.template",
		"spec.compositeName",
	}
}

func (in *MCPServerInstance) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &MCPServer{}, Name: in.Spec.MCPServerName},
		{ObjType: &PowerUserWorkspace{}, Name: in.Spec.PowerUserWorkspaceID},
	}
}

type MCPServerInstanceSpec struct {
	// UserID is the user that owns this MCP server instance.
	UserID string `json:"userID,omitempty"`
	// MCPServerName is the name of the MCP server this instance is associated with.
	MCPServerName string `json:"mcpServerName,omitempty"`
	// MCPCatalogName is the name of the MCP catalog that the server that this instance points to is shared within
	MCPCatalogName string `json:"mcpCatalogName,omitempty"`
	// MCPServerCatalogEntryName is the name of the MCP server catalog entry that the server that this instance points to is based on, if there is one.
	MCPServerCatalogEntryName string `json:"mcpServerCatalogEntryName,omitempty"`
	// PowerUserWorkspaceID is the name of the PowerUserWorkspace that the server that this instance points to is owned by, if there is one.
	PowerUserWorkspaceID string `json:"powerUserWorkspaceID,omitempty"`
	// Template indicates whether this MCP server instance is a template instance.
	// Template instances are hidden from user views and are used for creating copyable MCP server instances.
	Template bool `json:"template,omitempty"`
	// CompositeName is the name of the composite MCP server that this MCP server instance is a component of, if there is one.
	CompositeName string `json:"compositeName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPServerInstance `json:"items"`
}
