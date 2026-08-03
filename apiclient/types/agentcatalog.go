package types

import (
	"fmt"
	"net/url"
	"strings"
)

// AgentCatalog is a git repository that hosted agents and harnesses are
// discovered from, mirroring SkillRepository for skills.
type AgentCatalog struct {
	Metadata               `json:",inline"`
	AgentCatalogManifest   `json:",inline"`
	LastSyncTime           Time   `json:"lastSyncTime,omitzero"`
	IsSyncing              bool   `json:"isSyncing,omitempty"`
	SyncError              string `json:"syncError,omitempty"`
	ResolvedCommitSHA      string `json:"resolvedCommitSHA,omitempty"`
	DiscoveredAgentCount   int    `json:"discoveredAgentCount"`
	DiscoveredHarnessCount int    `json:"discoveredHarnessCount"`
}

type AgentCatalogManifest struct {
	DisplayName string `json:"displayName,omitempty"`
	RepoURL     string `json:"repoURL,omitempty"`
	Ref         string `json:"ref,omitempty"`
}

func (m AgentCatalogManifest) Validate() error {
	if strings.TrimSpace(m.DisplayName) == "" {
		return fmt.Errorf("displayName is required")
	}
	if strings.TrimSpace(m.RepoURL) == "" {
		return fmt.Errorf("repoURL is required")
	}
	return ValidateAgentCatalogURL(m.RepoURL)
}

// ValidateAgentCatalogURL mirrors the skill repository rule: HTTPS only, with a
// host and a path, so a source cannot be pointed at arbitrary schemes.
func ValidateAgentCatalogURL(repoURL string) error {
	return validateRepoURL("repoURL", repoURL)
}

// ValidateGitRepoURL applies the same rule to a hosted agent's git repository,
// whether set by an admin on the agent or by a user on an instance.
func ValidateGitRepoURL(repoURL string) error {
	return validateRepoURL("gitRepo", repoURL)
}

// ValidateGitRef checks a branch, tag or commit reference.
//
// The value reaches git as an argument, so it is constrained rather than
// escaped: a ref beginning with "-" would be read as an option, and the
// remaining rules keep it to what git itself accepts as a ref name.
func ValidateGitRef(ref string) error {
	if ref == "" {
		return nil
	}
	if len(ref) > 255 {
		return fmt.Errorf("gitRef must be 255 characters or fewer")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("gitRef must not begin with '-'")
	}
	if strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") {
		return fmt.Errorf("gitRef must not begin or end with '/'")
	}
	// ".." is how a ref escapes into a revision range, and git rejects it in a
	// ref name for the same reason.
	if strings.Contains(ref, "..") {
		return fmt.Errorf("gitRef must not contain '..'")
	}
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '/' || r == '+':
		default:
			return fmt.Errorf("gitRef may only contain letters, digits and . _ - / +")
		}
	}
	return nil
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

type AgentCatalogList List[AgentCatalog]
