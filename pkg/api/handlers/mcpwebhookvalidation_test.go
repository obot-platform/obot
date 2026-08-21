package handlers

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestMCPWebhookValidationManifest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.MCPWebhookValidationManifest
		wantErr  bool
		errText  string
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
		{
			name: "device target with wildcard events",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.MCPWebhookValidationResource{
					{
						Type: types.MCPWebhookValidationResourceTypeDeviceSelector,
						ID:   "*",
					},
				},
				LocalAgentEvents: types.LocalAgentEvents{
					types.LocalAgentEventAll,
				},
			},
		},
		{
			name: "device and MCP targets together",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.MCPWebhookValidationResource{
					{
						Type: types.MCPWebhookValidationResourceTypeDeviceSelector,
						ID:   "*",
					},
					{
						Type: types.MCPWebhookValidationResourceTypeSelector,
						ID:   "*",
					},
				},
				LocalAgentEvents: types.LocalAgentEvents{
					types.LocalAgentEventUserPrompt,
					types.LocalAgentEventToolResponse,
				},
			},
		},
		{
			name: "device target requires events",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.MCPWebhookValidationResource{
					{
						Type: types.MCPWebhookValidationResourceTypeDeviceSelector,
						ID:   "*",
					},
				},
			},
			wantErr: true,
			errText: "local agent events are required",
		},
		{
			name: "events require device target",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				LocalAgentEvents: types.LocalAgentEvents{
					types.LocalAgentEventUserPrompt,
				},
			},
			wantErr: true,
			errText: "device selector resource is required",
		},
		{
			name: "device target ID must be wildcard",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.MCPWebhookValidationResource{
					{
						Type: types.MCPWebhookValidationResourceTypeDeviceSelector,
						ID:   "device-1",
					},
				},
				LocalAgentEvents: types.LocalAgentEvents{
					types.LocalAgentEventAll,
				},
			},
			wantErr: true,
			errText: "deviceSelector resource ID must be '*'",
		},
		{
			name: "wildcard event cannot be combined with explicit event",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.MCPWebhookValidationResource{
					{
						Type: types.MCPWebhookValidationResourceTypeDeviceSelector,
						ID:   "*",
					},
				},
				LocalAgentEvents: types.LocalAgentEvents{
					types.LocalAgentEventAll,
					types.LocalAgentEventUserPrompt,
				},
			},
			wantErr: true,
			errText: "cannot be combined",
		},
		{
			name: "invalid local agent event",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.MCPWebhookValidationResource{
					{
						Type: types.MCPWebhookValidationResourceTypeDeviceSelector,
						ID:   "*",
					},
				},
				LocalAgentEvents: types.LocalAgentEvents{
					"invalid",
				},
			},
			wantErr: true,
			errText: "invalid local agent event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManifest(t.Context(), &tt.manifest, mcp.ValidationOptions{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errText != "" && !strings.Contains(err.Error(), tt.errText) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.errText)
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
					Env: []types.MCPEnv{
						{Key: "HOST", Required: true},
						{Key: "SPACE", Required: true},
					},
					RemoteConfig: &types.RemoteRuntimeConfig{
						IsTemplate:  true,
						URLTemplate: "https://${HOST}/mcp/${SPACE}",
					},
				},
			},
		},
	}

	err := applyRemoteURLTemplateToWebhookValidation(t.Context(), validation, map[string]string{
		"HOST":  "example.com",
		"SPACE": "abc123",
	}, mcp.ValidationOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	remoteConfig := validation.Spec.Manifest.SystemMCPServerManifest.RemoteConfig
	if remoteConfig.URL != "https://example.com/mcp/abc123" {
		t.Fatalf("expected rendered URL, got %q", remoteConfig.URL)
	}
}

func TestApplyRemoteURLTemplateToWebhookValidationRejectsUnknownOption(t *testing.T) {
	validation := &v1.MCPWebhookValidation{Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
		SystemMCPServerManifest: &types.SystemMCPServerManifest{
			Runtime: types.RuntimeRemote,
			Env: []types.MCPEnv{{
				Key: "REGION", Required: true,
				Options: []types.MCPConfigurationOption{{Name: "US", Value: "us"}}}},
			RemoteConfig: &types.RemoteRuntimeConfig{IsTemplate: true, URLTemplate: "https://${REGION}.example.com/mcp"},
		},
	}}}

	err := applyRemoteURLTemplateToWebhookValidation(t.Context(), validation, map[string]string{"REGION": "forged"}, mcp.ValidationOptions{})
	if err == nil || !strings.Contains(err.Error(), "not one of the configured options") {
		t.Fatalf("expected invalid option error, got %v", err)
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
