package types

type Skill struct {
	Metadata
	SkillManifest
	RepoID          string `json:"repoID,omitempty"`
	RepoURL         string `json:"repoURL,omitempty"`
	RepoRef         string `json:"repoRef,omitempty"`
	CommitSHA       string `json:"commitSHA,omitempty"`
	RelativePath    string `json:"relativePath,omitempty"`
	InstallHash     string `json:"installHash,omitempty"`
	Valid           bool   `json:"valid,omitempty"`
	ValidationError string `json:"validationError,omitempty"`
	LastIndexedAt   Time   `json:"lastIndexedAt,omitzero"`
}

type SkillManifest struct {
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
	License       string `json:"license,omitempty"`
	Compatibility string `json:"compatibility,omitempty"`
	AllowedTools  string `json:"allowedTools,omitempty"`
	// Tagged metadataValues rather than metadata: Skill embeds both Metadata and
	// SkillManifest, and two promoted fields sharing the "metadata" tag made
	// encoding/json drop both, so skills serialized no metadata at all.
	MetadataValues map[string]string `json:"metadataValues,omitempty"`
}

type SkillList List[Skill]
