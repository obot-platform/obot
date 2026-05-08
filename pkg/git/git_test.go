package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGitRepoURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/org/repo", true},
		{"https://gitlab.com/org/repo", true},
		{"https://example.com/org/repo.git", true},
		{"https://self-hosted.example.com/org/repo.git", true},
		{"https://example.com/some/raw/file.yaml", false},
		{"https://example.com/catalog.json", false},
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
			name:    "bitbucket without .git",
			url:     "https://bitbucket.org/org/repo",
			wantErr: true,
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
				{name: "token", token: "repo-token"},
			},
		},
		{
			name:          "explicit token ignores fallback token",
			token:         "repo-token",
			fallbackToken: "fallback-token",
			want: []cloneAuthAttempt{
				{name: "token", token: "repo-token"},
			},
		},
		{
			name: "anonymous only",
			want: []cloneAuthAttempt{
				{name: "anonymous"},
			},
		},
		{
			name:          "anonymous then fallback token",
			fallbackToken: "fallback-token",
			want: []cloneAuthAttempt{
				{name: "anonymous"},
				{name: "fallback token", token: "fallback-token"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cloneAuthAttempts(tt.token, tt.fallbackToken))
		})
	}
}
