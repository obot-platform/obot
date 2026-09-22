package skillrepository

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeJoinWithin(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name    string
		relPath string
		wantErr string
	}{
		{
			name:    "simple relative",
			relPath: "skills/my-skill",
		},
		{
			name:    "dot path",
			relPath: ".",
		},
		{
			name:    "empty path",
			relPath: "",
		},
		{
			name:    "nested valid",
			relPath: "a/b/c",
		},
		{
			name:    "traversal ../",
			relPath: "../escape",
			wantErr: "escapes",
		},
		{
			name:    "traversal ../../",
			relPath: "../../etc",
			wantErr: "escapes",
		},
		{
			name:    "absolute path",
			relPath: "/etc/passwd",
			wantErr: "escapes",
		},
		{
			name:    "nested traversal",
			relPath: "a/../../escape",
			wantErr: "escapes",
		},
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
