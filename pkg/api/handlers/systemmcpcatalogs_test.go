package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
