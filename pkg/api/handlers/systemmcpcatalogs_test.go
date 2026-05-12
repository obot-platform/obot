package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
)

func TestValidateSystemCatalogManifest_AllowsConfiguredLocalPath(t *testing.T) {
	manifest := &types.SystemMCPCatalogManifest{
		SourceURLs: []string{"/tmp/system-catalog"},
	}

	if err := validateSystemCatalogManifest(manifest, "/tmp/system-catalog"); err != nil {
		t.Fatalf("expected configured local path to be allowed, got %v", err)
	}

	if manifest.SourceURLs[0] != "/tmp/system-catalog" {
		t.Fatalf("expected local path to remain unchanged, got %q", manifest.SourceURLs[0])
	}
}

func TestValidateSystemCatalogManifest_NormalizesNonLocalSourceURLs(t *testing.T) {
	manifest := &types.SystemMCPCatalogManifest{
		SourceURLs: []string{"example.com/system-catalog.yaml"},
	}

	if err := validateSystemCatalogManifest(manifest, "/tmp/system-catalog"); err != nil {
		t.Fatalf("expected remote source URL to validate, got %v", err)
	}

	if manifest.SourceURLs[0] != "https://example.com/system-catalog.yaml" {
		t.Fatalf("expected source URL to be normalized, got %q", manifest.SourceURLs[0])
	}
}

func TestMergeCatalogTokensNormalizesKeys(t *testing.T) {
	tests := []struct {
		name       string
		sourceURLs []string
		incoming   map[string]string
		existing   map[string]string
		want       map[string]string
	}{
		{
			name:       "trims incoming credential key",
			sourceURLs: []string{"https://github.com/owner/repo"},
			incoming:   map[string]string{" https://github.com/owner/repo ": "token"},
			want:       map[string]string{"https://github.com/owner/repo": "token"},
		},
		{
			name:       "adds https to incoming credential key",
			sourceURLs: []string{"https://github.com/owner/repo"},
			incoming:   map[string]string{"github.com/owner/repo": "token"},
			want:       map[string]string{"https://github.com/owner/repo": "token"},
		},
		{
			name:       "preserves normalized existing token for masked incoming key",
			sourceURLs: []string{"https://github.com/owner/repo"},
			incoming:   map[string]string{" github.com/owner/repo ": "*"},
			existing:   map[string]string{"https://github.com/owner/repo": "existing-token"},
			want:       map[string]string{"https://github.com/owner/repo": "existing-token"},
		},
		{
			name:       "ignores inactive normalized credential key",
			sourceURLs: []string{"https://github.com/owner/repo"},
			incoming:   map[string]string{"github.com/other/repo": "token"},
			want:       map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mergeCatalogTokens(tt.sourceURLs, tt.incoming, tt.existing))
		})
	}
}

func TestMaskCatalogCredentialsNormalizesKeys(t *testing.T) {
	assert.Equal(t,
		map[string]string{"https://github.com/owner/repo": "*"},
		maskCatalogCredentials(
			[]string{"https://github.com/owner/repo"},
			map[string]string{" github.com/owner/repo ": "token"},
		),
	)
}
