package skillrepository

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxRepoSizeMB       = 100
	maxArchiveBytes     = 100 * 1024 * 1024
	maxExtractedFiles   = 10000
	maxExtractedBytes   = 100 * 1024 * 1024
	maxSkillMDBytes     = 1024 * 1024
	defaultGitHubAPIURL = "https://api.github.com"
)

type githubRepositoryFetcher struct {
	client            *http.Client
	apiBaseURL        string
	token             string
	maxRepoSizeMB     int
	maxArchiveBytes   int64
	maxExtractedFiles int
	maxExtractedBytes int64
}

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

type githubRepository struct {
	Owner string
	Repo  string
}

type githubRepositoryMetadata struct {
	Size          int    `json:"size"`
	DefaultBranch string `json:"default_branch"`
}

type githubCommit struct {
	SHA string `json:"sha"`
}

func newGitHubRepositoryFetcher() *githubRepositoryFetcher {
	return &githubRepositoryFetcher{
		client:            http.DefaultClient,
		apiBaseURL:        defaultGitHubAPIURL,
		token:             os.Getenv("GITHUB_AUTH_TOKEN"),
		maxRepoSizeMB:     maxRepoSizeMB,
		maxArchiveBytes:   maxArchiveBytes,
		maxExtractedFiles: maxExtractedFiles,
		maxExtractedBytes: maxExtractedBytes,
	}
}

func ValidateRepositoryURL(repoURL string) error {
	_, err := parseGitHubRepository(repoURL)
	return err
}

func (f *githubRepositoryFetcher) Fetch(ctx context.Context, repoURL, ref string) (*fetchedRepository, error) {
	repo, err := parseGitHubRepository(repoURL)
	if err != nil {
		return nil, err
	}

	metadata, err := f.getRepositoryMetadata(ctx, repo)
	if err != nil {
		return nil, err
	}
	if metadata.Size/1024 > f.maxRepoSizeMB {
		return nil, fmt.Errorf("repository %s/%s is too large: %d MB (limit: %d MB)", repo.Owner, repo.Repo, metadata.Size/1024, f.maxRepoSizeMB)
	}

	resolvedRef := ref
	if resolvedRef == "" {
		resolvedRef = metadata.DefaultBranch
	}
	if resolvedRef == "" {
		return nil, fmt.Errorf("repository %s/%s does not have a default branch", repo.Owner, repo.Repo)
	}

	commit, err := f.resolveCommitSHA(ctx, repo, resolvedRef)
	if err != nil {
		return nil, err
	}

	return f.downloadArchive(ctx, repo, commit)
}

func (f *githubRepositoryFetcher) MaterializeCommit(ctx context.Context, repoURL, commitSHA string) (*fetchedRepository, error) {
	repo, err := parseGitHubRepository(repoURL)
	if err != nil {
		return nil, err
	}
	if commitSHA == "" {
		return nil, fmt.Errorf("commit SHA is required")
	}

	return f.downloadArchive(ctx, repo, commitSHA)
}

func parseGitHubRepository(repoURL string) (githubRepository, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return githubRepository{}, fmt.Errorf("invalid repository URL: %w", err)
	}
	if u.Scheme != "https" {
		return githubRepository{}, fmt.Errorf("repository URL must use HTTPS")
	}
	if u.Host != "github.com" {
		return githubRepository{}, fmt.Errorf("repository host must be github.com")
	}
	if u.User != nil {
		return githubRepository{}, fmt.Errorf("repository URL must not include credentials")
	}

	trimmed := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return githubRepository{}, fmt.Errorf("repository URL must be of the form https://github.com/{owner}/{repo}")
	}

	return githubRepository{
		Owner: parts[0],
		Repo:  parts[1],
	}, nil
}

func (f *githubRepositoryFetcher) getRepositoryMetadata(ctx context.Context, repo githubRepository) (githubRepositoryMetadata, error) {
	var metadata githubRepositoryMetadata
	if err := f.getJSON(ctx, fmt.Sprintf("%s/repos/%s/%s", strings.TrimRight(f.apiBaseURL, "/"), repo.Owner, repo.Repo), &metadata); err != nil {
		return githubRepositoryMetadata{}, err
	}
	return metadata, nil
}

func (f *githubRepositoryFetcher) resolveCommitSHA(ctx context.Context, repo githubRepository, ref string) (string, error) {
	var commit githubCommit
	if err := f.getJSON(ctx, fmt.Sprintf("%s/repos/%s/%s/commits/%s", strings.TrimRight(f.apiBaseURL, "/"), repo.Owner, repo.Repo, url.PathEscape(ref)), &commit); err != nil {
		return "", err
	}
	if commit.SHA == "" {
		return "", fmt.Errorf("GitHub did not return a commit SHA for ref %q", ref)
	}
	return commit.SHA, nil
}

func (f *githubRepositoryFetcher) downloadArchive(ctx context.Context, repo githubRepository, ref string) (*fetchedRepository, error) {
	workspace, err := os.MkdirTemp("", "skill-repository-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp workspace: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(workspace)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/zipball/%s", strings.TrimRight(f.apiBaseURL, "/"), repo.Owner, repo.Repo, url.PathEscape(ref)), nil)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create archive request: %w", err)
	}
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to download repository archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		cleanup()
		return nil, fmt.Errorf("repository archive download failed (status %d): %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, f.maxArchiveBytes+1))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to read repository archive: %w", err)
	}
	if int64(len(data)) > f.maxArchiveBytes {
		cleanup()
		return nil, fmt.Errorf("repository archive exceeds maximum size of %d bytes", f.maxArchiveBytes)
	}

	repoRoot := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create repository extraction directory: %w", err)
	}

	if err := extractGitHubArchive(data, repoRoot, f.maxExtractedFiles, f.maxExtractedBytes); err != nil {
		cleanup()
		return nil, err
	}

	return &fetchedRepository{
		RepoRoot:  repoRoot,
		CommitSHA: ref,
		cleanup:   cleanup,
	}, nil
}

func (f *githubRepositoryFetcher) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create GitHub API request: %w", err)
	}
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("GitHub API request failed (status %d): %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode GitHub API response: %w", err)
	}
	return nil
}

func extractGitHubArchive(data []byte, destRoot string, maxFiles int, maxBytes int64) error {
	// TODO(g-linville): ask multiple models for a security assessment of this
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid repository archive: %w", err)
	}

	absDestRoot, err := filepath.Abs(destRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve extraction root: %w", err)
	}

	var (
		rootPrefix    string
		fileCount     int
		extractedSize int64
	)

	for _, file := range reader.File {
		parts := strings.SplitN(strings.TrimPrefix(file.Name, "/"), "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			return fmt.Errorf("archive entry %q is missing a repository root prefix", file.Name)
		}
		if rootPrefix == "" {
			rootPrefix = parts[0]
		} else if parts[0] != rootPrefix {
			return fmt.Errorf("archive contains multiple root prefixes")
		}
		if len(parts) == 1 {
			continue
		}

		relPath := path.Clean(parts[1])
		if relPath == "." || relPath == "" {
			continue
		}
		if strings.HasPrefix(relPath, "../") || path.IsAbs(relPath) {
			return fmt.Errorf("archive entry %q escapes the repository root", file.Name)
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive entry %q is a symbolic link", file.Name)
		}

		targetPath := filepath.Join(destRoot, filepath.FromSlash(relPath))
		absTargetPath, err := filepath.Abs(targetPath)
		if err != nil {
			return fmt.Errorf("failed to resolve extraction path for %q: %w", relPath, err)
		}
		if absTargetPath != absDestRoot && !strings.HasPrefix(absTargetPath, absDestRoot+string(filepath.Separator)) {
			return fmt.Errorf("archive entry %q escapes the extraction directory", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %q: %w", relPath, err)
			}
			continue
		}
		if !file.Mode().IsRegular() {
			return fmt.Errorf("archive entry %q is not a regular file", file.Name)
		}

		fileCount++
		if fileCount > maxFiles {
			return fmt.Errorf("repository archive exceeds maximum file count of %d", maxFiles)
		}
		// Fast-reject using declared size (untrusted metadata, but cheap to check).
		if file.UncompressedSize64 > uint64(maxBytes) {
			return fmt.Errorf("archive entry %q declares uncompressed size exceeding limit", file.Name)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create parent directory for %q: %w", relPath, err)
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open archive entry %q: %w", relPath, err)
		}

		mode := file.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}

		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create extracted file %q: %w", relPath, err)
		}

		remainingBytes := maxBytes - extractedSize
		written, copyErr := io.Copy(out, io.LimitReader(rc, remainingBytes+1))
		closeErr := out.Close()
		rcErr := rc.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to extract file %q: %w", relPath, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close extracted file %q: %w", relPath, closeErr)
		}
		if rcErr != nil {
			return fmt.Errorf("failed to close archive entry %q: %w", relPath, rcErr)
		}
		extractedSize += written
		if extractedSize > maxBytes {
			return fmt.Errorf("repository archive exceeds maximum extracted size of %d bytes", maxBytes)
		}
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
