package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemMCPCatalogSpec   `json:"spec,omitempty"`
	Status SystemMCPCatalogStatus `json:"status,omitempty"`
}

type SystemMCPCatalogSpec struct {
	DisplayName string   `json:"displayName,omitempty"`
	SourceURLs  []string `json:"sourceURLs,omitempty"`
}

type SystemMCPCatalogStatus struct {
	LastSyncTime metav1.Time       `json:"lastSyncTime,omitzero"`
	SyncErrors   map[string]string `json:"syncErrors,omitempty"`
	IsSyncing    bool              `json:"isSyncing,omitempty"`
}

func (in *SystemMCPCatalog) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Source URLs", "Spec.SourceURLs"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SystemMCPCatalog `json:"items"`
}
