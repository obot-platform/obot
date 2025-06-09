package v1

import (
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPCatalogSpec   `json:"spec,omitempty"`
	Status MCPCatalogStatus `json:"status,omitempty"`
}

type MCPCatalogSpec struct {
	DisplayName    string   `json:"displayName,omitempty"`
	SourceURLs     []string `json:"sourceURLs,omitempty"`
	AllowedUserIDs []string `json:"allowedUserIDs,omitempty"`
	IsReadOnly     bool     `json:"isReadOnly,omitempty"`
}

type MCPCatalogStatus struct {
	LastSyncTime metav1.Time `json:"lastSyncTime,omitzero"`
}

func (in *MCPCatalog) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Source URLs", "Spec.SourceURLs"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}

func (in *MCPCatalog) Get(field string) string {
	switch field {
	case "spec.sourceURLs":
		return strings.Join(in.Spec.SourceURLs, "\n")
	}
	return ""
}

func (in *MCPCatalog) FieldNames() []string {
	return []string{"spec.sourceURLs"}
}

func (in *MCPCatalog) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPCatalog `json:"items"`
}
