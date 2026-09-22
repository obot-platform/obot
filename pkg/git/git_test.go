package git

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestIsGitRepoURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{
			url:  "https://github.com/org/repo",
			want: true,
		},
		{
			url:  "https://gitlab.com/org/repo",
			want: true,
		},
		{
			url:  "https://bitbucket.org/org/repo",
			want: true,
		},
		{
			url:  "https://example.com/org/repo.git",
			want: true,
		},
		{
			url:  "https://self-hosted.example.com/org/repo.git",
			want: true,
		},
		{
			url:  "https://example.com/some/raw/file.yaml",
			want: false,
		},
		{
			url:  "https://example.com/catalog.json",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, IsGitRepoURL(tt.url))
		})
	}
}

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantClone  string
		wantBranch string
		wantErr    bool
	}{
		{
			name:       "github without .git",
			url:        "https://github.com/org/repo",
			wantClone:  "https://github.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "github with .git",
			url:        "https://github.com/org/repo.git",
			wantClone:  "https://github.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "github with branch",
			url:        "https://github.com/org/repo/my-branch",
			wantClone:  "https://github.com/org/repo.git",
			wantBranch: "my-branch",
		},
		{
			name:       "gitlab with .git",
			url:        "https://gitlab.com/org/repo.git",
			wantClone:  "https://gitlab.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "gitlab subgroup",
			url:        "https://gitlab.com/group/subgroup/repo.git",
			wantClone:  "https://gitlab.com/group/subgroup/repo.git",
			wantBranch: "main",
		},
		{
			name:       "gitlab subgroup with branch",
			url:        "https://gitlab.com/group/subgroup/repo.git/my-branch",
			wantClone:  "https://gitlab.com/group/subgroup/repo.git",
			wantBranch: "my-branch",
		},
		{
			name:       "gitlab without .git",
			url:        "https://gitlab.com/org/repo",
			wantClone:  "https://gitlab.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "gitlab with branch",
			url:        "https://gitlab.com/org/repo/my-branch",
			wantClone:  "https://gitlab.com/org/repo.git",
			wantBranch: "my-branch",
		},
		{
			name:       "bitbucket without .git",
			url:        "https://bitbucket.org/org/repo",
			wantClone:  "https://bitbucket.org/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "bitbucket with .git",
			url:        "https://bitbucket.org/org/repo.git",
			wantClone:  "https://bitbucket.org/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "bitbucket with branch",
			url:        "https://bitbucket.org/org/repo/feature/catalog",
			wantClone:  "https://bitbucket.org/org/repo.git",
			wantBranch: "feature/catalog",
		},
		{
			name:       "bitbucket with .git and branch",
			url:        "https://bitbucket.org/org/repo.git/feature/catalog",
			wantClone:  "https://bitbucket.org/org/repo.git",
			wantBranch: "feature/catalog",
		},
		{
			name:    "unknown host without .git is rejected",
			url:     "https://self-hosted.example.com/org/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloneURL, branch, err := parseGitURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantClone, cloneURL)
			assert.Equal(t, tt.wantBranch, branch)
		})
	}
}

func TestCloneAuthAttempts(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		fallbackToken string
		want          []cloneAuthAttempt
	}{
		{
			name:  "explicit token only",
			token: "repo-token",
			want: []cloneAuthAttempt{
				{
					name:  "token",
					token: "repo-token",
				},
			},
		},
		{
			name:          "explicit token ignores fallback token",
			token:         "repo-token",
			fallbackToken: "fallback-token",
			want: []cloneAuthAttempt{
				{
					name:  "token",
					token: "repo-token",
				},
			},
		},
		{
			name: "anonymous only",
			want: []cloneAuthAttempt{
				{
					name: "anonymous",
				},
			},
		},
		{
			name:          "anonymous then fallback token",
			fallbackToken: "fallback-token",
			want: []cloneAuthAttempt{
				{
					name: "anonymous",
				},
				{
					name:  "fallback token",
					token: "fallback-token",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cloneAuthAttempts(tt.token, tt.fallbackToken))
		})
	}
}

func TestValidateRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{
			name: "branch",
			ref:  "main",
		},
		{
			name: "nested branch",
			ref:  "feature/git-sync",
		},
		{
			name: "tag",
			ref:  "v1.2.3",
		},
		{
			name: "commit sha",
			ref:  "0123456789abcdef0123456789abcdef01234567",
		},
		{
			name:    "empty",
			wantErr: true,
		},
		{
			name:    "path traversal",
			ref:     "feature/../main",
			wantErr: true,
		},
		{
			name:    "leading dash",
			ref:     "-main",
			wantErr: true,
		},
		{
			name:    "contains colon",
			ref:     "main:other",
			wantErr: true,
		},
		{
			name:    "contains whitespace",
			ref:     "main branch",
			wantErr: true,
		},
		{
			name:    "trimmed whitespace",
			ref:     " main",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRef(tt.ref)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestCloneRefAttempts(t *testing.T) {
	t.Run("implicit ref only tries branch", func(t *testing.T) {
		assert.Equal(t, []cloneRefAttempt{
			{name: "branch", referenceName: "refs/heads/main", depth: 1},
		}, cloneRefAttempts("main", false))
	})

	t.Run("explicit ref tries branch then tag", func(t *testing.T) {
		assert.Equal(t, []cloneRefAttempt{
			{name: "branch", referenceName: "refs/heads/v1.0.0", depth: 1},
			{name: "tag", referenceName: "refs/tags/v1.0.0", depth: 1},
		}, cloneRefAttempts("v1.0.0", true))
	})

	t.Run("commit sha checks out hash", func(t *testing.T) {
		sha := "0123456789abcdef0123456789abcdef01234567"
		assert.Equal(t, []cloneRefAttempt{
			{name: "commit", checkoutHash: sha},
		}, cloneRefAttempts(sha, true))
	})
}

func TestRepositorySizeChecksReturnSentinel(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	t.Run("GitHub", func(t *testing.T) {
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"size": 204800}`)),
				Header:     make(http.Header),
			}, nil
		})

		err := checkGitHubRepoSize(t.Context(), "example", "repo", 100, "")
		assert.ErrorIs(t, err, errRepoTooLarge)
	})

	t.Run("GitLab", func(t *testing.T) {
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"statistics":{"repository_size":209715200}}`)),
				Header:     make(http.Header),
			}, nil
		})

		err := checkGitLabRepoSize(t.Context(), "gitlab.com", "example/repo", 100, "token")
		assert.ErrorIs(t, err, errRepoTooLarge)
	})
}

func TestRepoSizeLimitMB(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		want    int
		wantErr bool
	}{
		{
			name: "default",
			want: 100,
		},
		{
			name:  "custom",
			value: 250,
			want:  250,
		},
		{
			name:    "negative",
			value:   -1,
			wantErr: true,
		},
		{
			name:    "byte overflow",
			value:   8796093022208,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, err := repoSizeLimitMB(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, limit)
			}
		})
	}
}

// Verify Clone forwards the configured limit to both provider pre-checks:
// the same 200 MB repository is rejected at 100 MB but reaches cloning at 250 MB.
func TestCloneConfiguredSizeLimit(t *testing.T) {
	originalHTTPS := client.Protocols["https"]
	t.Cleanup(func() { client.InstallProtocol("https", originalHTTPS) })
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	t.Setenv("GITHUB_AUTH_TOKEN", "")
	for _, host := range []string{"github.com", "gitlab.com"} {
		t.Run(host, func(t *testing.T) {
			cloneAttempted := false
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.github.com" || strings.HasPrefix(req.URL.Path, "/api/v4/") {
					body := `{"size":204800,"statistics":{"repository_size":209715200}}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				}
				cloneAttempted = true
				return nil, context.Canceled
			})
			client.InstallProtocol("https", githttp.NewClient(&http.Client{Transport: http.DefaultTransport}))
			_, _, _, err := Clone(t.Context(), "https://"+host+"/example/repo", "token", "", 100)
			assert.ErrorIs(t, err, errRepoTooLarge)
			assert.False(t, cloneAttempted)

			_, _, _, err = Clone(t.Context(), "https://"+host+"/example/repo", "token", "", 250)
			assert.ErrorIs(t, err, context.Canceled)
			assert.True(t, cloneAttempted)
		})
	}
}

// Exercise the credentials sent by Clone through the real go-git HTTP transport.
func TestCloneBitbucketAPIToken(t *testing.T) {
	t.Setenv("GITHUB_AUTH_TOKEN", "")
	originalTransport := client.Protocols["https"]
	t.Cleanup(func() { client.InstallProtocol("https", originalTransport) })
	stop := errors.New("stop after inspecting clone request")
	requests := 0
	client.InstallProtocol("https", githttp.NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			assert.Equal(t, "bitbucket.org", req.URL.Host)
			assert.Equal(t, "/org/repo.git/info/refs", req.URL.Path)
			username, password, ok := req.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "x-bitbucket-api-token-auth", username)
			assert.Equal(t, "test-api-token", password)
			return nil, stop
		}),
	}))
	for _, repoURL := range []string{
		"https://bitbucket.org/org/repo.git",
		"https://bitbucket.org/org/repo",
		"https://bitbucket.org/org/repo/feature/catalog",
		"https://bitbucket.org/org/repo.git/feature/catalog",
	} {
		t.Run(repoURL, func(t *testing.T) {
			requests = 0
			_, _, cleanup, err := Clone(context.Background(), repoURL, "test-api-token", "")
			if cleanup != nil {
				cleanup()
			}
			assert.ErrorIs(t, err, stop)
			assert.Equal(t, 1, requests)
		})
	}
}
