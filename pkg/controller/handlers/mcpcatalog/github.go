package mcpcatalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/obot-platform/obot/apiclient/types"
)

var githubToken = os.Getenv("GITHUB_AUTH_TOKEN")

// GitHubRepoInfo represents the repository information from GitHub API
type GitHubRepoInfo struct {
	Size int `json:"size"` // Size in KB
}

// isGitRepoURL returns true if the URL points to a git repository on a known
// hosting platform (GitHub, GitLab, Bitbucket) or ends with ".git".
func isGitRepoURL(catalogURL string) bool {
	u, err := url.Parse(catalogURL)
	if err != nil {
		return false
	}
	switch u.Host {
	case "github.com", "gitlab.com", "bitbucket.org":
		return true
	}
	// Treat any HTTPS URL whose path ends in .git as a git repo.
	return strings.HasSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
}

// checkGitHubRepoSize checks the repository size using the GitHub API before cloning.
// It is only called for github.com URLs.
func checkGitHubRepoSize(org, repo string, maxSizeMB int, token string) error {
	if org == "obot-platform" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", org, repo)

	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create API request: %w", err)
	}

	// Per-URL token takes precedence over global env var.
	effectiveToken := token
	if effectiveToken == "" {
		effectiveToken = githubToken
	}
	if effectiveToken != "" {
		req.Header.Set("Authorization", "Bearer "+effectiveToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch repository info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("GitHub API returned status %d for repository %s/%s - %s", resp.StatusCode, org, repo, string(body))
		}
		return fmt.Errorf("GitHub API returned status %d for repository %s/%s", resp.StatusCode, org, repo)
	}

	var repoInfo GitHubRepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return fmt.Errorf("failed to parse repository info: %w", err)
	}

	sizeMB := repoInfo.Size / 1024
	if sizeMB > maxSizeMB {
		return fmt.Errorf("repository %s/%s is too large: %d MB (limit: %d MB)", org, repo, sizeMB, maxSizeMB)
	}

	return nil
}

// validateBranchName validates that the branch name doesn't contain suspicious characters.
func validateBranchName(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	if strings.Contains(branch, "..") || strings.Contains(branch, "\\") ||
		strings.Contains(branch, ":") || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name: %s", branch)
	}

	return nil
}

// isPathSafe checks if a file path is safe to read (not a symlink and within bounds).
func isPathSafe(path, baseDir string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic links are not allowed for security reasons")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute base directory: %w", err)
	}

	if !strings.HasPrefix(absPath, absBaseDir+string(filepath.Separator)) {
		return fmt.Errorf("file path is outside the allowed directory")
	}

	return nil
}

// readGitCatalog clones a git repository over HTTPS and reads its catalog entries.
// It works with any git hosting platform (GitHub, GitLab, Bitbucket, self-hosted, etc.).
func readGitCatalog(catalogURL string, token string) ([]types.MCPServerCatalogEntryManifest, error) {
	if strings.HasPrefix(catalogURL, "http://") {
		return nil, fmt.Errorf("only HTTPS is supported for git catalogs")
	}

	if !strings.HasPrefix(catalogURL, "https://") {
		catalogURL = "https://" + catalogURL
	}

	u, err := url.Parse(catalogURL)
	if err != nil {
		return nil, fmt.Errorf("invalid git URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid git URL format, expected <host>/org/repo")
	}
	org, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
	branch := "main"
	if len(parts) > 2 {
		branch = strings.Join(parts[2:], "/")
		if err := validateBranchName(branch); err != nil {
			return nil, fmt.Errorf("invalid branch name: %w", err)
		}
	}

	// The GitHub API allows us to check repo size before cloning; skip this for other hosts.
	if u.Host == "github.com" {
		const maxRepoSizeMB = 100
		if err := checkGitHubRepoSize(org, repo, maxRepoSizeMB, token); err != nil {
			return nil, fmt.Errorf("repository size check failed: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "catalog-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cloneURL := fmt.Sprintf("https://%s/%s/%s.git", u.Host, org, repo)

	cloneOptions := &git.CloneOptions{
		URL:           cloneURL,
		Depth:         1,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
	}

	// Per-URL token takes precedence over the global GITHUB_AUTH_TOKEN env var.
	effectiveToken := token
	if effectiveToken == "" {
		effectiveToken = githubToken
	}
	if effectiveToken != "" {
		cloneOptions.Auth = &githttp.BasicAuth{
			Username: "x-access-token", // Accepted as a dummy username by GitHub, GitLab, and Bitbucket.
			Password: effectiveToken,
		}
	}

	_, err = git.PlainClone(tempDir, false, cloneOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	return readMCPCatalogDirectory(tempDir)
}
