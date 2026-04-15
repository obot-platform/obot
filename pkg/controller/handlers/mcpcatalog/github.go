package mcpcatalog

import (
	"context"
	"encoding/json"
	"errors"
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

var errRepoTooLarge = errors.New("repository too large")

// isGitRepoURL returns true if the URL points to a git repository on a known
// hosting platform (GitHub, GitLab) or ends with ".git".
func isGitRepoURL(catalogURL string) bool {
	u, err := url.Parse(catalogURL)
	if err != nil {
		return false
	}
	switch u.Host {
	case "github.com", "gitlab.com":
		return true
	}
	// Treat any HTTPS URL that contains ".git" as a path segment boundary as a git repo
	// (e.g. /org/repo.git or /org/repo.git/branch).
	p := strings.TrimSuffix(u.Path, "/")
	return strings.HasSuffix(p, ".git") || strings.Contains(p, ".git/")
}

// checkGitHubRepoSize checks repo size via the GitHub API before cloning.
// Falls back to the GITHUB_AUTH_TOKEN env var if no per-URL token is provided.
func checkGitHubRepoSize(ctx context.Context, org, repo string, maxSizeMB int, token string) error {
	if org == "obot-platform" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", org, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create API request: %w", err)
	}

	effectiveToken := token
	if effectiveToken == "" {
		effectiveToken = githubToken
	}
	if effectiveToken != "" {
		req.Header.Set("Authorization", "Bearer "+effectiveToken)
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch repository info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("GitHub API returned status %d for %s/%s: %s", resp.StatusCode, org, repo, body)
		}
		return fmt.Errorf("GitHub API returned status %d for %s/%s", resp.StatusCode, org, repo)
	}

	var info struct {
		Size int `json:"size"` // kilobytes
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("failed to parse repository info: %w", err)
	}

	if sizeMB := info.Size / 1024; sizeMB > maxSizeMB {
		return fmt.Errorf("repository %s/%s is too large: %d MB (limit: %d MB)", org, repo, sizeMB, maxSizeMB)
	}
	return nil
}

// checkGitLabRepoSize checks repo size via the GitLab API before cloning.
// Only called when a per-URL token is available; skipped otherwise since the
// statistics endpoint requires authentication.
func checkGitLabRepoSize(ctx context.Context, host, projectPath string, maxSizeMB int, token string) error {
	if token == "" {
		return nil // statistics endpoint requires auth; skip and rely on the clone-time check
	}

	// GitLab expects the project path URL-encoded (e.g. "group%2Fsubgroup%2Frepo")
	apiURL := fmt.Sprintf("https://%s/api/v4/projects/%s?statistics=true", host, url.PathEscape(projectPath))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create API request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch repository info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("GitLab API returned status %d for %s: %s", resp.StatusCode, projectPath, body)
		}
		return fmt.Errorf("GitLab API returned status %d for %s", resp.StatusCode, projectPath)
	}

	var info struct {
		Statistics struct {
			RepositorySize int64 `json:"repository_size"` // bytes
		} `json:"statistics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("failed to parse repository info: %w", err)
	}

	if sizeMB := info.Statistics.RepositorySize / (1024 * 1024); sizeMB > int64(maxSizeMB) {
		return fmt.Errorf("repository %s is too large: %d MB (limit: %d MB)", projectPath, sizeMB, maxSizeMB)
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
// It works with any git hosting platform (GitHub, GitLab, self-hosted, etc.).
// parseGitURL parses a git repository URL and returns the clone URL and branch.
// It supports subgroups (e.g. gitlab.com/group/subgroup/repo.git) by using the
// .git suffix as the repo boundary. For GitHub, URLs without a .git suffix are
// also accepted for backward compatibility.
// Returns (cloneURL, branch, error).
func parseGitURL(catalogURL string) (string, string, error) {
	u, err := url.Parse(catalogURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid git URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid git URL format, expected <host>/org/repo")
	}

	var (
		repoPath string
		branch   string
	)

	// Find the .git boundary to determine where the repo path ends and the branch begins.
	// This handles subgroups (e.g. gitlab.com/group/subgroup/repo.git).
	for i, part := range parts {
		if !strings.HasSuffix(part, ".git") {
			continue
		}

		repoPath = strings.Join(parts[:i+1], "/")
		if i+1 < len(parts) {
			branch = strings.Join(parts[i+1:], "/")
			if err := validateBranchName(branch); err != nil {
				return "", "", fmt.Errorf("invalid branch name: %w", err)
			}
		}
		break
	}

	// For known git hosting platforms, support URLs without .git suffix.
	// The repo is assumed to be at parts[1] (e.g. github.com/org/repo or gitlab.com/org/repo).
	// Subgroups without .git are not supported; use the .git suffix form instead.
	if repoPath == "" {
		switch u.Host {
		case "github.com", "gitlab.com":
			repoPath = strings.Join(parts[:2], "/") + ".git"
			if len(parts) > 2 {
				branch = strings.Join(parts[2:], "/")
				if err := validateBranchName(branch); err != nil {
					return "", "", fmt.Errorf("invalid branch name: %w", err)
				}
			}
		default:
			return "", "", fmt.Errorf("invalid git URL format, URL path must end in .git (e.g. https://%s/org/repo.git)", u.Host)
		}
	}

	if branch == "" {
		branch = "main"
	}

	return fmt.Sprintf("https://%s/%s", u.Host, repoPath), branch, nil
}

func readGitCatalog(ctx context.Context, catalogURL string, token string) ([]types.MCPServerCatalogEntryManifest, error) {
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

	cloneURL, branch, err := parseGitURL(catalogURL)
	if err != nil {
		return nil, err
	}

	// Per-URL token takes precedence over the global GITHUB_AUTH_TOKEN env var.
	effectiveToken := token
	if effectiveToken == "" && u.Host == "github.com" {
		effectiveToken = githubToken
	}

	// Platform API pre-clone size checks: faster and more accurate than waiting
	// for the clone to start. The generic clone-time check below acts as a fallback.
	const maxRepoSizeMB = 100
	repoPath := strings.TrimPrefix(strings.TrimPrefix(cloneURL, "https://"+u.Host+"/"), "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	switch u.Host {
	case "github.com":
		parts := strings.SplitN(repoPath, "/", 2)
		if len(parts) == 2 {
			if err := checkGitHubRepoSize(ctx, parts[0], parts[1], maxRepoSizeMB, effectiveToken); err != nil {
				return nil, fmt.Errorf("repository size check failed: %w", err)
			}
		}
	case "gitlab.com":
		if err := checkGitLabRepoSize(ctx, u.Host, repoPath, maxRepoSizeMB, effectiveToken); err != nil {
			return nil, fmt.Errorf("repository size check failed: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "catalog-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cloneOptions := &git.CloneOptions{
		URL:           cloneURL,
		Depth:         1,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
	}

	if effectiveToken != "" {
		cloneOptions.Auth = &githttp.BasicAuth{
			Username: "x-access-token", // Accepted as a dummy username by GitHub and GitLab.
			Password: effectiveToken,
		}
	}

	// Cancel the clone if the downloaded data exceeds the limit.
	// Polling the temp directory size during the clone works for any git host,
	// and aborts early rather than waiting for the full transfer to complete.
	// Use WithCancelCause so we can distinguish a watcher cancellation from an
	// external cancellation (e.g. controller shutdown).
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go watchDirSize(ctx, cancel, tempDir, maxRepoSizeMB)

	_, err = git.PlainCloneContext(ctx, tempDir, false, cloneOptions)
	if err != nil {
		if context.Cause(ctx) == errRepoTooLarge {
			return nil, fmt.Errorf("repository is too large (limit: %d MB)", maxRepoSizeMB)
		}
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// stop watching dir size before reading directory
	cancel(nil)

	return readMCPCatalogDirectory(tempDir)
}

// watchDirSize polls dir every 200ms and calls cancel when its total size exceeds maxSizeMB.
// It returns when ctx is cancelled. Intended to be run as a goroutine.
func watchDirSize(ctx context.Context, cancel context.CancelCauseFunc, dir string, maxSizeMB int64) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	watchDirSizeTick(ctx, cancel, dir, maxSizeMB, ticker.C)
}

func watchDirSizeTick(ctx context.Context, cancel context.CancelCauseFunc, dir string, maxSizeMB int64, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			if dirSizeMB(dir) > maxSizeMB {
				cancel(errRepoTooLarge)
			}
		}
	}
}

// dirSizeMB returns the total size of all files in dir in megabytes.
// Errors are ignored to prevent unexpected clone cancellations.
func dirSizeMB(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total / (1024 * 1024)
}
