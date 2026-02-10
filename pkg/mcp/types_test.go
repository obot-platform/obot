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

			config, missing, err := legacyServerToServerConfig(mcpServer, "test-user-id", "test-scope", tt.credEnv, nil)

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
	baseURL := "http://localhost:8080"
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

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), baseURL, "test-user-id", "test-scope", "test-catalog", tt.credEnv, nil)

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

			if config.Issuer != baseURL {
				t.Errorf("expected issuer %s, got %s", baseURL, config.Issuer)
			}

			// Verify the audiences were set correctly
			expectedAudiences := mcpServer.ValidConnectURLs(baseURL)
			if len(config.Audiences) != len(expectedAudiences) {
				t.Errorf("expected %d audiences, got %d: expected %v, got %v", len(expectedAudiences), len(config.Audiences), expectedAudiences, config.Audiences)
			} else {
				for i, expected := range expectedAudiences {
					if config.Audiences[i] != expected {
						t.Errorf("audience %d: expected %s, got %s", i, expected, config.Audiences[i])
					}
				}
			}
		})
	}
}

func TestServerToServerConfig_WithPrefix(t *testing.T) {
	baseURL := "http://localhost:8080"
	tests := []struct {
		name            string
		headers         []types.MCPHeader
		env             []types.MCPEnv
		credEnv         map[string]string
		expectedHeaders []string
		expectedEnv     []string
		expectedMissing []string
	}{
		{
			name: "header with prefix applied to user value",
			headers: []types.MCPHeader{
				{Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "my-token"},
			expectedHeaders: []string{"Authorization=Bearer my-token"},
			expectedMissing: []string{},
		},
		{
			name: "header with prefix not applied to static value",
			headers: []types.MCPHeader{
				{Key: "Authorization", Value: "static-token", Prefix: "Bearer "},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=static-token"},
			expectedMissing: []string{},
		},
		{
			name: "env var with Bearer prefix",
			env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "API_KEY", Prefix: "Bearer ", Required: true}},
			},
			credEnv:         map[string]string{"API_KEY": "secret-key-123"},
			expectedEnv:     []string{"API_KEY=Bearer secret-key-123"},
			expectedMissing: []string{},
		},
		{
			name: "env var with sk- prefix (OpenAI style)",
			env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "OPENAI_API_KEY", Prefix: "sk-", Required: true}},
			},
			credEnv:         map[string]string{"OPENAI_API_KEY": "proj-abc123xyz"},
			expectedEnv:     []string{"OPENAI_API_KEY=sk-proj-abc123xyz"},
			expectedMissing: []string{},
		},
		{
			name: "multiple headers and env vars with different prefixes",
			headers: []types.MCPHeader{
				{Key: "Authorization", Prefix: "Bearer ", Required: true},
				{Key: "X-API-Key", Prefix: "Key ", Required: true},
			},
			env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "TOKEN", Prefix: "Token ", Required: true}},
				{MCPHeader: types.MCPHeader{Key: "SECRET", Required: true}}, // No prefix
			},
			credEnv: map[string]string{
				"Authorization": "auth-token",
				"X-API-Key":     "api-key-value",
				"TOKEN":         "token-value",
				"SECRET":        "secret-value",
			},
			expectedHeaders: []string{"Authorization=Bearer auth-token", "X-API-Key=Key api-key-value"},
			expectedEnv:     []string{"TOKEN=Token token-value", "SECRET=secret-value"},
			expectedMissing: []string{},
		},
		{
			name: "prefix not applied when value is empty",
			headers: []types.MCPHeader{
				{Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{},
			expectedMissing: []string{"Authorization"},
		},
		{
			name: "prefix not duplicated when user already included it in header",
			headers: []types.MCPHeader{
				{Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "Bearer my-token"},
			expectedHeaders: []string{"Authorization=Bearer my-token"},
			expectedMissing: []string{},
		},
		{
			name: "prefix not duplicated when user already included it in env var",
			env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "API_KEY", Prefix: "sk-", Required: true}},
			},
			credEnv:         map[string]string{"API_KEY": "sk-proj-abc123"},
			expectedEnv:     []string{"API_KEY=sk-proj-abc123"},
			expectedMissing: []string{},
		},
		{
			name: "mixed - some with prefix already included, some without",
			headers: []types.MCPHeader{
				{Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "API_KEY", Prefix: "sk-", Required: true}},
				{MCPHeader: types.MCPHeader{Key: "TOKEN", Prefix: "Token ", Required: true}},
			},
			credEnv: map[string]string{
				"Authorization": "Bearer already-has-it",
				"API_KEY":       "proj-needs-it",
				"TOKEN":         "Token already-has-it",
			},
			expectedHeaders: []string{"Authorization=Bearer already-has-it"},
			expectedEnv:     []string{"API_KEY=sk-proj-needs-it", "TOKEN=Token already-has-it"},
			expectedMissing: []string{},
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
						Env: tt.env,
					},
				},
			}
			mcpServer.Name = "test-server"

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), baseURL, "test-user-id", "test-scope", "test-catalog", tt.credEnv, nil)

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

			// Compare env vars
			if len(config.Env) != len(tt.expectedEnv) {
				t.Errorf("expected %d env vars, got %d: expected %v, got %v", len(tt.expectedEnv), len(config.Env), tt.expectedEnv, config.Env)
			} else {
				for i, expected := range tt.expectedEnv {
					if config.Env[i] != expected {
						t.Errorf("env var %d: expected %s, got %s", i, expected, config.Env[i])
					}
				}
			}

			// Compare missing
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing items, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing item %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}
		})
	}
}

func TestLegacyServerToServerConfig_WithPrefix(t *testing.T) {
	tests := []struct {
		name            string
		headers         []types.MCPHeader
		env             []types.MCPEnv
		credEnv         map[string]string
		expectedHeaders []string
		expectedEnv     []string
		expectedMissing []string
	}{
		{
			name: "legacy header with prefix applied to user value",
			headers: []types.MCPHeader{
				{Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "my-token"},
			expectedHeaders: []string{"Authorization=Bearer my-token"},
			expectedMissing: []string{},
		},
		{
			name: "legacy env var with prefix",
			env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "API_KEY", Prefix: "sk-", Required: true}},
			},
			credEnv:         map[string]string{"API_KEY": "test123"},
			expectedEnv:     []string{"API_KEY=sk-test123"},
			expectedMissing: []string{},
		},
		{
			name: "legacy header - prefix not duplicated when already included",
			headers: []types.MCPHeader{
				{Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "Bearer token123"},
			expectedHeaders: []string{"Authorization=Bearer token123"},
			expectedMissing: []string{},
		},
		{
			name: "legacy env var - prefix not duplicated when already included",
			env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "API_KEY", Prefix: "sk-", Required: true}},
			},
			credEnv:         map[string]string{"API_KEY": "sk-test123"},
			expectedEnv:     []string{"API_KEY=sk-test123"},
			expectedMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{
						Headers: tt.headers,
						Env:     tt.env,
					},
				},
			}
			mcpServer.Name = "test-server"

			config, missing, err := legacyServerToServerConfig(mcpServer, "test-user-id", "test-scope", tt.credEnv, map[string]struct{}{})

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

			// Compare env vars
			if len(config.Env) != len(tt.expectedEnv) {
				t.Errorf("expected %d env vars, got %d: expected %v, got %v", len(tt.expectedEnv), len(config.Env), tt.expectedEnv, config.Env)
			} else {
				for i, expected := range tt.expectedEnv {
					if config.Env[i] != expected {
						t.Errorf("env var %d: expected %s, got %s", i, expected, config.Env[i])
					}
				}
			}

			// Compare missing
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing items, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing item %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}
		})
	}
}

func TestServerToServerConfig_StaticHeaders_EdgeCases(t *testing.T) {
	baseURL := "http://localhost:8080"
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

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), baseURL, "test-user-id", "test-scope", "test-catalog", tt.credEnv, nil)

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
