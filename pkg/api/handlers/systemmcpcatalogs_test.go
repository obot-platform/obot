package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMaskCatalogCredential(t *testing.T) {
	assert.Equal(t, "****", maskCatalogCredential(""))
	assert.Equal(t, "****", maskCatalogCredential("abc"))
	assert.Equal(t, "****", maskCatalogCredential("1234"))
	assert.Equal(t, "****1234", maskCatalogCredential("ghp_abcdefghij1234"))
}

func TestIsMaskedCatalogCredential(t *testing.T) {
	assert.True(t, isMaskedCatalogCredential("****"))
	assert.True(t, isMaskedCatalogCredential("****1234"))
	assert.True(t, isMaskedCatalogCredential("****abc"))
	assert.False(t, isMaskedCatalogCredential("ghp_abcdefghij1234"))
	assert.False(t, isMaskedCatalogCredential("****12345"))
}

func TestMaskCatalogCredentials(t *testing.T) {
	masked := maskCatalogCredentials(
		[]string{"https://github.com/org/repo", "https://github.com/org/other"},
		map[string]string{
			"https://github.com/org/repo": "ghp_abcdefghij1234",
		},
	)
	assert.Equal(t, map[string]string{
		"https://github.com/org/repo": "****1234",
	}, masked)
}

func TestMergeCatalogTokens(t *testing.T) {
	sourceURLs := []string{"https://github.com/org/repo", "https://github.com/org/other"}
	existing := map[string]string{
		"https://github.com/org/repo": "ghp_abcdefghij1234",
	}

	t.Run("preserves existing token for wildcard sentinel", func(t *testing.T) {
		got := mergeCatalogTokens(sourceURLs, map[string]string{
			"https://github.com/org/repo": "*",
		}, existing)
		assert.Equal(t, map[string]string{
			"https://github.com/org/repo": "ghp_abcdefghij1234",
		}, got)
	})

	t.Run("preserves existing token for masked round-trip", func(t *testing.T) {
		got := mergeCatalogTokens(sourceURLs, map[string]string{
			"https://github.com/org/repo": "****1234",
		}, existing)
		assert.Equal(t, map[string]string{
			"https://github.com/org/repo": "ghp_abcdefghij1234",
		}, got)
	})

	t.Run("preserves existing short token for fully masked round-trip", func(t *testing.T) {
		shortExisting := map[string]string{
			"https://github.com/org/repo": "abc",
		}
		got := mergeCatalogTokens(sourceURLs, map[string]string{
			"https://github.com/org/repo": "****",
		}, shortExisting)
		assert.Equal(t, map[string]string{
			"https://github.com/org/repo": "abc",
		}, got)
	})

	t.Run("stores new token when provided", func(t *testing.T) {
		got := mergeCatalogTokens(sourceURLs, map[string]string{
			"https://github.com/org/other": "new-token-value",
		}, existing)
		assert.Equal(t, map[string]string{
			"https://github.com/org/repo":  "ghp_abcdefghij1234",
			"https://github.com/org/other": "new-token-value",
		}, got)
	})

	t.Run("ignores masked credential remapped to new source URL", func(t *testing.T) {
		got := mergeCatalogTokens(
			[]string{"https://github.com/org/renamed"},
			map[string]string{
				"https://github.com/org/renamed": "****1234",
			},
			map[string]string{
				"https://github.com/org/repo": "ghp_abcdefghij1234",
			},
		)
		assert.Empty(t, got)
	})

	t.Run("preserves existing token when masked credential already persisted as secret", func(t *testing.T) {
		got := mergeCatalogTokens(
			[]string{"https://github.com/sangee2004/mcp-catalog"},
			map[string]string{
				"https://github.com/sangee2004/mcp-catalog": "****3132",
			},
			map[string]string{
				"https://github.com/sangee2004/mcp-catalog": "****3132",
			},
		)
		assert.Equal(t, map[string]string{
			"https://github.com/sangee2004/mcp-catalog": "****3132",
		}, got)
	})
}

func TestConvertSystemMCPServerCatalogEntryResources(t *testing.T) {
	resources := &types.MCPResourceRequirements{
		Requests: types.MCPResourceRequests{CPU: "250m", Memory: "512Mi"},
		Limits:   types.MCPResourceRequests{CPU: "1", Memory: "1Gi"},
	}

	entry := ConvertSystemMCPServerCatalogEntry(v1.SystemMCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "entry"},
		Spec: v1.SystemMCPServerCatalogEntrySpec{
			Manifest: types.SystemMCPServerCatalogEntryManifest{
				Name:      "entry",
				Resources: resources,
			},
		},
	})

	assert.Equal(t, resources, entry.Manifest.Resources)
}

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
