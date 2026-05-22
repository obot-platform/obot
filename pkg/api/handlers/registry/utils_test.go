package registry

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestReverseDNSFromURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		expected    string
		shouldError bool
	}{
		{
			name:     "standard domain",
			baseURL:  "https://obot.example.com",
			expected: "com.example.obot",
		},
		{
			name:     "subdomain",
			baseURL:  "https://app.obot.example.com",
			expected: "com.example.obot.app",
		},
		{
			name:     "localhost",
			baseURL:  "http://localhost:8080",
			expected: "local.localhost",
		},
		{
			name:     "IP address",
			baseURL:  "http://192.168.1.100",
			expected: "local.192-168-1-100",
		},
		{
			name:     "single label domain",
			baseURL:  "http://obot",
			expected: "obot",
		},
		{
			name:     "with port",
			baseURL:  "https://obot.ai:443",
			expected: "ai.obot",
		},
		{
			name:        "invalid URL",
			baseURL:     "not a url",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ReverseDNSFromURL(tt.baseURL)
			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("ReverseDNSFromURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatRegistryServerName(t *testing.T) {
	tests := []struct {
		name       string
		reverseDNS string
		serverName string
		expected   string
	}{
		{
			name:       "standard names",
			reverseDNS: "ai.obot",
			serverName: "filesystem",
			expected:   "ai.obot/filesystem",
		},
		{
			name:       "name with special chars",
			reverseDNS: "com.example",
			serverName: "My_Server-123",
			expected:   "com.example/my-server-123",
		},
		{
			name:       "name with spaces",
			reverseDNS: "io.github",
			serverName: "Weather API Server",
			expected:   "io.github/weather-api-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRegistryServerName(tt.reverseDNS, tt.serverName)
			if result != tt.expected {
				t.Errorf("FormatRegistryServerName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHideMultiUserTemplateFromRegistry(t *testing.T) {
	multiUserEntry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeMultiUser,
			},
		},
	}
	singleUserEntry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeSingleUser,
			},
		},
	}

	tests := []struct {
		name  string
		user  kuser.Info
		entry v1.MCPServerCatalogEntry
		want  bool
	}{
		{
			name:  "basic user cannot see multi-user templates",
			user:  &kuser.DefaultInfo{Groups: types.RoleBasic.Groups()},
			entry: multiUserEntry,
			want:  true,
		},
		{
			name:  "basic user can see single-user entries",
			user:  &kuser.DefaultInfo{Groups: types.RoleBasic.Groups()},
			entry: singleUserEntry,
			want:  false,
		},
		{
			name:  "admin can see multi-user templates",
			user:  &kuser.DefaultInfo{Groups: types.RoleAdmin.Groups()},
			entry: multiUserEntry,
			want:  false,
		},
		{
			name:  "power user plus can see multi-user templates",
			user:  &kuser.DefaultInfo{Groups: types.RolePowerUserPlus.Groups()},
			entry: multiUserEntry,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handlers.HideMultiUserTemplate(api.Context{User: tt.user}, tt.entry)
			if got != tt.want {
				t.Errorf("HideMultiUserTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}
