package registry

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertVMCPToRegistry(t *testing.T) {
	created := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	definition := v1.VMCP{
		Name:              "vmcp1example",
		CreationTimestamp: metav1.NewTime(created),
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				DisplayName: "Example vMCP",
				Description: "An example.\n\n## Features\n\nMultiple components.",
				Icon:        "https://example.com/icon.svg",
			},
		},
	}
	got := ConvertVMCPToRegistry(definition, "https://obot.example.com/", "com.example.obot")
	if got.Server.Name != "com.example.obot/vmcp1example" || got.Server.Title != "Example vMCP" {
		t.Fatalf("unexpected registry identity: %#v", got.Server)
	}
	if got.Server.Description != definition.Spec.Manifest.Description || got.Server.Meta.PublisherProvided.GitHub.Readme != definition.Spec.Manifest.Description {
		t.Fatalf("description and README must come from the vMCP: %#v", got.Server)
	}
	if len(got.Server.Icons) != 1 || got.Server.Icons[0].Src != definition.Spec.Manifest.Icon || got.Server.Icons[0].MimeType != "image/svg+xml" {
		t.Fatalf("unexpected icons: %#v", got.Server.Icons)
	}
	if got.Server.Version != "latest" || got.Server.Schema == "" || !got.Meta.Official.IsLatest || got.Meta.Official.Status != "active" {
		t.Fatalf("unexpected registry metadata: %#v", got)
	}
	if got.Meta.Official.CreatedAt != created.Format(time.RFC3339) || got.CreatedAtUnix != created.Unix() {
		t.Fatalf("unexpected creation time: %#v", got)
	}
	assertRegistryRemote(t, got, definition.Name)

	definition.Spec.Manifest.DisplayName = ""
	definition.Spec.Manifest.Icon = ""
	got = ConvertVMCPToRegistry(definition, "https://obot.example.com", "com.example.obot")
	if got.Server.Title != definition.Name || len(got.Server.Icons) != 0 {
		t.Fatalf("unexpected metadata fallbacks: %#v", got.Server)
	}
}

func TestRegistryDescriptionFitsSchema(t *testing.T) {
	for _, tc := range []struct {
		name        string
		description string
		displayName string
		want        string
	}{
		{
			name:        "long Unicode description",
			description: strings.Repeat("天", 120),
			want:        strings.Repeat("天", 100),
		},
		{
			name:        "empty description falls back to display name",
			description: " \n ",
			displayName: "Weather Tools",
			want:        "Weather Tools",
		},
		{
			name: "empty draft falls back to ID",
			want: "vmcp1example",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := registryDefinition("vmcp1example", "")
			definition.Spec.Manifest.DisplayName = tc.displayName
			definition.Spec.Manifest.Description = tc.description
			got := ConvertVMCPToRegistry(*definition, "https://obot.example.com", "com.example.obot")
			if got.Server.Description != tc.want || !utf8.ValidString(got.Server.Description) {
				t.Fatalf("description = %q, want %q", got.Server.Description, tc.want)
			}
			if got.Server.Meta.PublisherProvided.GitHub.Readme != tc.description {
				t.Fatal("README must retain the complete description")
			}
		})
	}
}

func TestRegistryURLIndependentOfConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		components []types.VMCPComponent
		ready      bool
	}{
		{
			name: "empty draft",
		},
		{
			name: "requires user configuration",
			components: []types.VMCPComponent{
				{
					ID:   "component-1",
					Name: "Remote",
					CatalogEntry: types.MCPServerCatalogEntrySnapshot{
						Manifest: types.MCPServerCatalogEntryManifest{
							Config: []types.MCPConfig{
								{
									Key:       "API_KEY",
									Required:  true,
									Sensitive: true,
								},
							},
						},
					},
					Configuration: []types.VMCPConfigurationPolicy{
						{
							Key:    "API_KEY",
							Policy: types.VMCPConfigurationPolicyUserAllowed,
						},
					},
				},
			},
		},
		{
			name: "configured and ready",
			components: []types.VMCPComponent{
				{
					ID: "component-1",
				},
				{
					ID: "component-2",
				},
			},
			ready: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := v1.VMCP{
				ObjectMeta: registryObjectMeta("vmcp1example"),
				Spec: v1.VMCPSpec{
					Manifest: types.VMCPManifest{Components: tc.components},
				},
				Status: v1.VMCPStatus{Ready: tc.ready},
			}
			got := ConvertVMCPToRegistry(definition, "https://obot.example.com", "com.example.obot")
			assertRegistryRemote(t, got, definition.Name)
			if got.Meta.Obot != nil || strings.Contains(got.Server.Meta.PublisherProvided.GitHub.Readme, "Add this server to a vMCP") {
				t.Fatalf("setup must be handled by consent: %#v", got)
			}
		})
	}
}

func assertRegistryRemote(t *testing.T, got types.RegistryServerResponse, id string) {
	t.Helper()
	if len(got.Server.Remotes) != 1 || got.Server.Remotes[0].Type != "streamable-http" || got.Server.Remotes[0].URL != "https://obot.example.com/mcp-connect/"+id {
		t.Fatalf("unexpected remotes: %#v", got.Server.Remotes)
	}
}

// Localhost callback setup must go through vMCP consent for both shared and
// dedicated components; registry clients must never connect to the upstream URL.
func TestLocalhostCallbackUsesVMCPRemote(t *testing.T) {
	for _, tc := range []struct {
		name            string
		forceSingleUser bool
	}{
		{
			name: "shared component",
		},
		{
			name:            "dedicated component",
			forceSingleUser: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := registryDefinition("vmcp1callback", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
			definition.Spec.Manifest.Components = []types.VMCPComponent{
				{
					ID:              "component-1",
					ForceSingleUser: tc.forceSingleUser,
					CatalogEntry: types.MCPServerCatalogEntrySnapshot{
						Manifest: types.MCPServerCatalogEntryManifest{
							Runtime: types.RuntimeRemote,
							RemoteConfig: &types.RemoteCatalogConfig{
								FixedURL:                 "https://example.com/mcp",
								LocalhostCallbackEnabled: true,
							},
						},
					},
				},
			}
			got := ConvertVMCPToRegistry(*definition, "https://obot.example.com", "com.example.obot")
			assertRegistryRemote(t, got, definition.Name)
		})
	}
}
