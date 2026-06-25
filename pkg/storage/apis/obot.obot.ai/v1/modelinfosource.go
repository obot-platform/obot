package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ModelInfoSource syncs ModelInfo records from an external source.
type ModelInfoSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelInfoSourceSpec   `json:"spec,omitempty"`
	Status ModelInfoSourceStatus `json:"status,omitempty"`
}

type ModelInfoSourceSpec struct {
	Manifest types.ModelInfoSourceManifest `json:"manifest,omitempty"`
}

type ModelInfoSourceStatus struct {
	LastSyncTime metav1.Time `json:"lastSyncTime,omitzero"`
	SyncError    string      `json:"syncError,omitempty"`
	ModelCount   int         `json:"modelCount,omitempty"`
}

func (in *ModelInfoSource) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"URL", "Spec.Manifest.URL"},
		{"Models", "Status.ModelCount"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelInfoSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelInfoSource `json:"items"`
}
