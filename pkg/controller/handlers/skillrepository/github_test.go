package skillrepository

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubRepository(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   string
	}{
		{
			name:      "valid HTTPS URL",
			url:       "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "valid with .git suffix",
			url:       "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "valid with trailing slash",
			url:       "https://github.com/owner/repo/",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:    "HTTP scheme rejected",
			url:     "http://github.com/owner/repo",
			wantErr: "HTTPS",
		},
		{
			name:    "SSH scheme rejected",
			url:     "ssh://github.com/owner/repo",
			wantErr: "HTTPS",
		},
		{
			name:    "non-github host",
			url:     "https://gitlab.com/owner/repo",
			wantErr: "github.com",
		},
		{
			name:    "embedded credentials",
			url:     "https://user:pass@github.com/owner/repo",
			wantErr: "credentials",
		},
		{
			name:    "too many path segments",
			url:     "https://github.com/owner/repo/extra",
			wantErr: "form",
		},
		{
			name:    "too few path segments",
			url:     "https://github.com/owner",
			wantErr: "form",
		},
		{
			name:    "empty owner",
			url:     "https://github.com//repo",
			wantErr: "form",
		},
		{
			name:    "empty repo after trim",
			url:     "https://github.com/owner/",
			wantErr: "form",
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: "HTTPS",
		},
		{
			name:    "not a URL",
			url:     "not-a-url",
			wantErr: "HTTPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGitHubRepository(tt.url)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, got.Owner)
			assert.Equal(t, tt.wantRepo, got.Repo)
		})
	}
}

func TestValidateRepositoryURL(t *testing.T) {
	assert.NoError(t, ValidateRepositoryURL("https://github.com/owner/repo"))
	assert.Error(t, ValidateRepositoryURL("http://github.com/owner/repo"))
}

// zipEntry describes a single entry in a test ZIP archive.
type zipEntry struct {
	Name      string
	Body      []byte
	Mode      os.FileMode
	IsDir     bool
	IsSymlink bool
}

// buildZipArchive constructs an in-memory ZIP archive from the given entries.
func buildZipArchive(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.Name}
		if e.IsDir {
			fh.Name = strings.TrimSuffix(fh.Name, "/") + "/"
			fh.SetMode(os.ModeDir | 0o755)
		} else if e.IsSymlink {
			fh.SetMode(os.ModeSymlink | 0o777)
		} else {
			mode := e.Mode
			if mode == 0 {
				mode = 0o644
			}
			fh.SetMode(mode)
		}

		fw, err := w.CreateHeader(fh)
		require.NoError(t, err)
		if !e.IsDir {
			_, err = fw.Write(e.Body)
			require.NoError(t, err)
		}
	}

	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestExtractGitHubArchive(t *testing.T) {
	t.Run("valid single-root archive", func(t *testing.T) {
		data := buildZipArchive(t, []zipEntry{
			{Name: "root-abc123/", IsDir: true},
			{Name: "root-abc123/file.txt", Body: []byte("hello")},
			{Name: "root-abc123/sub/", IsDir: true},
			{Name: "root-abc123/sub/nested.txt", Body: []byte("world")},
		})
		dest := t.TempDir()
		require.NoError(t, extractGitHubArchive(data, dest, 100, 1024*1024))

		content, err := os.ReadFile(filepath.Join(dest, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(content))

		content, err = os.ReadFile(filepath.Join(dest, "sub", "nested.txt"))
		require.NoError(t, err)
		assert.Equal(t, "world", string(content))
	})

	t.Run("path traversal via ../", func(t *testing.T) {
		data := buildZipArchive(t, []zipEntry{
			{Name: "root/", IsDir: true},
			{Name: "root/../../etc/passwd", Body: []byte("bad")},
		})
		dest := t.TempDir()
		err := extractGitHubArchive(data, dest, 100, 1024*1024)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "escapes")
	})

	t.Run("symlink entry rejected", func(t *testing.T) {
		data := buildZipArchive(t, []zipEntry{
			{Name: "root/", IsDir: true},
			{Name: "root/link", Body: []byte("/etc/passwd"), IsSymlink: true},
		})
		dest := t.TempDir()
		err := extractGitHubArchive(data, dest, 100, 1024*1024)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "symbolic link")
	})

	t.Run("multiple root prefixes rejected", func(t *testing.T) {
		data := buildZipArchive(t, []zipEntry{
			{Name: "root1/file.txt", Body: []byte("a")},
			{Name: "root2/file.txt", Body: []byte("b")},
		})
		dest := t.TempDir()
		err := extractGitHubArchive(data, dest, 100, 1024*1024)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multiple root")
	})

	t.Run("file count limit exceeded", func(t *testing.T) {
		entries := []zipEntry{{Name: "root/", IsDir: true}}
		for i := 0; i < 5; i++ {
			entries = append(entries, zipEntry{
				Name: fmt.Sprintf("root/file%d.txt", i),
				Body: []byte("x"),
			})
		}
		data := buildZipArchive(t, entries)
		dest := t.TempDir()
		err := extractGitHubArchive(data, dest, 3, 1024*1024)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "file count")
	})

	t.Run("extracted size limit exceeded via declared size", func(t *testing.T) {
		bigContent := bytes.Repeat([]byte("A"), 1024)
		data := buildZipArchive(t, []zipEntry{
			{Name: "root/", IsDir: true},
			{Name: "root/big.txt", Body: bigContent},
		})
		dest := t.TempDir()
		err := extractGitHubArchive(data, dest, 100, 512)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeding limit")
	})

	t.Run("extracted size limit exceeded via cumulative writes", func(t *testing.T) {
		// Each file is small enough to pass the declared-size fast-reject,
		// but their cumulative actual bytes exceed the limit.
		content := bytes.Repeat([]byte("B"), 400)
		data := buildZipArchive(t, []zipEntry{
			{Name: "root/", IsDir: true},
			{Name: "root/a.txt", Body: content},
			{Name: "root/b.txt", Body: content},
			{Name: "root/c.txt", Body: content},
		})
		dest := t.TempDir()
		err := extractGitHubArchive(data, dest, 100, 1000)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extracted size")
	})

	t.Run("empty archive", func(t *testing.T) {
		data := buildZipArchive(t, nil)
		dest := t.TempDir()
		require.NoError(t, extractGitHubArchive(data, dest, 100, 1024*1024))
	})

	t.Run("nested directories auto-created", func(t *testing.T) {
		data := buildZipArchive(t, []zipEntry{
			{Name: "root/a/b/c/deep.txt", Body: []byte("deep")},
		})
		dest := t.TempDir()
		require.NoError(t, extractGitHubArchive(data, dest, 100, 1024*1024))

		content, err := os.ReadFile(filepath.Join(dest, "a", "b", "c", "deep.txt"))
		require.NoError(t, err)
		assert.Equal(t, "deep", string(content))
	})
}

func TestSafeJoinWithin(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name    string
		relPath string
		wantErr string
	}{
		{name: "simple relative", relPath: "skills/my-skill"},
		{name: "dot path", relPath: "."},
		{name: "empty path", relPath: ""},
		{name: "nested valid", relPath: "a/b/c"},
		{name: "traversal ../", relPath: "../escape", wantErr: "escapes"},
		{name: "traversal ../../", relPath: "../../etc", wantErr: "escapes"},
		{name: "absolute path", relPath: "/etc/passwd", wantErr: "escapes"},
		{name: "nested traversal", relPath: "a/../../escape", wantErr: "escapes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := safeJoinWithin(base, tt.relPath)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			// Result should be within the base directory
			absBase, _ := filepath.Abs(base)
			assert.True(t, result == absBase || strings.HasPrefix(result, absBase+string(filepath.Separator)),
				"result %q should be within base %q", result, absBase)
		})
	}
}

// newTestGitHubServer creates an httptest server that mocks the GitHub API.
// It supports GET /repos/{owner}/{repo}, /repos/{owner}/{repo}/commits/{ref},
// and /repos/{owner}/{repo}/zipball/{ref}.
func newTestGitHubServer(t *testing.T, opts testGitHubServerOpts) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		// Record auth header for verification
		if opts.recordAuthHeader != nil {
			opts.recordAuthHeader(r.Header.Get("Authorization"))
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		// /repos/{owner}/{repo}
		if len(parts) == 3 {
			if opts.metadataStatus != 0 {
				w.WriteHeader(opts.metadataStatus)
				return
			}
			meta := opts.metadata
			if meta == nil {
				meta = &githubRepositoryMetadata{Size: 1024, DefaultBranch: "main"}
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(meta))
			return
		}
		// /repos/{owner}/{repo}/commits/{ref}
		if len(parts) == 5 && parts[3] == "commits" {
			if opts.commitStatus != 0 {
				w.WriteHeader(opts.commitStatus)
				return
			}
			commit := githubCommit{SHA: opts.commitSHA}
			if commit.SHA == "" {
				commit.SHA = "abc123def456"
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(commit))
			return
		}
		// /repos/{owner}/{repo}/git/trees/{sha}
		if len(parts) == 6 && parts[3] == "git" && parts[4] == "trees" {
			if opts.treeStatus != 0 {
				w.WriteHeader(opts.treeStatus)
				return
			}
			tree := opts.tree
			if tree == nil {
				tree = &githubTree{
					Tree: []githubTreeEntry{
						{Path: "README.md", Type: "blob"},
					},
				}
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(tree))
			return
		}
		// /repos/{owner}/{repo}/zipball/{ref}
		if len(parts) == 5 && parts[3] == "zipball" {
			if opts.archiveStatus != 0 {
				w.WriteHeader(opts.archiveStatus)
				return
			}
			if opts.archiveData != nil {
				_, err := w.Write(opts.archiveData)
				require.NoError(t, err)
				return
			}
			// Return a valid archive with one file
			data := buildZipArchive(t, []zipEntry{
				{Name: "owner-repo-abc123/", IsDir: true},
				{Name: "owner-repo-abc123/README.md", Body: []byte("# Hello")},
			})
			_, err := w.Write(data)
			require.NoError(t, err)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	return httptest.NewServer(mux)
}

type testGitHubServerOpts struct {
	metadata         *githubRepositoryMetadata
	metadataStatus   int
	commitSHA        string
	commitStatus     int
	tree             *githubTree
	treeStatus       int
	archiveData      []byte
	archiveStatus    int
	recordAuthHeader func(string)
}

func TestGitHubRepositoryFetcher_Fetch(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 10000,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		result, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.NoError(t, err)
		defer result.Cleanup()

		assert.DirExists(t, result.RepoRoot)
		assert.NotEmpty(t, result.CommitSHA)
		// Verify extracted file exists
		assert.FileExists(t, filepath.Join(result.RepoRoot, "README.md"))
	})

	t.Run("repo too large", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			metadata: &githubRepositoryMetadata{
				Size:          200 * 1024, // 200 MB in KB
				DefaultBranch: "main",
			},
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:        srv.Client(),
			apiBaseURL:    srv.URL,
			maxRepoSizeMB: 100,
		}

		_, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too large")
	})

	t.Run("empty default branch no ref", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			metadata: &githubRepositoryMetadata{
				Size:          1024,
				DefaultBranch: "",
			},
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:        srv.Client(),
			apiBaseURL:    srv.URL,
			maxRepoSizeMB: 100,
		}

		_, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "default branch")
	})

	t.Run("uses default branch when ref empty", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			metadata: &githubRepositoryMetadata{
				Size:          1024,
				DefaultBranch: "develop",
			},
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 10000,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		result, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "")
		require.NoError(t, err)
		defer result.Cleanup()
		assert.NotEmpty(t, result.CommitSHA)
	})

	t.Run("metadata HTTP error", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			metadataStatus: http.StatusNotFound,
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:        srv.Client(),
			apiBaseURL:    srv.URL,
			maxRepoSizeMB: 100,
		}

		_, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("archive HTTP error", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			archiveStatus: http.StatusInternalServerError,
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 10000,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		_, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("repository tree entry count exceeds limit", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			tree: &githubTree{
				Tree: []githubTreeEntry{
					{Path: "a", Type: "blob"},
					{Path: "b", Type: "blob"},
					{Path: "c", Type: "blob"},
					{Path: "d", Type: "blob"},
				},
			},
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 3,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		_, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum entry count")
	})

	t.Run("truncated repository tree rejected", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			tree: &githubTree{
				Tree:      []githubTreeEntry{{Path: "README.md", Type: "blob"}},
				Truncated: true,
			},
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 10000,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		_, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tree listing")
		assert.Contains(t, err.Error(), "truncated")
	})

	t.Run("archive too large", func(t *testing.T) {
		bigArchive := bytes.Repeat([]byte("X"), 200)
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			archiveData: bigArchive,
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100, // very small limit
			maxExtractedFiles: 10000,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		_, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum size")
	})

	t.Run("auth token forwarded", func(t *testing.T) {
		var authHeaders []string
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			recordAuthHeader: func(h string) {
				authHeaders = append(authHeaders, h)
			},
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			token:             "test-token-123",
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 10000,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		result, err := f.Fetch(context.Background(), "https://github.com/owner/repo", "main")
		require.NoError(t, err)
		defer result.Cleanup()

		// Metadata, commit, tree, and archive requests should all include the auth header.
		require.GreaterOrEqual(t, len(authHeaders), 4)
		for _, h := range authHeaders {
			assert.Equal(t, "Bearer test-token-123", h)
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		f := &githubRepositoryFetcher{}
		_, err := f.Fetch(context.Background(), "http://gitlab.com/bad/url", "main")
		require.Error(t, err)
	})
}

func TestGitHubRepositoryFetcher_MaterializeCommit(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 10000,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		result, err := f.MaterializeCommit(context.Background(), "https://github.com/owner/repo", "abc123")
		require.NoError(t, err)
		defer result.Cleanup()
		assert.DirExists(t, result.RepoRoot)
	})

	t.Run("entry count limit enforced", func(t *testing.T) {
		srv := newTestGitHubServer(t, testGitHubServerOpts{
			tree: &githubTree{
				Tree: []githubTreeEntry{
					{Path: "a", Type: "blob"},
					{Path: "b", Type: "blob"},
					{Path: "c", Type: "blob"},
					{Path: "d", Type: "blob"},
				},
			},
		})
		defer srv.Close()

		f := &githubRepositoryFetcher{
			client:            srv.Client(),
			apiBaseURL:        srv.URL,
			maxRepoSizeMB:     100,
			maxArchiveBytes:   100 * 1024 * 1024,
			maxExtractedFiles: 3,
			maxExtractedBytes: 100 * 1024 * 1024,
		}

		_, err := f.MaterializeCommit(context.Background(), "https://github.com/owner/repo", "abc123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum entry count")
	})

	t.Run("empty commit SHA", func(t *testing.T) {
		f := &githubRepositoryFetcher{}
		_, err := f.MaterializeCommit(context.Background(), "https://github.com/owner/repo", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commit SHA is required")
	})

	t.Run("invalid URL", func(t *testing.T) {
		f := &githubRepositoryFetcher{}
		_, err := f.MaterializeCommit(context.Background(), "not-valid", "abc123")
		require.Error(t, err)
	})
}
