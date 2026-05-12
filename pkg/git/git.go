package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/obot-platform/obot/logger"
)

const maxRepoSizeMB = 100

var errRepoTooLarge = errors.New("repository too large")

var log = logger.Package()

type cloneAuthAttempt struct {
	name  string
	token string
}

type cloneRefAttempt struct {
	name          string
	referenceName plumbing.ReferenceName
	checkoutHash  string
	depth         int
}

func cloneAuthAttempts(token, fallbackToken string) []cloneAuthAttempt {
	if token != "" {
		return []cloneAuthAttempt{{name: "token", token: token}}
	}
	if fallbackToken != "" {
		return []cloneAuthAttempt{
			{name: "anonymous"},
			{name: "fallback token", token: fallbackToken},
		}
	}
	return []cloneAuthAttempt{{name: "anonymous"}}
}

// IsGitRepoURL returns true if the URL points to a git repository on a known
// hosting platform (GitHub, GitLab) or ends with ".git".
func IsGitRepoURL(repoURL string) bool {
	u, err := url.Parse(repoURL)
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

// ResolveToken returns token if non-empty, otherwise falls back to GITHUB_AUTH_TOKEN env var.
func ResolveToken(token string) string {
	if token != "" {
		return token
	}
	return os.Getenv("GITHUB_AUTH_TOKEN")
}

// Clone clones a git repository over HTTPS into a temporary directory.
// Returns the directory path, resolved HEAD commit SHA, a cleanup function, and any error.
//
// The repoURL may embed a branch via the path (e.g. github.com/org/repo.git/mybranch).
// If ref is non-empty it overrides any branch embedded in the URL.
// If token is empty, Clone tries anonymous clone before retrying with GITHUB_AUTH_TOKEN.
func Clone(ctx context.Context, repoURL, token, ref string) (dir string, commitSHA string, cleanup func(), err error) {
	if strings.HasPrefix(repoURL, "http://") {
		return "", "", nil, fmt.Errorf("only HTTPS is supported for git repositories")
	}
	if !strings.HasPrefix(repoURL, "https://") {
		repoURL = "https://" + repoURL
	}

	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid git URL: %w", err)
	}

	cloneURL, urlBranch, err := parseGitURL(repoURL)
	if err != nil {
		return "", "", nil, err
	}

	resolvedRef := ref
	if resolvedRef == "" {
		resolvedRef = urlBranch
	} else if err := validateRef(resolvedRef); err != nil {
		return "", "", nil, err
	}

	fallbackToken := os.Getenv("GITHUB_AUTH_TOKEN")
	apiToken := token
	if apiToken == "" {
		apiToken = fallbackToken
	}

	// Platform API pre-clone size checks: faster and more accurate than waiting
	// for the clone to start. The sizeLimitedFS below acts as a hard fallback.
	repoPath := strings.TrimPrefix(cloneURL, "https://"+u.Host+"/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	switch u.Host {
	case "github.com":
		parts := strings.SplitN(repoPath, "/", 2)
		if len(parts) == 2 {
			if err := checkGitHubRepoSize(ctx, parts[0], parts[1], maxRepoSizeMB, apiToken); err != nil {
				if errors.Is(err, errRepoTooLarge) || isContextError(err) {
					return "", "", nil, fmt.Errorf("repository size check failed: %w", err)
				}
				log.Warnf("GitHub repository size check failed; continuing with clone-time size limit: repo=%s error=%v", repoPath, err)
			}
		}
	case "gitlab.com":
		if err := checkGitLabRepoSize(ctx, u.Host, repoPath, maxRepoSizeMB, token); err != nil {
			if errors.Is(err, errRepoTooLarge) || isContextError(err) {
				return "", "", nil, fmt.Errorf("repository size check failed: %w", err)
			}
			log.Warnf("GitLab repository size check failed; continuing with clone-time size limit: repo=%s error=%v", repoPath, err)
		}
	}

	parentDir, err := os.MkdirTemp("", "git-clone-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	cleanupFn := func() { _ = os.RemoveAll(parentDir) }

	attempts := cloneAuthAttempts(token, fallbackToken)
	refAttempts := cloneRefAttempts(resolvedRef, ref != "")
	attemptErrs := make([]error, 0, len(attempts)*len(refAttempts))
	for _, attempt := range attempts {
		for _, refAttempt := range refAttempts {
			tempDir, err := os.MkdirTemp(parentDir, "clone-*")
			if err != nil {
				cleanupFn()
				return "", "", nil, fmt.Errorf("failed to create temporary directory: %w", err)
			}

			cloneOptions := &gogit.CloneOptions{
				URL:           cloneURL,
				Depth:         refAttempt.depth,
				ReferenceName: refAttempt.referenceName,
			}

			if attempt.token != "" {
				cloneOptions.Auth = &githttp.BasicAuth{
					Username: "x-access-token", // Accepted as a dummy username by GitHub and GitLab.
					Password: attempt.token,
				}
			}

			limitedFS := &sizeLimitedFS{
				Filesystem: osfs.New(tempDir),
				maxBytes:   maxRepoSizeMB * 1024 * 1024,
			}
			storer := gitfs.NewStorage(chroot.New(limitedFS, ".git"), cache.NewObjectLRUDefault())

			clonedRepo, cloneErr := gogit.CloneContext(ctx, storer, limitedFS, cloneOptions)
			if cloneErr != nil {
				attemptErrs = append(attemptErrs, fmt.Errorf("%s %s: %w", attempt.name, refAttempt.name, cloneErr))
				if errors.Is(cloneErr, errRepoTooLarge) {
					cleanupFn()
					return "", "", nil, fmt.Errorf("repository is too large (limit: %d MB)", maxRepoSizeMB)
				}
				if isContextError(cloneErr) {
					cleanupFn()
					return "", "", nil, fmt.Errorf("failed to clone repository: %w", cloneErr)
				}
				continue
			}

			if refAttempt.checkoutHash != "" {
				worktree, err := clonedRepo.Worktree()
				if err != nil {
					cleanupFn()
					return "", "", nil, fmt.Errorf("failed to open worktree: %w", err)
				}
				if err := worktree.Checkout(&gogit.CheckoutOptions{Hash: plumbing.NewHash(refAttempt.checkoutHash)}); err != nil {
					attemptErrs = append(attemptErrs, fmt.Errorf("%s %s: %w", attempt.name, refAttempt.name, err))
					continue
				}
			}

			head, err := clonedRepo.Head()
			if err != nil {
				cleanupFn()
				return "", "", nil, fmt.Errorf("failed to resolve HEAD: %w", err)
			}

			return tempDir, head.Hash().String(), cleanupFn, nil
		}
	}

	cleanupFn()
	return "", "", nil, fmt.Errorf("failed to clone repository after %d attempt(s): %w", len(attemptErrs), errors.Join(attemptErrs...))
}

// parseGitURL parses a git repository URL and returns the clone URL and branch.
// It supports subgroups (e.g. gitlab.com/group/subgroup/repo.git) by using the
// .git suffix as the repo boundary. For GitHub and GitLab, URLs without a .git
// suffix are also accepted for backward compatibility.
// Returns (cloneURL, branch, error).
func parseGitURL(repoURL string) (string, string, error) {
	u, err := url.Parse(repoURL)
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

func validateBranchName(branch string) error {
	return validateRef(branch)
}

func validateRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref name cannot be empty")
	}
	if strings.TrimSpace(ref) != ref || strings.ContainsAny(ref, " \t\n\r") || strings.Contains(ref, "..") || strings.Contains(ref, "\\") ||
		strings.Contains(ref, ":") || strings.HasPrefix(ref, "-") {
		return fmt.Errorf("invalid ref name: %s", ref)
	}
	return nil
}

func cloneRefAttempts(ref string, explicit bool) []cloneRefAttempt {
	if isFullCommitSHA(ref) {
		return []cloneRefAttempt{{name: "commit", checkoutHash: ref}}
	}

	attempts := []cloneRefAttempt{{
		name:          "branch",
		referenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", ref)),
		depth:         1,
	}}
	if explicit {
		attempts = append(attempts, cloneRefAttempt{
			name:          "tag",
			referenceName: plumbing.ReferenceName(fmt.Sprintf("refs/tags/%s", ref)),
			depth:         1,
		})
	}
	return attempts
}

func isFullCommitSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, c := range ref {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// GitHubRepoInfo represents repository information from the GitHub API.
type GitHubRepoInfo struct {
	Size int `json:"size"` // Size in KB
}

// checkGitHubRepoSize checks repo size via the GitHub API before cloning.
func checkGitHubRepoSize(ctx context.Context, org, repo string, maxSizeMB int, token string) error {
	if org == "obot-platform" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", org, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create API request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch repository info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("GitHub API returned status %d for repository %s/%s - %s", resp.StatusCode, org, repo, string(body))
		}
		return fmt.Errorf("GitHub API returned status %d for %s/%s", resp.StatusCode, org, repo)
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

// checkGitLabRepoSize checks repo size via the GitLab API before cloning.
// Skipped if no token is provided since the statistics endpoint requires authentication.
func checkGitLabRepoSize(ctx context.Context, host, projectPath string, maxSizeMB int, token string) error {
	if token == "" {
		return nil // statistics endpoint requires auth; rely on the clone-time size check
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

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil // token lacks statistics scope; fall back to the clone-time size check
	}
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
