package types

import (
	"strings"
	"testing"
)

func TestMCPTunnelManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest MCPTunnelManifest
		wantErr  string
	}{
		{
			name:     "no allowed URLs",
			manifest: MCPTunnelManifest{DisplayName: "Office"},
		},
		{
			name: "exact prefix and suffix",
			manifest: MCPTunnelManifest{
				DisplayName: "Office",
				AllowedURLs: []string{
					"https://api.example.com/mcp",
					"https://internal.example.com/*",
					"*.corp.example.com",
				},
			},
		},
		{
			name:     "missing display name",
			manifest: MCPTunnelManifest{},
			wantErr:  "displayName is required",
		},
		{
			name: "empty allowed URL",
			manifest: MCPTunnelManifest{
				DisplayName: "Office",
				AllowedURLs: []string{" "},
			},
			wantErr: "must not be empty",
		},
		{
			name: "match all wildcard",
			manifest: MCPTunnelManifest{
				DisplayName: "Office",
				AllowedURLs: []string{"*"},
			},
		},
		{
			name: "wildcard in middle",
			manifest: MCPTunnelManifest{
				DisplayName: "Office",
				AllowedURLs: []string{"api.*.example.com"},
			},
			wantErr: "beginning or end",
		},
		{
			name: "multiple wildcards",
			manifest: MCPTunnelManifest{
				DisplayName: "Office",
				AllowedURLs: []string{"*example.com*"},
			},
			wantErr: "at most one wildcard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestMCPTunnelManifestAllowsURL(t *testing.T) {
	manifest := MCPTunnelManifest{
		DisplayName: "Office",
		AllowedURLs: []string{
			"https://fixed.example.com/mcp",
			"https://prefix.example.com/",
			"https://prefix.example.com/*",
			"*/suffix",
			"internal.example.com",
			"*.corp.example.com",
			"build-*",
		},
	}

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "exact full URL",
			target: "https://fixed.example.com/mcp",
			want:   true,
		},
		{
			name:   "full URL host matching is case insensitive",
			target: "https://FIXED.EXAMPLE.COM/mcp",
			want:   true,
		},
		{
			name:   "full URL default port is canonicalized",
			target: "https://fixed.example.com:443/mcp",
			want:   true,
		},
		{
			name:   "full URL prefix",
			target: "https://prefix.example.com/mcp",
			want:   true,
		},
		{
			name:   "full URL suffix",
			target: "https://another.example.com/suffix",
			want:   true,
		},
		{
			name:   "exact hostname",
			target: "https://internal.example.com/mcp",
			want:   true,
		},
		{
			name:   "hostname suffix",
			target: "https://api.dev.corp.example.com/mcp",
			want:   true,
		},
		{
			name:   "hostname prefix",
			target: "http://build-runner/mcp",
			want:   true,
		},
		{
			name:   "hostname matching is case insensitive",
			target: "https://INTERNAL.EXAMPLE.COM/mcp",
			want:   true,
		},
		{
			name:   "direct hostname candidate",
			target: "internal.example.com",
			want:   true,
		},
		{
			name:   "hostname suffix does not match bare suffix",
			target: "https://corp.example.com/mcp",
		},
		{
			name:   "hostname suffix does not match URL path",
			target: "https://public.example.com/mcp/.corp.example.com",
		},
		{
			name:   "different URL",
			target: "https://public.example.com/mcp",
		},
		{
			name: "empty target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manifest.AllowsURL(tt.target); got != tt.want {
				t.Fatalf("AllowsURL(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestMCPTunnelManifestAllowsAnyURL(t *testing.T) {
	manifest := MCPTunnelManifest{
		DisplayName: "Office",
		AllowedURLs: []string{"*"},
	}
	if !manifest.AllowsURL("https://any.example.com/mcp") {
		t.Fatal(`AllowsURL() = false with "*" allowed URL`)
	}
}
