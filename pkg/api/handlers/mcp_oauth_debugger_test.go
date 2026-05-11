package handlers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"golang.org/x/oauth2"
)

func TestOAuthDebuggerMetadata(t *testing.T) {
	authServer := nmcp.AuthorizationServerMetadata{
		Issuer:                            "https://auth.example.com",
		AuthorizationEndpoint:             "https://auth.example.com/authorize",
		TokenEndpoint:                     "https://auth.example.com/token",
		RegistrationEndpoint:              "https://auth.example.com/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
	}
	authServerJSON := mustJSON(t, authServer)

	registration := nmcp.ClientRegistrationMetadata{Scope: "read write"}
	registrationJSON := mustJSON(t, registration)

	m := &MCPHandler{serverURL: "https://obot.example.com"}
	metadata, parsedAuthServer, parsedRegistration, err := m.oauthDebuggerMetadata(v1.MCPServer{
		Status: v1.MCPServerStatus{
			OAuthMetadata: &v1.OAuthMetadata{
				AuthorizationServerURL:      authServer.Issuer,
				AuthorizationServerMetadata: authServerJSON,
				ClientRegistration:          registrationJSON,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metadata.AuthorizationServerURL != authServer.Issuer {
		t.Fatalf("expected metadata to be returned")
	}
	if !reflect.DeepEqual(parsedAuthServer, authServer) {
		t.Fatalf("parsed auth server mismatch:\nexpected: %#v\nactual:   %#v", authServer, parsedAuthServer)
	}

	expectedRegistration := nmcp.ClientRegistrationMetadata{
		RedirectURIs:            []string{"https://obot.example.com/oauth/mcp/callback"},
		TokenEndpointAuthMethod: "client_secret_post",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		ClientName:              "Obot MCP OAuth Debugger",
		Scope:                   "read write",
	}
	if !reflect.DeepEqual(parsedRegistration, expectedRegistration) {
		t.Fatalf("parsed registration mismatch:\nexpected: %#v\nactual:   %#v", expectedRegistration, parsedRegistration)
	}
}

func TestOAuthDebuggerMetadataErrors(t *testing.T) {
	tests := []struct {
		name             string
		oauthMetadata    *v1.OAuthMetadata
		expectedContains string
	}{
		{
			name: "invalid auth server metadata",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: json.RawMessage(`{`),
			},
			expectedContains: "failed to parse OAuth authorization server metadata",
		},
		{
			name: "missing authorization endpoint",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: mustJSON(t, nmcp.AuthorizationServerMetadata{
					TokenEndpoint: "https://auth.example.com/token",
				}),
			},
			expectedContains: "authorization_endpoint",
		},
		{
			name: "missing token endpoint",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: mustJSON(t, nmcp.AuthorizationServerMetadata{
					AuthorizationEndpoint: "https://auth.example.com/authorize",
				}),
			},
			expectedContains: "token_endpoint",
		},
		{
			name: "invalid client registration metadata",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: mustJSON(t, nmcp.AuthorizationServerMetadata{
					AuthorizationEndpoint: "https://auth.example.com/authorize",
					TokenEndpoint:         "https://auth.example.com/token",
				}),
				ClientRegistration: json.RawMessage(`{`),
			},
			expectedContains: "failed to parse OAuth client registration metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := (&MCPHandler{}).oauthDebuggerMetadata(v1.MCPServer{
				Status: v1.MCPServerStatus{OAuthMetadata: tt.oauthMetadata},
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.expectedContains) {
				t.Fatalf("expected error to contain %q, got %q", tt.expectedContains, err.Error())
			}
		})
	}
}

func TestOAuthDebuggerAuthStyle(t *testing.T) {
	tests := []struct {
		method   string
		expected oauth2.AuthStyle
	}{
		{method: "client_secret_basic", expected: oauth2.AuthStyleInHeader},
		{method: "client_secret_post", expected: oauth2.AuthStyleInParams},
		{method: "", expected: oauth2.AuthStyleAutoDetect},
		{method: "private_key_jwt", expected: oauth2.AuthStyleAutoDetect},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if actual := oauthDebuggerAuthStyle(tt.method); actual != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
