package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServerSources struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemMCPServerSourcesSpec   `json:"spec,omitempty"`
	Status SystemMCPServerSourcesStatus `json:"status,omitempty"`
}

type SystemMCPServerSourcesSpec struct {
	SourceURLs []string `json:"sourceURLs,omitempty"`
}

type SystemMCPServerSourcesStatus struct {
	LastSyncTime metav1.Time       `json:"lastSyncTime,omitzero"`
	SyncErrors   map[string]string `json:"syncErrors,omitempty"`
	IsSyncing    bool              `json:"isSyncing,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServerSourcesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SystemMCPServerSources `json:"items"`
}
