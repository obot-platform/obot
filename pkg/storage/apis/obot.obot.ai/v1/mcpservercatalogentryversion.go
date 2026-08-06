package v1

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ DeleteRefs    = (*MCPServerCatalogEntryVersion)(nil)
	_ fields.Fields = (*MCPServerCatalogEntryVersion)(nil)
)

func MCPServerCatalogEntryVersionName(entryName string, version int) string {
	return name.SafeHashConcatName(entryName, fmt.Sprintf("v%d", version))
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntryVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MCPServerCatalogEntryVersionSpec `json:"spec,omitempty"`
}

func (in *MCPServerCatalogEntryVersion) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Catalog Entry", "Spec.MCPServerCatalogEntryName"},
		{"Version", "Spec.Version"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *MCPServerCatalogEntryVersion) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPServerCatalogEntryVersion) Get(field string) string {
	switch field {
	case "spec.mcpServerCatalogEntryName":
		return in.Spec.MCPServerCatalogEntryName
	case "spec.version":
		return strconv.Itoa(in.Spec.Version)
	case "spec.sourceURL":
		return in.Spec.SourceURL
	case "spec.active":
		return strconv.FormatBool(in.Spec.Active)
	}
	return ""
}

func (in *MCPServerCatalogEntryVersion) FieldNames() []string {
	return []string{
		"spec.mcpServerCatalogEntryName",
		"spec.version",
		"spec.sourceURL",
		"spec.active",
	}
}

func (in *MCPServerCatalogEntryVersion) DeleteRefs() []Ref {
	return []Ref{{ObjType: &MCPServerCatalogEntry{}, Name: in.Spec.MCPServerCatalogEntryName}}
}

type MCPServerCatalogEntryVersionSpec struct {
	MCPServerCatalogEntryName string                              `json:"mcpServerCatalogEntryName"`
	Version                   int                                 `json:"version"`
	Manifest                  types.MCPServerCatalogEntryManifest `json:"manifest"`
	UnsupportedTools          []string                            `json:"unsupportedTools,omitempty"`
	SourceURL                 string                              `json:"sourceURL,omitempty"`
	// Active is false for source versions retained only because a deployment still references them.
	Active bool `json:"active"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntryVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPServerCatalogEntryVersion `json:"items"`
}
