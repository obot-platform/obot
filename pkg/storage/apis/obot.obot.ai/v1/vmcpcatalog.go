package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VMCPCatalogSyncAnnotation = "obot.ai/vmcp-catalog-sync"
	VMCPCatalogFinalizer      = "obot.ai/vmcp-catalog"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCPCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              VMCPCatalogSpec   `json:"spec"`
	Status            VMCPCatalogStatus `json:"status"`
}

type VMCPCatalogSpec struct {
	DisplayName               string            `json:"displayName,omitempty"`
	SourceURLs                []string          `json:"sourceURLs,omitempty"`
	SourceURLGitCredentialIDs map[string]string `json:"sourceURLGitCredentialIDs,omitempty"`
}

type VMCPCatalogStatus struct {
	LastSyncTime metav1.Time       `json:"lastSyncTime,omitzero"`
	SyncErrors   map[string]string `json:"syncErrors,omitempty"`
	IsSyncing    bool              `json:"isSyncing,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCPCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VMCPCatalog `json:"items"`
}

func (in *VMCPCatalog) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Source URLs", "Spec.SourceURLs"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}
