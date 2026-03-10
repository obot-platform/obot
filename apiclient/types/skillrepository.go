package types

type SkillRepository struct {
	Metadata
	SkillRepositoryManifest
	LastSyncTime         Time   `json:"lastSyncTime,omitzero"`
	IsSyncing            bool   `json:"isSyncing,omitempty"`
	SyncError            string `json:"syncError,omitempty"`
	ResolvedCommitSHA    string `json:"resolvedCommitSHA,omitempty"`
	DiscoveredSkillCount int    `json:"discoveredSkillCount,omitempty"`
}

type SkillRepositoryManifest struct {
	DisplayName string `json:"displayName,omitempty"`
	RepoURL     string `json:"repoURL,omitempty"`
	Ref         string `json:"ref,omitempty"`
}

type SkillRepositoryList List[SkillRepository]
