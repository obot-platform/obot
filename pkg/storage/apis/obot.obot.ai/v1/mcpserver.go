package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*MCPServer)(nil)
	_ DeleteRefs    = (*MCPServer)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

func (in *MCPServer) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPServer) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return in.Spec.ThreadName
	case "spec.userID":
		return in.Spec.UserID
	case "spec.mcpServerCatalogEntryName":
		return in.Spec.MCPServerCatalogEntryName
	case "spec.sharedWithinMCPCatalogName":
		return in.Spec.SharedWithinMCPCatalogName
	}
	return ""
}

func (in *MCPServer) FieldNames() []string {
	return []string{
		"spec.threadName",
		"spec.userID",
		"spec.mcpServerCatalogEntryName",
		"spec.sharedWithinMCPCatalogName",
	}
}

func (in *MCPServer) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
		{ObjType: &ToolReference{}, Name: in.Spec.ToolReferenceName},
		{ObjType: &MCPServerCatalogEntry{}, Name: in.Spec.MCPServerCatalogEntryName},
		{ObjType: &MCPCatalog{}, Name: in.Spec.SharedWithinMCPCatalogName},
	}
}

type MCPServerSpec struct {
	Manifest types.MCPServerManifest `json:"manifest,omitempty"`
	// List of tool names that are known to not work well in Obot.
	UnsupportedTools []string `json:"unsupportedTools,omitempty"`
	// ThreadName is the project or thread that owns this server, if there is one.
	ThreadName string `json:"threadName,omitempty"`
	// UserID is the user that created this server.
	UserID string `json:"userID,omitempty"`
	// SharedWithinMCPCatalogName contains the name of the MCPCatalog inside of which this server was directly created by the admin, if there is one.
	SharedWithinMCPCatalogName string `json:"sharedWithinMCPCatalogName,omitempty"`
	// MCPServerCatalogEntryName contains the name of the MCPServerCatalogEntry from which this MCP server was created, if there is one.
	MCPServerCatalogEntryName string `json:"mcpServerCatalogEntryName,omitempty"`
	// ToolReferenceName contains the name of the legacy gptscript tool reference for this MCP server, if there is one.
	ToolReferenceName string `json:"toolReferenceName,omitempty"`
}

type MCPServerStatus struct {
	// NeedsUpdate indicates whether the configuration in the catalog entry has drifted from the server's configuration.
	NeedsUpdate bool `json:"needsUpdate,omitempty"`
}

type MCPServerType string

type MCPServerMetadata struct {
	// A human-readable name for the server.
	Name string `json:"name,omitempty"`
	// A human-readable description of the server.
	Description string `json:"description,omitempty"`
	// The HTTP URL of the server if it is accessible via SSE or HTTP Streaming
	HTTPURL string `json:"httpURL,omitempty"`
	// The GitRepo of the server code
	GitRepo string `json:"gitRepo,omitempty"`
}

type MCPCommand struct {
}

type MCPServerCapabilities struct {
	// Sampling indicates whether the server supports MCP Sampling.
	Sampling bool `json:"sampling,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPServer `json:"items"`
}
