package v1

import (
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
	DisplayName string   `json:"displayName,omitempty"`
	SourceURLs  []string `json:"sourceURLs,omitempty"`
}

type MCPCatalogStatus struct {
	LastSyncTime metav1.Time `json:"lastSyncTime,omitzero"`
	// SyncErrors is a map of source URLs to the error encountered while syncing it, if any.
	SyncErrors map[string]string `json:"syncErrors,omitempty"`
	IsSyncing  bool              `json:"isSyncing,omitempty"`
}

func (in *MCPCatalog) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Source URLs", "Spec.SourceURLs"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPCatalog `json:"items"`
}
