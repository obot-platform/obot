package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/runtime"
)

type oauthDebuggerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oauthDebuggerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOAuthDebuggerMetadata(t *testing.T) {
	authServer := mcp.AuthorizationServerMetadata{
		Issuer:                            "https://auth.example.com",
		AuthorizationEndpoint:             "https://auth.example.com/authorize",
		TokenEndpoint:                     "https://auth.example.com/token",
		RegistrationEndpoint:              "https://auth.example.com/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
	}
	authServerJSON := mustJSON(t, authServer)

	registration := mcp.ClientRegistrationMetadata{Scope: "read write"}
	registrationJSON := mustJSON(t, registration)

	m := &MCPHandler{serverURL: "https://obot.example.com"}
	const resourceURL = "https://resource.example.com/mcp"
	parsedAuthServer, parsedRegistration, parsedResourceURL, err := m.oauthDebuggerMetadata(v1.MCPServer{
		Status: v1.MCPServerStatus{
			OAuthMetadata: &v1.OAuthMetadata{
				AuthorizationServerURL:      authServer.Issuer,
				ProtectedResourceMetadata:   runtime.RawExtension{Raw: json.RawMessage(`{"resource":["https://resource.example.com/mcp"]}`)},
				AuthorizationServerMetadata: runtime.RawExtension{Raw: authServerJSON},
				ClientRegistration:          runtime.RawExtension{Raw: registrationJSON},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(parsedAuthServer, authServer) {
		t.Fatalf("parsed auth server mismatch:\nexpected: %#v\nactual:   %#v", authServer, parsedAuthServer)
	}

	expectedRegistration := mcp.ClientRegistrationMetadata{
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
	if parsedResourceURL != resourceURL {
		t.Fatalf("parsed resource URL = %q, want %q", parsedResourceURL, resourceURL)
	}
}

func TestOAuthDebuggerMetadataErrors(t *testing.T) {
	tests := []struct {
		name             string
		oauthMetadata    *v1.OAuthMetadata
		expectedContains string
	}{
		{
			name: "invalid protected resource metadata",
			oauthMetadata: &v1.OAuthMetadata{
				ProtectedResourceMetadata: runtime.RawExtension{Raw: json.RawMessage(`{`)},
			},
			expectedContains: "failed to parse OAuth protected resource metadata",
		},
		{
			name: "invalid auth server metadata",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: runtime.RawExtension{Raw: json.RawMessage(`{`)},
			},
			expectedContains: "failed to parse OAuth authorization server metadata",
		},
		{
			name: "missing authorization endpoint",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: runtime.RawExtension{Raw: mustJSON(t, mcp.AuthorizationServerMetadata{
					TokenEndpoint: "https://auth.example.com/token",
				})},
			},
			expectedContains: "authorization_endpoint",
		},
		{
			name: "missing token endpoint",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: runtime.RawExtension{Raw: mustJSON(t, mcp.AuthorizationServerMetadata{
					AuthorizationEndpoint: "https://auth.example.com/authorize",
				})},
			},
			expectedContains: "token_endpoint",
		},
		{
			name: "invalid client registration metadata",
			oauthMetadata: &v1.OAuthMetadata{
				AuthorizationServerMetadata: runtime.RawExtension{Raw: mustJSON(t, mcp.AuthorizationServerMetadata{
					AuthorizationEndpoint: "https://auth.example.com/authorize",
					TokenEndpoint:         "https://auth.example.com/token",
				})},
				ClientRegistration: runtime.RawExtension{Raw: json.RawMessage(`{`)},
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
		name            string
		method          string
		hasClientSecret bool
		staticClient    bool
		expected        oauth2.AuthStyle
	}{
		{name: "static confidential client", method: "client_secret_basic", hasClientSecret: true, staticClient: true, expected: oauth2.AuthStyleAutoDetect},
		{name: "static public client", method: "client_secret_basic", staticClient: true, expected: oauth2.AuthStyleAutoDetect},
		{name: "dynamic public client", method: "client_secret_basic", expected: oauth2.AuthStyleInParams},
		{name: "dynamic confidential client basic", method: "client_secret_basic", hasClientSecret: true, expected: oauth2.AuthStyleInHeader},
		{name: "dynamic confidential client post", method: "client_secret_post", hasClientSecret: true, expected: oauth2.AuthStyleInParams},
		{name: "dynamic confidential client unspecified", hasClientSecret: true, expected: oauth2.AuthStyleAutoDetect},
		{name: "dynamic confidential client private key", method: "private_key_jwt", hasClientSecret: true, expected: oauth2.AuthStyleAutoDetect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := oauthDebuggerAuthStyle(tt.method, tt.hasClientSecret, tt.staticClient); actual != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}

func TestOAuthDebuggerStaticClient(t *testing.T) {
	authServer := mcp.AuthorizationServerMetadata{
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/token",
	}

	client := oauthDebuggerStaticClient("client-id", "client-secret", authServer)

	if !client.Static {
		t.Fatal("expected static client")
	}
	if client.ClientID != "client-id" || client.ClientSecret != "client-secret" {
		t.Fatalf("expected static credentials to be set, got %q/%q", client.ClientID, client.ClientSecret)
	}
	if client.AuthorizeURL != authServer.AuthorizationEndpoint || client.TokenURL != authServer.TokenEndpoint {
		t.Fatalf("expected auth server URLs to be set")
	}
}

func TestOAuthDebuggerUsesCIMD(t *testing.T) {
	tests := []struct {
		name         string
		serverURL    string
		oauthMeta    *v1.OAuthMetadata
		clientID     string
		clientSecret string
		forceDynamic bool
		expected     bool
	}{
		{
			name:      "supported without static credentials",
			serverURL: "https://obot.example.com",
			oauthMeta: &v1.OAuthMetadata{
				ClientIDMetadataDocumentSupported: true,
			},
			expected: true,
		},
		{
			name:         "dynamic client registration forced",
			serverURL:    "https://obot.example.com",
			forceDynamic: true,
			oauthMeta: &v1.OAuthMetadata{
				ClientIDMetadataDocumentSupported: true,
			},
		},
		{
			name:      "static credentials win",
			serverURL: "https://obot.example.com",
			oauthMeta: &v1.OAuthMetadata{
				ClientIDMetadataDocumentSupported: true,
			},
			clientID:     "client-id",
			clientSecret: "client-secret",
		},
		{
			name:      "public static client wins",
			serverURL: "https://obot.example.com",
			oauthMeta: &v1.OAuthMetadata{
				ClientIDMetadataDocumentSupported: true,
			},
			clientID: "public-client-id",
		},
		{
			name:      "unsupported by auth server",
			serverURL: "https://obot.example.com",
			oauthMeta: &v1.OAuthMetadata{
				ClientIDMetadataDocumentSupported: false,
			},
		},
		{
			name:      "obot client id must be https",
			serverURL: "http://obot.example.com",
			oauthMeta: &v1.OAuthMetadata{
				ClientIDMetadataDocumentSupported: true,
			},
		},
		{
			name:      "missing metadata",
			serverURL: "https://obot.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := v1.MCPServer{
				Status: v1.MCPServerStatus{OAuthMetadata: tt.oauthMeta},
			}
			got := (&MCPHandler{serverURL: tt.serverURL, forceDynamicClient: tt.forceDynamic}).useOAuthDebuggerCIMD(server, tt.clientID, tt.clientSecret)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestOAuthDebuggerCIMDClient(t *testing.T) {
	authServer := mcp.AuthorizationServerMetadata{
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/token",
	}
	registration := mcp.ClientRegistrationMetadata{
		RedirectURIs:  []string{"https://obot.example.com/oauth/mcp/callback"},
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"},
		ClientName:    "Obot MCP OAuth Debugger",
		Scope:         "read write",
	}

	client := (&MCPHandler{serverURL: "https://obot.example.com"}).oauthDebuggerCIMDClient(authServer, registration)

	if client.ClientID != system.OAuthClientIDMetadataURL("https://obot.example.com") {
		t.Fatalf("expected CIMD client ID, got %q", client.ClientID)
	}
	if client.ClientSecret != "" {
		t.Fatalf("expected no client secret, got %q", client.ClientSecret)
	}
	if client.TokenEndpointAuthMethod != "none" {
		t.Fatalf("expected token_endpoint_auth_method none, got %q", client.TokenEndpointAuthMethod)
	}
	if client.AuthorizeURL != authServer.AuthorizationEndpoint || client.TokenURL != authServer.TokenEndpoint {
		t.Fatalf("expected auth server URLs to be set")
	}
	if !reflect.DeepEqual(client.RedirectURIs, registration.RedirectURIs) {
		t.Fatalf("expected redirect URIs %#v, got %#v", registration.RedirectURIs, client.RedirectURIs)
	}
}

func TestRegisterOAuthDebuggerClientUsesProvidedHTTPClient(t *testing.T) {
	registration := mcp.ClientRegistrationMetadata{
		ClientName:   "Obot MCP OAuth Debugger",
		RedirectURIs: []string{"https://obot.example.com/oauth/mcp/callback"},
	}
	expected := types.OAuthClient{
		ClientID:     "registered-client",
		ClientSecret: "registered-secret",
	}
	registrationResponse := expected
	registrationResponse.Static = true
	called := false
	httpClient := &http.Client{Transport: oauthDebuggerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		called = true
		if request.Method != http.MethodPost || request.URL.String() != "https://auth.internal.test/register" {
			t.Errorf("registration request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
			t.Errorf("registration request headers = %#v", request.Header)
		}
		var actual mcp.ClientRegistrationMetadata
		if err := json.NewDecoder(request.Body).Decode(&actual); err != nil {
			t.Errorf("decode registration request: %v", err)
		} else if !reflect.DeepEqual(actual, registration) {
			t.Errorf("registration request = %#v, want %#v", actual, registration)
		}

		body := strings.NewReader(string(mustJSON(t, registrationResponse)))
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(body),
			Request:    request,
		}, nil
	})}

	actual, err := registerOAuthDebuggerClient(t.Context(), httpClient, "https://auth.internal.test/register", registration)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("provided HTTP client was not used")
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("registered client = %#v, want %#v", actual, expected)
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
