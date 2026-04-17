package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*PublishedArtifact)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PublishedArtifact struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PublishedArtifactSpec   `json:"spec,omitempty"`
	Status PublishedArtifactStatus `json:"status,omitempty"`
}

func (in *PublishedArtifact) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *PublishedArtifact) Get(field string) (value string) {
	switch field {
	case "spec.authorID":
		return in.Spec.AuthorID
	case "spec.artifactType":
		return string(in.Spec.ArtifactType)
	}
	return ""
}

func (in *PublishedArtifact) FieldNames() []string {
	return []string{"spec.authorID", "spec.artifactType"}
}

func (*PublishedArtifact) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Skill Name", "Spec.Name"},
		{"Type", "Spec.ArtifactType"},
		{"Author", "Spec.AuthorID"},
		{"Version", "Spec.LatestVersion"},
		{"Versions", "{{len .Status.Versions}}"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type PublishedArtifactSpec struct {
	types.PublishedArtifactManifest `json:",inline"`

	// AuthorID is the user ID of the artifact's creator (extracted from auth token).
	AuthorID string `json:"authorID,omitempty"`

	// LatestVersion is the current highest version number for this artifact.
	LatestVersion int `json:"latestVersion,omitempty"`

	// LegacyVisibility is retained only so old stored artifacts can be migrated.
	LegacyVisibility string `json:"visibility,omitempty"`

	// BlobKey is the S3 path to the latest version's ZIP blob.
	// Convention: published-artifacts/{id}/v{N}.zip
	BlobKey string `json:"blobKey,omitempty"`
}

type PublishedArtifactStatus struct {
	// Versions tracks metadata for each published version.
	Versions []types.PublishedArtifactVersionEntry `json:"versions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PublishedArtifactList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PublishedArtifact `json:"items"`
}
