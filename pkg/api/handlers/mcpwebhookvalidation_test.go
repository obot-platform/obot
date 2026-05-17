package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestMCPWebhookValidationManifest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.MCPWebhookValidationManifest
		wantErr  bool
	}{
		{
			name: "url only",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
			},
		},
		{
			name: "system server manifest only",
			manifest: types.MCPWebhookValidationManifest{
				ToolName: "validate-webhook",
				SystemMCPServerManifest: &types.SystemMCPServerManifest{
					Name:    "validator",
					Enabled: new(true),
					Runtime: types.RuntimeContainerized,
					ContainerizedConfig: &types.ContainerizedRuntimeConfig{
						Image: "example/image:latest",
						Port:  8080,
						Path:  "/mcp",
					},
				},
			},
		},
		{
			name: "system server catalog entry id only",
			manifest: types.MCPWebhookValidationManifest{
				SystemMCPServerCatalogEntryID: "system-mcpcatentry1",
			},
		},
		{
			name:     "missing url system server manifest and catalog entry id",
			manifest: types.MCPWebhookValidationManifest{},
			wantErr:  true,
		},
		{
			name: "url and system server manifest are mutually exclusive",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				SystemMCPServerManifest: &types.SystemMCPServerManifest{
					Name: "validator",
				},
			},
			wantErr: true,
		},
		{
			name: "url and system server catalog entry id are mutually exclusive",
			manifest: types.MCPWebhookValidationManifest{
				URL:                           "https://example.com/webhook",
				SystemMCPServerCatalogEntryID: "system-mcpcatentry1",
			},
			wantErr: true,
		},
		{
			name: "validation allows embedded manifest shape checks to happen later",
			manifest: types.MCPWebhookValidationManifest{
				SystemMCPServerManifest: &types.SystemMCPServerManifest{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManifest(&tt.manifest)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSystemMCPServerManifestFromCatalogEntry(t *testing.T) {
	resources := &types.MCPResourceRequirements{
		Requests: types.MCPResourceRequests{CPU: "250m", Memory: "512Mi"},
	}
	manifest := systemMCPServerManifestFromCatalogEntry(types.SystemMCPServerCatalogEntryManifest{
		Name:             "validator",
		ShortDescription: "short",
		Description:      "long",
		Runtime:          types.RuntimeRemote,
		Resources:        resources,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL: "https://example.com/mcp",
			Headers:  []types.MCPHeader{{Key: "Authorization", Value: "Bearer token"}},
		},
	}, true)

	if manifest.Name != "validator" {
		t.Fatalf("expected manifest name to be copied, got %q", manifest.Name)
	}
	if manifest.Enabled == nil || *manifest.Enabled {
		t.Fatalf("expected manifest to be disabled")
	}
	if manifest.RemoteConfig == nil || manifest.RemoteConfig.URL != "https://example.com/mcp" {
		t.Fatalf("expected fixed remote URL to be mapped, got %#v", manifest.RemoteConfig)
	}
	if manifest.Resources != resources {
		t.Fatalf("expected resources to be copied")
	}
}

func TestApplyRemoteURLTemplateToWebhookValidation(t *testing.T) {
	validation := &v1.MCPWebhookValidation{
		Spec: v1.MCPWebhookValidationSpec{
			Manifest: types.MCPWebhookValidationManifest{
				SystemMCPServerManifest: &types.SystemMCPServerManifest{
					Name:    "validator",
					Runtime: types.RuntimeRemote,
					RemoteConfig: &types.RemoteRuntimeConfig{
						IsTemplate:  true,
						URLTemplate: "https://${HOST}/mcp/${SPACE}",
					},
				},
			},
		},
	}

	err := applyRemoteURLTemplateToWebhookValidation(validation, map[string]string{
		"HOST":  "example.com",
		"SPACE": "abc123",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	remoteConfig := validation.Spec.Manifest.SystemMCPServerManifest.RemoteConfig
	if remoteConfig.URL != "https://example.com/mcp/abc123" {
		t.Fatalf("expected rendered URL, got %q", remoteConfig.URL)
	}
}

func TestResolveManifestFromCatalogEntry_RejectsEmbeddedManifest(t *testing.T) {
	h := &MCPWebhookValidationHandler{}
	manifest := &types.MCPWebhookValidationManifest{
		SystemMCPServerCatalogEntryID: "system-mcpcatentry1",
		SystemMCPServerManifest: &types.SystemMCPServerManifest{
			Name: "validator",
		},
	}

	err := h.resolveManifestFromCatalogEntry(api.Context{}, manifest)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "error code 400 (Bad Request): system MCP server manifest and system MCP server catalog entry ID are mutually exclusive" {
		t.Fatalf("unexpected error: %v", err)
	}
}
