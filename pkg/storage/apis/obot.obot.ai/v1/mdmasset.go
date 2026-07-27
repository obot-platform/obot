package v1

import (
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MDMAssetSource is the singleton desired source reconciled into immutable
// MDMAsset snapshots.
type MDMAssetSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MDMAssetSourceSpec   `json:"spec,omitempty"`
	Status MDMAssetSourceStatus `json:"status,omitempty"`
}

type MDMAssetSourceSpec struct {
	types.MDMAssetSourceManifest `json:",inline"`
}

type MDMAssetSourceStatus struct {
	LastSyncTime metav1.Time `json:"lastSyncTime,omitzero"`
	SyncError    string      `json:"syncError,omitempty"`
	LatestDigest string      `json:"latestDigest,omitempty"`
}

func (in *MDMAssetSource) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Source", "Spec.Source"},
		{"Latest Digest", "Status.LatestDigest"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MDMAssetSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MDMAssetSource `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MDMAsset is the queryable metadata for one immutable validated bundle. The
// canonical archive bytes are stored by digest in the gateway database because
// storage resources pass through the Kubernetes API request-size limit.
type MDMAsset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MDMAssetSpec `json:"spec,omitempty"`
	Status EmptyStatus  `json:"status,omitempty"`
}

// MDMAssetSpec mirrors types.MDMAssetManifest field-for-field, except Fields
// is a runtime.RawExtension: a json.RawMessage marshals as raw JSON but is
// typed as a base64 string in the generated OpenAPI schema, which breaks the
// API server's managedFields tracking.
type MDMAssetSpec struct {
	SchemaVersion     string                        `json:"schemaVersion"`
	ObotSentryVersion string                        `json:"obotSentryVersion"`
	Fields            runtime.RawExtension          `json:"fields"`
	Platforms         []types.MDMAssetPlatform      `json:"platforms"`
	Configurations    []types.MDMAssetConfiguration `json:"configurations"`

	Digest string `json:"digest"`
}

func (in *MDMAsset) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Digest", "Spec.Digest"},
		{"ObotSentry Version", "Spec.ObotSentryVersion"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

// MDMAssetName returns the deterministic storage name for a bundle digest.
func MDMAssetName(digest string) string {
	return name.SafeConcatName("mdm-asset", digest)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MDMAssetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MDMAsset `json:"items"`
}
