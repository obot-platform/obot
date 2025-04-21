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
	}
	return ""
}

func (in *MCPServer) FieldNames() []string {
	return []string{
		"spec.threadName",
	}
}

func (in *MCPServer) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
		{ObjType: &ToolReference{}, Name: in.Spec.ToolReferenceName},
	}
}

type MCPServerSpec struct {
	Manifest types.MCPServerManifest `json:"manifest,omitempty"`
	// The project or thread that owns this server.
	ThreadName                string `json:"threadName,omitempty"`
	UserID                    string `json:"userID,omitempty"`
	MCPServerCatalogEntryName string `json:"mcpServerCatalogEntryName,omitempty"`
	ToolReferenceName         string `json:"toolReferenceName,omitempty"`
}

type MCPServerStatus struct {
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
