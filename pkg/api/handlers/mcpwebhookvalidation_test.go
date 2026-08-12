package handlers

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			name: "all devices with surfaces and v1",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.Resource{{
					Type: types.ResourceTypeDevice,
					ID:   "*",
				}},
				DeviceSurfaces: []types.FilterSurface{
					types.FilterSurfaceToolResponse,
					types.FilterSurfaceUserPrompt,
				},
				ContractVersion: types.FilterContractVersionV1,
			},
		},
		{
			name: "all devices needs surfaces",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.Resource{{
					Type: types.ResourceTypeDevice,
					ID:   "*",
				}},
				ContractVersion: types.FilterContractVersionV1,
			},
			wantErr: true,
		},
		{
			name: "surfaces need all devices",
			manifest: types.MCPWebhookValidationManifest{
				URL:            "https://example.com/webhook",
				DeviceSurfaces: []types.FilterSurface{types.FilterSurfaceUserPrompt},
			},
			wantErr: true,
		},
		{
			name: "all devices requires v1",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.Resource{{
					Type: types.ResourceTypeDevice,
					ID:   "*",
				}},
				DeviceSurfaces: []types.FilterSurface{
					types.FilterSurfaceUserPrompt,
				},
				ContractVersion: types.FilterContractVersionLegacyMCP,
			},
			wantErr: true,
		},
		{
			name: "invalid device id",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.Resource{{
					Type: types.ResourceTypeDevice,
					ID:   "device-1",
				}},
				DeviceSurfaces: []types.FilterSurface{
					types.FilterSurfaceUserPrompt,
				},
				ContractVersion: types.FilterContractVersionV1,
			},
			wantErr: true,
		},
		{
			name: "duplicate device resource",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.Resource{
					{
						Type: types.ResourceTypeDevice,
						ID:   "*",
					},
					{
						Type: types.ResourceTypeDevice,
						ID:   "*",
					},
				},
				DeviceSurfaces: []types.FilterSurface{
					types.FilterSurfaceUserPrompt,
				},
				ContractVersion: types.FilterContractVersionV1,
			},
			wantErr: true,
		},
		{
			name: "duplicate surface",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.Resource{{
					Type: types.ResourceTypeDevice,
					ID:   "*",
				}},
				DeviceSurfaces: []types.FilterSurface{
					types.FilterSurfaceUserPrompt,
					types.FilterSurfaceUserPrompt,
				},
				ContractVersion: types.FilterContractVersionV1,
			},
			wantErr: true,
		},
		{
			name: "unknown surface",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				Resources: []types.Resource{{
					Type: types.ResourceTypeDevice,
					ID:   "*",
				}},
				DeviceSurfaces: []types.FilterSurface{
					"unknown",
				},
				ContractVersion: types.FilterContractVersionV1,
			},
			wantErr: true,
		},
		{
			name: "unknown contract",
			manifest: types.MCPWebhookValidationManifest{
				URL:             "https://example.com/webhook",
				ContractVersion: "future/v2",
			},
			wantErr: true,
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
			err := validateManifest(t.Context(), &tt.manifest, mcp.ValidationOptions{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.manifest.AppliesToDevices() &&
				!slices.IsSortedFunc(tt.manifest.DeviceSurfaces, func(a, b types.FilterSurface) int {
					return slices.Index(types.KnownFilterSurfaces(), a) - slices.Index(types.KnownFilterSurfaces(), b)
				}) {
				t.Fatalf("device surfaces were not normalized: %v", tt.manifest.DeviceSurfaces)
			}
		})
	}
}

func TestDefaultFilterContractVersion(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.MCPWebhookValidationManifest
		fallback types.FilterContractVersion
		want     types.FilterContractVersion
	}{
		{
			name:     "new custom",
			fallback: types.FilterContractVersionV1,
			want:     types.FilterContractVersionV1,
		},
		{
			name:     "legacy update",
			fallback: types.FilterContractVersionLegacyMCP,
			want:     types.FilterContractVersionLegacyMCP,
		},
		{
			name: "device selection upgrades omission",
			manifest: types.MCPWebhookValidationManifest{
				Resources: []types.Resource{{
					Type: types.ResourceTypeDevice,
					ID:   "*",
				}},
			},
			fallback: types.FilterContractVersionLegacyMCP,
			want:     types.FilterContractVersionV1,
		},
		{
			name: "explicit value preserved",
			manifest: types.MCPWebhookValidationManifest{
				ContractVersion: types.FilterContractVersionLegacyMCP,
			},
			fallback: types.FilterContractVersionV1,
			want:     types.FilterContractVersionLegacyMCP,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultFilterContractVersion(&tt.manifest, tt.fallback)
			if tt.manifest.ContractVersion != tt.want {
				t.Fatalf("contract version = %q, want %q", tt.manifest.ContractVersion, tt.want)
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

func TestResolveManifestFromCatalogEntryCopiesV1ContractMetadata(t *testing.T) {
	entry := &v1.SystemMCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "pii-filter", Namespace: system.DefaultNamespace},
		Spec: v1.SystemMCPServerCatalogEntrySpec{Manifest: types.SystemMCPServerCatalogEntryManifest{
			Name:                "PII Filter",
			SystemMCPServerType: types.SystemMCPServerTypeFilter,
			FilterConfig: &types.FilterConfig{
				ToolName:        "filter_pii",
				ContractVersion: types.FilterContractVersionV1,
			},
			Runtime:        types.RuntimeRemote,
			ServerUserType: types.ServerUserTypeSingleUser,
			RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
		}},
	}
	req := api.Context{
		Request:        httptest.NewRequest(http.MethodPost, "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Storage: storage.Client(fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(entry).
			Build()),
	}
	manifest := types.MCPWebhookValidationManifest{SystemMCPServerCatalogEntryID: entry.Name}

	if err := (&MCPWebhookValidationHandler{mcpSessionManager: &mcp.SessionManager{}}).resolveManifestFromCatalogEntry(req, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ContractVersion != types.FilterContractVersionV1 || manifest.ToolName != "filter_pii" {
		t.Fatalf("catalog metadata not resolved: %#v", manifest)
	}
}
