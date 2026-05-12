package skillrepository

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRepositoryURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{
			name: "valid GitHub HTTPS URL",
			url:  "https://github.com/owner/repo",
		},
		{
			name: "valid GitLab HTTPS URL",
			url:  "https://gitlab.com/owner/repo",
		},
		{
			name: "valid GitHub URL with ref path",
			url:  "https://github.com/owner/repo/main",
		},
		{
			name: "valid GitLab subgroup with .git suffix",
			url:  "https://gitlab.com/group/subgroup/repo.git/main",
		},
		{
			name: "valid with .git suffix",
			url:  "https://example.com/owner/repo.git",
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
			name:    "non-git HTTPS URL",
			url:     "https://example.com/some/page",
			wantErr: "does not appear to be a git repository",
		},
		{
			name:    "GitHub owner only rejected",
			url:     "https://github.com/owner",
			wantErr: "owner and repository",
		},
		{
			name:    "GitLab owner only rejected",
			url:     "https://gitlab.com/owner",
			wantErr: "owner and repository",
		},
		{
			name:    "embedded credentials rejected",
			url:     "https://token@github.com/owner/repo",
			wantErr: "must not include credentials",
		},
		{
			name:    "non-GitHub host without .git rejected",
			url:     "https://example.com/owner/repo",
			wantErr: "does not appear to be a git repository",
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: "HTTPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepositoryURL(tt.url)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
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
