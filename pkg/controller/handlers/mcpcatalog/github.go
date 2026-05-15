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

	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitfs "github.com/go-git/go-git/v5/storage/filesystem"
)

// GitHubRepoInfo represents the repository information from GitHub API
type GitHubRepoInfo struct {
	Size int `json:"size"` // Size in KB
}

var errRepoTooLarge = errors.New("repository too large")

type gitCloneAuthAttempt struct {
	name  string
	token string
}

func gitCloneAuthAttempts(catalogToken, fallbackToken string) []gitCloneAuthAttempt {
	if catalogToken != "" {
		return []gitCloneAuthAttempt{{name: "catalog token", token: catalogToken}}
	}
	if fallbackToken != "" {
		return []gitCloneAuthAttempt{
			{name: "anonymous"},
			{name: "fallback token", token: fallbackToken},
		}
	}
	return []gitCloneAuthAttempt{{name: "anonymous"}}
}

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
func checkGitHubRepoSize(ctx context.Context, org, repo string, maxSizeMB int, token string) error {
	if org == "obot-platform" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", org, repo)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create API request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

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
		return fmt.Errorf("GitHub API returned status %d for %s/%s", resp.StatusCode, org, repo)
	}

	// Parse response
	var repoInfo GitHubRepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return fmt.Errorf("failed to parse repository info: %w", err)
	}

	// Check size (GitHub API returns size in KB)
	sizeMB := repoInfo.Size / 1024
	if sizeMB > maxSizeMB {
		return fmt.Errorf("%w: repository %s/%s is %d MB (limit: %d MB)", errRepoTooLarge, org, repo, sizeMB, maxSizeMB)
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
		return fmt.Errorf("%w: repository %s is %d MB (limit: %d MB)", errRepoTooLarge, projectPath, sizeMB, maxSizeMB)
	}
	return nil
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func cloneGitCatalog(ctx context.Context, parentDir, cloneURL, ref string, explicitRef bool, maxRepoSizeMB int, attempt gitCloneAuthAttempt) (string, error) {
	refNames := []plumbing.ReferenceName{
		plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", ref)),
	}
	if explicitRef {
		refNames = append(refNames, plumbing.ReferenceName(fmt.Sprintf("refs/tags/%s", ref)))
	}

	var errs []error
	for _, refName := range refNames {
		tempDir, err := os.MkdirTemp(parentDir, "clone-*")
		if err != nil {
			return "", fmt.Errorf("failed to create temporary directory: %w", err)
		}

		cloneOptions := &git.CloneOptions{
			URL:           cloneURL,
			Depth:         1,
			ReferenceName: refName,
		}

		if attempt.token != "" {
			cloneOptions.Auth = &githttp.BasicAuth{
				Username: "x-access-token", // Accepted as a dummy username by GitHub and GitLab.
				Password: attempt.token,
			}
		}

		limitedFS := &sizeLimitedFS{
			Filesystem: osfs.New(tempDir),
			maxBytes:   int64(maxRepoSizeMB) * 1024 * 1024,
		}
		storer := gitfs.NewStorage(chroot.New(limitedFS, ".git"), cache.NewObjectLRUDefault())

		if _, err = git.CloneContext(ctx, storer, limitedFS, cloneOptions); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", refName, err))
			continue
		}

		return tempDir, nil
	}

	return "", errors.Join(errs...)
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
// Returns (cloneURL, ref, explicitRef, error).
func parseGitURL(catalogURL string) (string, string, bool, error) {
	if !strings.Contains(catalogURL, "://") {
		catalogURL = "https://" + catalogURL
	}

	u, err := url.Parse(catalogURL)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid git URL: %w", err)
	}

	path := strings.Trim(u.Path, "/")
	explicitRef := ""
	if i := strings.LastIndex(path, "@"); i >= 0 {
		explicitRef = path[i+1:]
		if err := validateBranchName(explicitRef); err != nil {
			return "", "", false, fmt.Errorf("invalid ref name: %w", err)
		}
		path = strings.TrimSuffix(path[:i], "/")
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", false, fmt.Errorf("invalid git URL format, expected <host>/org/repo")
	}

	var (
		repoPath string
		ref      string
	)

	// Find the .git boundary to determine where the repo path ends and the branch begins.
	// This handles subgroups (e.g. gitlab.com/group/subgroup/repo.git).
	for i, part := range parts {
		if !strings.HasSuffix(part, ".git") {
			continue
		}

		repoPath = strings.Join(parts[:i+1], "/")
		if i+1 < len(parts) {
			ref = strings.Join(parts[i+1:], "/")
			if err := validateBranchName(ref); err != nil {
				return "", "", false, fmt.Errorf("invalid branch name: %w", err)
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
				ref = strings.Join(parts[2:], "/")
				if err := validateBranchName(ref); err != nil {
					return "", "", false, fmt.Errorf("invalid branch name: %w", err)
				}
			}
		default:
			return "", "", false, fmt.Errorf("invalid git URL format, URL path must end in .git (e.g. https://%s/org/repo.git)", u.Host)
		}
	}

	if explicitRef != "" {
		ref = explicitRef
	}

	if ref == "" {
		ref = "main"
	}

	return fmt.Sprintf("https://%s/%s", u.Host, repoPath), ref, explicitRef != "", nil
}

func readGitCatalogEntries[T any](ctx context.Context, catalogURL string, token string) ([]T, error) {
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

	cloneURL, ref, explicitRef, err := parseGitURL(catalogURL)
	if err != nil {
		return nil, err
	}

	fallbackToken := os.Getenv("GITHUB_AUTH_TOKEN")

	// Platform API pre-clone size checks: faster and more accurate than waiting
	// for the clone to start. The generic clone-time check below acts as a fallback.
	const maxRepoSizeMB = 100
	repoPath := strings.TrimPrefix(strings.TrimPrefix(cloneURL, "https://"+u.Host+"/"), "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	switch u.Host {
	case "github.com":
		parts := strings.SplitN(repoPath, "/", 2)
		if len(parts) == 2 {
			apiToken := token
			if apiToken == "" {
				apiToken = fallbackToken
			}
			if err := checkGitHubRepoSize(ctx, parts[0], parts[1], maxRepoSizeMB, apiToken); err != nil {
				if errors.Is(err, errRepoTooLarge) || isContextError(err) {
					return nil, fmt.Errorf("repository size check failed: %w", err)
				}
				log.Warnf("GitHub catalog repository size check failed; continuing with clone-time size limit: repo=%s error=%v", repoPath, err)
			}
		}
	case "gitlab.com":
		if err := checkGitLabRepoSize(ctx, u.Host, repoPath, maxRepoSizeMB, token); err != nil {
			if errors.Is(err, errRepoTooLarge) || isContextError(err) {
				return nil, fmt.Errorf("repository size check failed: %w", err)
			}
			log.Warnf("GitLab catalog repository size check failed; continuing with clone-time size limit: repo=%s error=%v", repoPath, err)
		}
	}

	tempDir, err := os.MkdirTemp("", "catalog-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	attempts := gitCloneAuthAttempts(token, fallbackToken)
	attemptErrs := make([]error, len(attempts))
	for i, attempt := range attempts {
		cloneDir, err := cloneGitCatalog(ctx, tempDir, cloneURL, ref, explicitRef, maxRepoSizeMB, attempt)
		if err == nil {
			return readCatalogDirectory[T](cloneDir)
		}
		attemptErrs[i] = fmt.Errorf("%s: %w", attempt.name, err)
		if errors.Is(err, errRepoTooLarge) {
			return nil, fmt.Errorf("repository is too large (limit: %d MB)", maxRepoSizeMB)
		}
		if isContextError(err) {
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	return nil, fmt.Errorf("failed to clone repository after %d attempt(s): %w", len(attemptErrs), errors.Join(attemptErrs...))
}
