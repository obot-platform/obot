package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ DeleteRefs    = (*SystemMCPServerCatalogEntry)(nil)
	_ fields.Fields = (*SystemMCPServerCatalogEntry)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServerCatalogEntry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemMCPServerCatalogEntrySpec   `json:"spec,omitempty"`
	Status SystemMCPServerCatalogEntryStatus `json:"status,omitempty"`
}

func (in *SystemMCPServerCatalogEntry) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"System MCP Catalog", "Spec.SystemMCPCatalogName"},
		{"System MCP Server Type", "Spec.Manifest.SystemMCPServerType"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *SystemMCPServerCatalogEntry) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *SystemMCPServerCatalogEntry) Get(field string) string {
	switch field {
	case "spec.systemMCPCatalogName":
		return in.Spec.SystemMCPCatalogName
	case "spec.manifest.systemMCPServerType":
		return string(in.Spec.Manifest.SystemMCPServerType)
	}
	return ""
}

func (in *SystemMCPServerCatalogEntry) FieldNames() []string {
	return []string{
		"spec.systemMCPCatalogName",
		"spec.manifest.systemMCPServerType",
	}
}

func (in *SystemMCPServerCatalogEntry) DeleteRefs() []Ref {
	return []Ref{{ObjType: &SystemMCPCatalog{}, Name: in.Spec.SystemMCPCatalogName}}
}

type SystemMCPServerCatalogEntrySpec struct {
	Manifest             types.SystemMCPServerCatalogEntryManifest `json:"manifest,omitempty"`
	SourceURL            string                                    `json:"sourceURL,omitempty"`
	Editable             bool                                      `json:"editable,omitempty"`
	SystemMCPCatalogName string                                    `json:"systemMCPCatalogName,omitempty"`
}

type SystemMCPServerCatalogEntryStatus struct {
	LastUpdated               *metav1.Time `json:"lastUpdated,omitempty"`
	ToolPreviewsLastGenerated *metav1.Time `json:"toolPreviewsLastGenerated,omitempty"`
	ManifestHash              string       `json:"manifestHash,omitempty"`
	NeedsUpdate               bool         `json:"needsUpdate,omitempty"`
	OAuthCredentialConfigured bool         `json:"oauthCredentialConfigured,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServerCatalogEntryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SystemMCPServerCatalogEntry `json:"items"`
}
