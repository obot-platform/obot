package skillrepository

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	gitpkg "github.com/obot-platform/obot/pkg/git"
)

type fetchedRepository struct {
	RepoRoot  string
	CommitSHA string
	cleanup   func()
}

func (f *fetchedRepository) Cleanup() {
	if f != nil && f.cleanup != nil {
		f.cleanup()
	}
}

// ValidateRepositoryURL returns an error if repoURL is not a valid HTTPS git repository URL.
func ValidateRepositoryURL(repoURL string) error {
	u, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("invalid repository URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("repository URL must use HTTPS")
	}
	if !gitpkg.IsGitRepoURL(repoURL) {
		return fmt.Errorf("repository URL does not appear to be a git repository")
	}
	return nil
}

func safeJoinWithin(baseDir, relPath string) (string, error) {
	cleanPath := path.Clean(filepath.ToSlash(relPath))
	if cleanPath == "." || cleanPath == "" {
		return baseDir, nil
	}
	if strings.HasPrefix(cleanPath, "../") || path.IsAbs(cleanPath) {
		return "", fmt.Errorf("path %q escapes the repository root", relPath)
	}

	joined := filepath.Join(baseDir, filepath.FromSlash(cleanPath))
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	if absJoined != absBase && !strings.HasPrefix(absJoined, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the repository root", relPath)
	}

	return absJoined, nil
}
