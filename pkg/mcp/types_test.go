package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestLegacyServerToServerConfig_StaticHeaders(t *testing.T) {
	tests := []struct {
		name            string
		headers         []types.MCPHeader
		credEnv         map[string]string
		expectedHeaders []string
		expectedMissing []string
	}{
		{
			name: "static header only",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "user-configurable header only",
			headers: []types.MCPHeader{
				{Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "mixed static and user-configurable",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
				{Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"Authorization=Bearer static-token", "X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "missing required user-configurable header",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
				{Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{"X-API-Key"},
		},
		{
			name: "optional user-configurable header missing",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
				{Key: "X-Optional", Required: false},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "static header overrides credential",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "empty static value falls back to credential",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer user-token"},
			expectedMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{
						Headers: tt.headers,
					},
				},
			}
			mcpServer.Name = "test-server"

			config, missing, err := legacyServerToServerConfig(mcpServer, "test-scope", tt.credEnv, map[string]struct{}{})

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare headers
			if len(config.Headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d: expected %v, got %v", len(tt.expectedHeaders), len(config.Headers), tt.expectedHeaders, config.Headers)
			} else {
				for i, expected := range tt.expectedHeaders {
					if config.Headers[i] != expected {
						t.Errorf("header %d: expected %s, got %s", i, expected, config.Headers[i])
					}
				}
			}

			// Compare missing headers
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing headers, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing header %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}
		})
	}
}

func TestServerToServerConfig_StaticHeaders_Remote(t *testing.T) {
	tests := []struct {
		name            string
		headers         []types.MCPHeader
		credEnv         map[string]string
		expectedHeaders []string
		expectedMissing []string
	}{
		{
			name: "static header only",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "user-configurable header only",
			headers: []types.MCPHeader{
				{Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "mixed static and user-configurable",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
				{Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"Authorization=Bearer static-token", "X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "missing required user-configurable header",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
				{Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{"X-API-Key"},
		},
		{
			name: "optional user-configurable header missing",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
				{Key: "X-Optional", Required: false},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "static header overrides credential",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "Bearer static-token"},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "empty static value falls back to credential",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer user-token"},
			expectedMissing: []string{},
		},
		{
			name: "empty credential value is ignored",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "", Required: true},
			},
			credEnv:         map[string]string{"Authorization": ""},
			expectedHeaders: []string{},
			expectedMissing: []string{"Authorization"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{
						Runtime: types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{
							URL:     "https://example.com/mcp",
							Headers: tt.headers,
						},
					},
				},
			}
			mcpServer.Name = "test-server"

			config, missing, err := ServerToServerConfig(mcpServer, "test-scope", tt.credEnv)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare headers
			if len(config.Headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d: expected %v, got %v", len(tt.expectedHeaders), len(config.Headers), tt.expectedHeaders, config.Headers)
			} else {
				for i, expected := range tt.expectedHeaders {
					if config.Headers[i] != expected {
						t.Errorf("header %d: expected %s, got %s", i, expected, config.Headers[i])
					}
				}
			}

			// Compare missing headers
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing headers, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing header %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}

			// Verify the URL was set correctly
			if config.URL != "https://example.com/mcp" {
				t.Errorf("expected URL https://example.com/mcp, got %s", config.URL)
			}

			// Verify the runtime was set correctly
			if config.Runtime != types.RuntimeRemote {
				t.Errorf("expected runtime %v, got %v", types.RuntimeRemote, config.Runtime)
			}
		})
	}
}

func TestServerToServerConfig_StaticHeaders_EdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		manifest        types.MCPServerManifest
		credEnv         map[string]string
		expectedHeaders []string
		expectedMissing []string
		expectError     bool
	}{
		{
			name: "header with special characters in value",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "Authorization", Value: "Bearer token-with-special!@#$%^&*()characters"},
					},
				},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer token-with-special!@#$%^&*()characters"},
			expectedMissing: []string{},
			expectError:     false,
		},
		{
			name: "nil remote config should return error",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: nil,
			},
			credEnv:     map[string]string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: tt.manifest,
				},
			}
			mcpServer.Name = "test-server"

			config, missing, err := ServerToServerConfig(mcpServer, "test-scope", tt.credEnv)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare headers
			if len(config.Headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d: expected %v, got %v", len(tt.expectedHeaders), len(config.Headers), tt.expectedHeaders, config.Headers)
			} else {
				for i, expected := range tt.expectedHeaders {
					if config.Headers[i] != expected {
						t.Errorf("header %d: expected %s, got %s", i, expected, config.Headers[i])
					}
				}
			}

			// Compare missing headers
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing headers, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing header %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}
		})
	}
}
