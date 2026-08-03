// Package agentcatalog holds the server-side rules for an agent config source.
//
// The manifest validates its own shape, in apiclient/types, where the UI can
// apply the same rules before submitting. What lives here is the part that
// depends on how this server is running rather than on the data: a repository
// on the local filesystem is a reasonable source when a developer is iterating
// on a catalog, and is not one anywhere else. That is Obot's policy, not a
// property of the manifest, so a client has no business knowing it.
package agentcatalog

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// Validate applies the manifest's own rules, additionally accepting a
// repository on the local filesystem when allowLocalRepos is set -- which
// callers must derive from Obot's development mode.
func Validate(manifest types.AgentCatalogManifest, allowLocalRepos bool) error {
	if !allowLocalRepos || !isLocalRepo(manifest.RepoURL) {
		return manifest.Validate()
	}

	// The shape checks still apply; only the URL rule is relaxed.
	if strings.TrimSpace(manifest.DisplayName) == "" {
		return fmt.Errorf("displayName is required")
	}
	if err := validateLocalRepoURL(manifest.RepoURL); err != nil {
		return err
	}
	// A local checkout is still cloned by ref, so relaxing where the repository
	// may live must not relax what may be passed to git alongside it.
	return types.ValidateGitRef(manifest.Ref)
}

// isLocalRepo reports whether a value is asking to be read off this machine, so
// that anything else keeps failing with the ordinary error rather than the
// local-path one.
func isLocalRepo(repoURL string) bool {
	repoURL = strings.TrimSpace(repoURL)
	if filepath.IsAbs(repoURL) {
		return true
	}
	u, err := url.Parse(repoURL)
	return err == nil && u.Scheme == "file"
}

// validateLocalRepoURL accepts an absolute filesystem path or a file:// URL.
// Relative paths stay invalid: their meaning would depend on the server
// process's working directory, which is not something a catalog should rest on.
func validateLocalRepoURL(repoURL string) error {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return fmt.Errorf("repoURL is required")
	}
	if filepath.IsAbs(repoURL) {
		return nil
	}

	u, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("invalid repoURL: %v", err)
	}
	if u.Host != "" && u.Host != "localhost" {
		return fmt.Errorf("file repoURL host must be empty or localhost")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("file repoURL must not include user information, a query, or a fragment")
	}
	if !filepath.IsAbs(u.Path) {
		return fmt.Errorf("file repoURL must contain an absolute path")
	}
	return nil
}
