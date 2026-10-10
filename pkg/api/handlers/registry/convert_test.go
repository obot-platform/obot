package registry

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestConvertMCPServerCatalogEntryToRegistryRemoteFixedURLRequiresVMCP(t *testing.T) {
	entry := registryTestCatalogEntry(types.RemoteCatalogConfig{
		FixedURL: "https://api.example.com/mcp",
	})

	got, err := ConvertMCPServerCatalogEntryToRegistry(t.Context(), entry, "https://obot.example.com", "com.example.obot", newMimeFetcher())
	if err != nil {
		t.Fatal(err)
	}

	if got.Meta.Obot == nil || !got.Meta.Obot.ConfigurationRequired {
		t.Fatalf("expected fixed URL entry to require a vMCP, got obot meta %#v", got.Meta.Obot)
	}
	if len(got.Server.Remotes) != 0 {
		t.Fatalf("remote count = %d, want 0", len(got.Server.Remotes))
	}
}

func TestConvertMCPServerCatalogEntryToRegistryRemoteHostnameRequiresConfiguration(t *testing.T) {
	entry := registryTestCatalogEntry(types.RemoteCatalogConfig{
		Hostname: "api.example.com",
	})

	got, err := ConvertMCPServerCatalogEntryToRegistry(t.Context(), entry, "https://obot.example.com", "com.example.obot", newMimeFetcher())
	if err != nil {
		t.Fatal(err)
	}

	if got.Meta.Obot == nil || !got.Meta.Obot.ConfigurationRequired {
		t.Fatalf("expected hostname entry to require configuration, got obot meta %#v", got.Meta.Obot)
	}
	if len(got.Server.Remotes) != 0 {
		t.Fatalf("expected no remotes for hostname entry, got %#v", got.Server.Remotes)
	}
}

func TestConvertMCPServerCatalogEntryToRegistryRemoteURLTemplateRequiresConfiguration(t *testing.T) {
	entry := registryTestCatalogEntry(types.RemoteCatalogConfig{
		URLTemplate: "https://${WORKSPACE}.example.com/mcp",
	})

	got, err := ConvertMCPServerCatalogEntryToRegistry(t.Context(), entry, "https://obot.example.com", "com.example.obot", newMimeFetcher())
	if err != nil {
		t.Fatal(err)
	}

	if got.Meta.Obot == nil || !got.Meta.Obot.ConfigurationRequired {
		t.Fatalf("expected URL template entry to require configuration, got obot meta %#v", got.Meta.Obot)
	}
	if len(got.Server.Remotes) != 0 {
		t.Fatalf("expected no remotes for URL template entry, got %#v", got.Server.Remotes)
	}
}

func TestConvertMCPServerCatalogEntryToRegistryRemoteStaticOAuthRequiresConfigurationUntilConfigured(t *testing.T) {
	entry := registryTestCatalogEntry(types.RemoteCatalogConfig{
		FixedURL:            "https://api.example.com/mcp",
		StaticOAuthRequired: true,
	})

	got, err := ConvertMCPServerCatalogEntryToRegistry(t.Context(), entry, "https://obot.example.com", "com.example.obot", newMimeFetcher())
	if err != nil {
		t.Fatal(err)
	}

	if got.Meta.Obot == nil || !got.Meta.Obot.ConfigurationRequired {
		t.Fatalf("expected unconfigured static OAuth entry to require configuration, got obot meta %#v", got.Meta.Obot)
	}
	if len(got.Server.Remotes) != 0 {
		t.Fatalf("expected no remotes for unconfigured static OAuth entry, got %#v", got.Server.Remotes)
	}

	entry.Status.OAuthCredentialConfigured = true
	got, err = ConvertMCPServerCatalogEntryToRegistry(t.Context(), entry, "https://obot.example.com", "com.example.obot", newMimeFetcher())
	if err != nil {
		t.Fatal(err)
	}

	if got.Meta.Obot == nil || !got.Meta.Obot.ConfigurationRequired {
		t.Fatalf("expected configured static OAuth entry to require a vMCP, got obot meta %#v", got.Meta.Obot)
	}
	if len(got.Server.Remotes) != 0 {
		t.Fatalf("remote count = %d, want 0", len(got.Server.Remotes))
	}
}

func TestConvertMCPServerToRegistryNeedsURLRequiresConfiguration(t *testing.T) {
	server := v1.MCPServer{
		Name: "ms1needsurl",
		Spec: v1.MCPServerSpec{
			UserID:   "user-1",
			NeedsURL: true,
			Manifest: types.MCPServerManifest{
				Name:    "Needs URL",
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Hostname: "api.example.com",
				},
			},
		},
	}

	got, err := ConvertMCPServerToRegistry(t.Context(), server, nil, "https://obot.example.com", server.Name, "com.example.obot", newMimeFetcher())
	if err != nil {
		t.Fatal(err)
	}

	if got.Meta.Obot == nil || !got.Meta.Obot.ConfigurationRequired {
		t.Fatalf("expected server needing URL to require configuration, got obot meta %#v", got.Meta.Obot)
	}
	if len(got.Server.Remotes) != 0 {
		t.Fatalf("expected no remotes for server needing URL, got %#v", got.Server.Remotes)
	}
}

func TestConvertMCPServerToRegistryConfiguredServersRequireVMCP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		userID string
	}{
		{
			name: "shared",
		},
		{
			name:   "personal",
			userID: "user-1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := v1.MCPServer{
				Name: "ms1configured",
				Spec: v1.MCPServerSpec{
					UserID: tc.userID,
					Manifest: types.MCPServerManifest{
						Name:    "Configured server",
						Runtime: types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{
							URL: "https://api.example.com/mcp",
						},
					},
				},
			}
			got, err := ConvertMCPServerToRegistry(t.Context(), server, nil, "https://obot.example.com", server.Name, "com.example.obot", newMimeFetcher())
			if err != nil {
				t.Fatal(err)
			}
			if got.Meta.Obot == nil || !got.Meta.Obot.ConfigurationRequired {
				t.Fatalf("expected configured server to require a vMCP, got obot meta %#v", got.Meta.Obot)
			}
			if len(got.Server.Remotes) != 0 {
				t.Fatalf("expected no direct connection URLs, got remotes %#v", got.Server.Remotes)
			}
		})
	}
}

func registryTestCatalogEntry(remoteConfig types.RemoteCatalogConfig) v1.MCPServerCatalogEntry {
	return v1.MCPServerCatalogEntry{
		Name: "remote-entry",
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:        "Remote Entry",
				Description: "Remote entry",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL:            remoteConfig.FixedURL,
					Hostname:            remoteConfig.Hostname,
					URLTemplate:         remoteConfig.URLTemplate,
					StaticOAuthRequired: remoteConfig.StaticOAuthRequired,
				},
			},
		},
	}
}
