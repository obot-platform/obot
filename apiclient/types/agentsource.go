package types

import (
	"fmt"
	"net/url"
	"strings"
)

// AgentSource is a git repository that hosted agents and harnesses are
// discovered from, mirroring SkillRepository for skills.
type AgentSource struct {
	Metadata               `json:",inline"`
	AgentSourceManifest    `json:",inline"`
	LastSyncTime           Time   `json:"lastSyncTime,omitzero"`
	IsSyncing              bool   `json:"isSyncing,omitempty"`
	SyncError              string `json:"syncError,omitempty"`
	ResolvedCommitSHA      string `json:"resolvedCommitSHA,omitempty"`
	DiscoveredAgentCount   int    `json:"discoveredAgentCount"`
	DiscoveredHarnessCount int    `json:"discoveredHarnessCount"`
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
	return validateRepoURL("repoURL", repoURL)
}

// ValidateGitRepoURL applies the same rule to a hosted agent's git repository,
// whether set by an admin on the agent or by a user on an instance.
func ValidateGitRepoURL(repoURL string) error {
	return validateRepoURL("gitRepo", repoURL)
}

func validateRepoURL(field, repoURL string) error {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return fmt.Errorf("invalid %s: %v", field, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%s must be an https URL", field)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must include a host", field)
	}
	if strings.Trim(u.Path, "/") == "" {
		return fmt.Errorf("%s must include a repository path", field)
	}
	return nil
}

type AgentSourceList List[AgentSource]
