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

// isGitURL checks if a URL points to a git repository.
// Returns true for GitHub URLs or URLs with .git suffix.
func isGitURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	// GitHub URLs are always git repos, even without .git suffix
	if u.Host == "github.com" {
		return true
	}
	// For other hosts, require .git suffix
	return strings.HasSuffix(u.Path, ".git") || strings.Contains(u.Path, ".git/")
}

// extractCredentials extracts username and password from a URL.
// Returns empty strings if no credentials present.
func extractCredentials(rawURL string) (username, password string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return "", ""
	}
	username = u.User.Username()
	password, _ = u.User.Password()
	return username, password
}

// GitHubRepoInfo represents the repository information from GitHub API
type GitHubRepoInfo struct {
	Size int `json:"size"` // Size in KB
}

// checkRepoSize checks the repository size using GitHub API before cloning
func checkRepoSize(org, repo string, maxSizeMB int) error {
	if org == "obot-platform" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", org, repo)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create API request: %w", err)
	}

	// Add authentication if token is available
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch repository info: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("GitHub API returned status %d for repository %s/%s - %s", resp.StatusCode, org, repo, string(body))
		}
		return fmt.Errorf("GitHub API returned status %d for repository %s/%s", resp.StatusCode, org, repo)
	}

	// Parse response
	var repoInfo GitHubRepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return fmt.Errorf("failed to parse repository info: %w", err)
	}

	// Check size (GitHub API returns size in KB)
	sizeMB := repoInfo.Size / 1024
	if sizeMB > maxSizeMB {
		return fmt.Errorf("repository %s/%s is too large: %d MB (limit: %d MB)", org, repo, sizeMB, maxSizeMB)
	}

	return nil
}

// validateBranchName validates that the branch name doesn't contain suspicious characters
func validateBranchName(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Check for path traversal attempts and other suspicious characters
	if strings.Contains(branch, "..") || strings.Contains(branch, "\\") ||
		strings.Contains(branch, ":") || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name: %s", branch)
	}

	return nil
}

// isPathSafe checks if a file path is safe to read (not a symlink and within bounds)
func isPathSafe(path, baseDir string) error {
	// Check if it's a symbolic link
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Skip symbolic links to prevent path traversal
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic links are not allowed for security reasons")
	}

	// Resolve the absolute path and ensure it's within the base directory
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute base directory: %w", err)
	}

	// Ensure the file is within the base directory
	if !strings.HasPrefix(absPath, absBaseDir+string(filepath.Separator)) {
		return fmt.Errorf("file path is outside the allowed directory")
	}

	return nil
}

// readGitCatalog clones a git repository and reads catalog entries.
// Supports any git host with URL-embedded credentials or GITHUB_AUTH_TOKEN for GitHub.
func readGitCatalog(catalogURL string) ([]types.MCPServerCatalogEntryManifest, error) {
	// Make sure we don't use plain HTTP
	if strings.HasPrefix(catalogURL, "http://") {
		return nil, fmt.Errorf("only HTTPS is supported for Git catalogs")
	}

	// Normalize the URL to ensure HTTPS
	if !strings.HasPrefix(catalogURL, "https://") {
		catalogURL = "https://" + catalogURL
	}

	// Parse URL to ensure it's valid
	u, err := url.Parse(catalogURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Git URL: %w", err)
	}

	// Extract credentials from URL if present
	username, password := extractCredentials(catalogURL)

	// Parse the path to extract repo path and optional branch
	// Supports formats:
	//   - /org/repo (GitHub only, for backward compatibility)
	//   - /org/repo.git
	//   - /org/repo.git/branch/name
	//   - /group/subgroup/repo.git (GitLab nested groups)
	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")

	var repoPathParts []string
	var branch string

	// Look for .git suffix to determine repo boundary
	for i, part := range pathParts {
		if strings.HasSuffix(part, ".git") {
			repoPathParts = pathParts[:i+1]
			if i+1 < len(pathParts) {
				branch = strings.Join(pathParts[i+1:], "/")
				if err := validateBranchName(branch); err != nil {
					return nil, fmt.Errorf("invalid branch name: %w", err)
				}
			}
			break
		}
	}

	// For GitHub, support URLs without .git suffix (backward compatibility)
	// Format: github.com/org/repo or github.com/org/repo/branch
	if len(repoPathParts) == 0 && u.Host == "github.com" {
		if len(pathParts) < 2 {
			return nil, fmt.Errorf("invalid GitHub URL format, expected github.com/org/repo")
		}
		repoPathParts = pathParts[:2]
		// Append .git for cloning
		repoPathParts[1] = repoPathParts[1] + ".git"
		if len(pathParts) > 2 {
			branch = strings.Join(pathParts[2:], "/")
			if err := validateBranchName(branch); err != nil {
				return nil, fmt.Errorf("invalid branch name: %w", err)
			}
		}
	}

	if len(repoPathParts) == 0 {
		return nil, fmt.Errorf("invalid Git URL format, expected URL with path ending in .git")
	}

	if branch == "" {
		branch = "main"
	}

	// GitHub-specific size check (only if GitHub and we have access)
	if u.Host == "github.com" && len(repoPathParts) >= 2 {
		org := repoPathParts[0]
		repo := strings.TrimSuffix(repoPathParts[1], ".git")
		const maxRepoSizeMB = 100
		if err := checkRepoSize(org, repo, maxRepoSizeMB); err != nil {
			return nil, fmt.Errorf("repository size check failed: %w", err)
		}
	}

	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "catalog-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Build clone URL without credentials and without branch path
	cloneURL := &url.URL{
		Scheme: "https",
		Host:   u.Host,
		Path:   "/" + strings.Join(repoPathParts, "/"),
	}

	// Set up clone options
	cloneOptions := &git.CloneOptions{
		URL:           cloneURL.String(),
		Depth:         1,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
	}

	// Set up authentication - prioritize URL-embedded credentials
	if username != "" && password != "" {
		cloneOptions.Auth = &githttp.BasicAuth{
			Username: username,
			Password: password,
		}
	} else if u.Host == "github.com" && githubToken != "" {
		// Fallback to environment token for GitHub
		cloneOptions.Auth = &githttp.BasicAuth{
			Username: "obot", // Use a dummy username. The username is ignored, but required to be non-empty.
			Password: githubToken,
		}
	}

	// Use go-git to clone the repository
	_, err = git.PlainClone(tempDir, false, cloneOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	return readMCPCatalogDirectory(tempDir)
}
