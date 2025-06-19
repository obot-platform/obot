package v1

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ DeleteRefs = (*MCPServerCatalogEntry)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerCatalogEntrySpec   `json:"spec,omitempty"`
	Status MCPServerCatalogEntryStatus `json:"status,omitempty"`
}

func (in *MCPServerCatalogEntry) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Tool Reference", "Spec.ToolReferenceName"},
		{"MCP Catalog", "Spec.MCPCatalogName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *MCPServerCatalogEntry) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPServerCatalogEntry) Get(field string) string {
	switch field {
	case "spec.mcpCatalogName":
		return in.Spec.MCPCatalogName
	}
	return ""
}

func (in *MCPServerCatalogEntry) FieldNames() []string {
	return []string{
		"spec.mcpCatalogName",
	}
}

func (in *MCPServerCatalogEntry) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &ToolReference{}, Name: in.Spec.ToolReferenceName},
		{ObjType: &MCPCatalog{}, Name: in.Spec.MCPCatalogName},
	}
}

type MCPServerCatalogEntrySpec struct {
	CommandManifest   types.MCPServerCatalogEntryManifest `json:"commandManifest,omitzero"`
	URLManifest       types.MCPServerCatalogEntryManifest `json:"urlManifest,omitzero"`
	ToolReferenceName string                              `json:"toolReferenceName,omitempty"`
	UnsupportedTools  []string                            `json:"unsupportedTools,omitempty"`
	MCPCatalogName    string                              `json:"mcpCatalogName,omitempty"`
	Editable          bool                                `json:"editable,omitempty"`
	SourceURL         string                              `json:"sourceURL,omitempty"`
}

type MCPServerCatalogEntryStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPServerCatalogEntry `json:"items"`
}
