package types

// PublishedArtifactType represents the type of a published artifact.
type PublishedArtifactType string

const (
	PublishedArtifactTypeWorkflow PublishedArtifactType = "workflow"
	PublishedArtifactTypeSkill    PublishedArtifactType = "skill"
)

// PublishedArtifactManifest contains the user/client-editable fields for a published artifact.
type PublishedArtifactManifest struct {
	Name         string                `json:"name"`
	Description  string                `json:"description,omitempty"`
	ArtifactType PublishedArtifactType `json:"artifactType"`
	AuthorEmail  string                `json:"authorEmail,omitempty"`
}

// PublishedArtifact represents a published artifact in the API.
type PublishedArtifact struct {
	Metadata
	PublishedArtifactManifest
	DisplayName   string                            `json:"displayName,omitempty"`
	AuthorID      string                            `json:"authorID"`
	LatestVersion int                               `json:"latestVersion"`
	Versions      []PublishedArtifactVersionSummary `json:"versions,omitempty"`
}

// PublishedArtifactVersionSummary is the public view of a version entry (no internal blob keys).
type PublishedArtifactVersionSummary struct {
	Version     int       `json:"version"`
	Description string    `json:"description,omitempty"`
	CreatedAt   Time      `json:"createdAt"`
	Subjects    []Subject `json:"subjects,omitempty"`
}

// PublishedArtifactList is a list of published artifacts.
type PublishedArtifactList List[PublishedArtifact]

// PublishedArtifactVersionEntry represents metadata for a single version of an artifact.
type PublishedArtifactVersionEntry struct {
	Version     int       `json:"version"`
	BlobKey     string    `json:"blobKey"`
	Description string    `json:"description,omitempty"`
	CreatedAt   Time      `json:"createdAt"`
	Subjects    []Subject `json:"subjects,omitempty"`
}
