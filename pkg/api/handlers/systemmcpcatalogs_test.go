package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
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
