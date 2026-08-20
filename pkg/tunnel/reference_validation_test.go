package tunnel

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateCatalogEntryTunnelReferences(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.MCPTunnel{
				Name:      "mcptunnel-office",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPTunnelSpec{
					Manifest: types.MCPTunnelManifest{
						DisplayName: "Office",
						AllowedURLs: []string{
							"https://fixed.example.com/mcp",
							"*.internal.example",
						},
					},
				},
			},
			&v1.MCPTunnel{
				Name:      "mcptunnel-legacy",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPTunnelSpec{
					Manifest: types.MCPTunnelManifest{
						AllowedURLs: []string{"https://fixed.example.com/mcp"},
					},
				},
			},
		).
		Build()

	tests := []struct {
		name           string
		manifest       types.MCPServerCatalogEntryManifest
		wantErr        string
		notWantErrText string
	}{
		{
			name: "allows exact fixed URL",
			manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
				FixedURL:   "https://fixed.example.com/mcp",
				TunnelName: "mcptunnel-office",
			}),
		},
		{
			name: "allows hostname suffix",
			manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
				Hostname:   "api.internal.example",
				TunnelName: "mcptunnel-office",
			}),
		},
		{
			name: "rejects disallowed fixed URL",
			manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
				FixedURL:   "https://other.example.com/mcp",
				TunnelName: "mcptunnel-office",
			}),
			wantErr:        `MCP tunnel "Office" does not allow target`,
			notWantErrText: "mcptunnel-office",
		},
		{
			name: "rejects URL template",
			manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
				URLTemplate: "https://${HOST}/mcp",
				TunnelName:  "mcptunnel-office",
			}),
			wantErr: "cannot be used",
		},
		{
			name: "rejects unknown tunnel",
			manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
				FixedURL:   "https://fixed.example.com/mcp",
				TunnelName: "mcptunnel-missing",
			}),
			wantErr: "failed to get MCP tunnel",
		},
		{
			name: "uses tunnel ID when display name is not set",
			manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
				FixedURL:   "https://fixed.example.com/mcp",
				TunnelName: "mcptunnel-legacy",
			}),
			wantErr: `MCP tunnel "mcptunnel-legacy" is invalid`,
		},
		{
			name: "rejects padded tunnel name",
			manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
				FixedURL:   "https://fixed.example.com/mcp",
				TunnelName: " mcptunnel-office ",
			}),
			wantErr: "leading or trailing whitespace",
		},
		{
			name: "validates composite component recursively",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{{
						CatalogEntryID: "remote",
						Manifest: remoteCatalogTunnelManifest(types.RemoteCatalogConfig{
							FixedURL:   "https://other.example.com/mcp",
							TunnelName: "mcptunnel-office",
						}),
					}},
				},
			},
			wantErr: "componentServers[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCatalogEntryTunnelReferences(t.Context(), client, tt.manifest)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
			}
			if tt.notWantErrText != "" && strings.Contains(err.Error(), tt.notWantErrText) {
				t.Fatalf("error = %v, do not want error containing %q", err, tt.notWantErrText)
			}
		})
	}
}

func TestValidateServerTunnelReferences(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.MCPTunnel{
			Name:      "mcptunnel-office",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPTunnelSpec{
				Manifest: types.MCPTunnelManifest{
					DisplayName: "Office",
					AllowedURLs: []string{"*.internal.example"},
				},
			},
		}).
		Build()

	err := ValidateServerTunnelReferences(t.Context(), client, types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{
			URL:        "http://api.internal.example:8080/mcp",
			TunnelName: "mcptunnel-office",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func remoteCatalogTunnelManifest(config types.RemoteCatalogConfig) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Runtime:        types.RuntimeRemote,
		ServerUserType: types.ServerUserTypeSingleUser,
		RemoteConfig:   &config,
	}
}
