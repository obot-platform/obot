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

func isGitHubURL(catalogURL string) bool {
	u, err := url.Parse(catalogURL)
	return err == nil && u.Host == "github.com"
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

func readGitHubCatalog(catalogURL string) ([]types.MCPServerCatalogEntryManifest, error) {
	// Make sure we don't use plain HTTP
	if strings.HasPrefix(catalogURL, "http://") {
		return nil, fmt.Errorf("only HTTPS is supported for GitHub catalogs")
	}

	// Normalize the URL to ensure HTTPS
	if !strings.HasPrefix(catalogURL, "https://") {
		catalogURL = "https://" + catalogURL
	}

	// Parse URL to ensure it's valid
	u, err := url.Parse(catalogURL)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub URL: %w", err)
	}

	// Should not be possible, but check anyway.
	if u.Host != "github.com" {
		return nil, fmt.Errorf("not a GitHub URL: %s", catalogURL)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URL format, expected github.com/org/repo")
	}
	org, repo := parts[0], parts[1]
	branch := "main"
	if len(parts) > 2 {
		branch = strings.Join(parts[2:], "/")
		// Validate branch name for security
		if err := validateBranchName(branch); err != nil {
			return nil, fmt.Errorf("invalid branch name: %w", err)
		}
	}

	// Check repository size before cloning (limit to 100 MB)
	const maxRepoSizeMB = 100
	if err := checkRepoSize(org, repo, maxRepoSizeMB); err != nil {
		return nil, fmt.Errorf("repository size check failed: %w", err)
	}

	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "catalog-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the repository
	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", org, repo)

	// Set up clone options
	cloneOptions := &git.CloneOptions{
		URL:           cloneURL,
		Depth:         1,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
	}

	// Set up git credentials if token is available
	if githubToken != "" {
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
