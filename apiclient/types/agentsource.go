package types

import (
	"fmt"
	"net/url"
	"strings"
)

// AgentSource is a git repository that hosted agents are discovered from,
// mirroring SkillRepository for skills.
type AgentSource struct {
	Metadata             `json:",inline"`
	AgentSourceManifest  `json:",inline"`
	LastSyncTime         Time   `json:"lastSyncTime,omitzero"`
	IsSyncing            bool   `json:"isSyncing,omitempty"`
	SyncError            string `json:"syncError,omitempty"`
	ResolvedCommitSHA    string `json:"resolvedCommitSHA,omitempty"`
	DiscoveredAgentCount int    `json:"discoveredAgentCount"`
}

type AgentSourceManifest struct {
	DisplayName string `json:"displayName,omitempty"`
	RepoURL     string `json:"repoURL,omitempty"`
	Ref         string `json:"ref,omitempty"`
}

func (m AgentSourceManifest) Validate() error {
	if strings.TrimSpace(m.DisplayName) == "" {
		return fmt.Errorf("displayName is required")
	}
	if strings.TrimSpace(m.RepoURL) == "" {
		return fmt.Errorf("repoURL is required")
	}
	return ValidateAgentSourceURL(m.RepoURL)
}

// ValidateAgentSourceURL mirrors the skill repository rule: HTTPS only, with a
// host and a path, so a source cannot be pointed at arbitrary schemes.
func ValidateAgentSourceURL(repoURL string) error {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return fmt.Errorf("invalid repoURL: %v", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("repoURL must be an https URL")
	}
	if u.Host == "" {
		return fmt.Errorf("repoURL must include a host")
	}
	if strings.Trim(u.Path, "/") == "" {
		return fmt.Errorf("repoURL must include a repository path")
	}
	return nil
}

type AgentSourceList List[AgentSource]
