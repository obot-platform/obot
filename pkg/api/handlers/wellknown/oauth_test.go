package wellknown

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
)

func TestOAuthAuthorizationAppendsMCPIDToOAuthEndpoints(t *testing.T) {
	h := &handler{
		config: handlers.OAuthAuthorizationServerConfig{
			Issuer:                "https://obot.example.com",
			AuthorizationEndpoint: "https://obot.example.com/oauth/authorize",
			TokenEndpoint:         "https://obot.example.com/oauth/token",
			RegistrationEndpoint:  "https://obot.example.com/oauth/register",
			JWKSURI:               "https://obot.example.com/oauth/jwks.json",
		},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server/test-mcp", nil)
	request.SetPathValue("mcp_id", "test-mcp")

	if err := h.oauthAuthorization(api.Context{
		ResponseWriter: recorder,
		Request:        request,
	}); err != nil {
		t.Fatal(err)
	}

	var got handlers.OAuthAuthorizationServerConfig
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.AuthorizationEndpoint != "https://obot.example.com/oauth/authorize/test-mcp" {
		t.Fatalf("authorization_endpoint = %q", got.AuthorizationEndpoint)
	}
	if got.TokenEndpoint != "https://obot.example.com/oauth/token/test-mcp" {
		t.Fatalf("token_endpoint = %q", got.TokenEndpoint)
	}
	if got.RegistrationEndpoint != "https://obot.example.com/oauth/register/test-mcp" {
		t.Fatalf("registration_endpoint = %q", got.RegistrationEndpoint)
	}
	if got.Issuer != "https://obot.example.com/test-mcp" {
		t.Fatalf("issuer = %q", got.Issuer)
	}
	if got.JWKSURI != h.config.JWKSURI {
		t.Fatalf("jwks_uri = %q", got.JWKSURI)
	}
}

func TestVersionedOAuthMetadataPreservesExactResource(t *testing.T) {
	h := &handler{
		baseURL: "https://obot.example.com",
		config: handlers.OAuthAuthorizationServerConfig{
			Issuer:                "https://obot.example.com",
			AuthorizationEndpoint: "https://obot.example.com/oauth/authorize",
			TokenEndpoint:         "https://obot.example.com/oauth/token",
			RegistrationEndpoint:  "https://obot.example.com/oauth/register",
		},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/versioned-mcp-connect/catalog-entry/3", nil)
	request.SetPathValue("entry_id", "catalog-entry")
	request.SetPathValue("version", "3")
	if err := h.oauthProtectedResource(api.Context{ResponseWriter: recorder, Request: request}); err != nil {
		t.Fatal(err)
	}
	var protected map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&protected); err != nil {
		t.Fatal(err)
	}
	const resource = "https://obot.example.com/versioned-mcp-connect/catalog-entry/3"
	if protected["resource"] != resource {
		t.Fatalf("resource = %q, want %q", protected["resource"], resource)
	}
	servers := protected["authorization_servers"].([]any)
	if len(servers) != 1 || servers[0] != resource {
		t.Fatalf("authorization_servers = %#v", servers)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server/versioned-mcp-connect/catalog-entry/3", nil)
	request.SetPathValue("entry_id", "catalog-entry")
	request.SetPathValue("version", "3")
	if err := h.oauthAuthorization(api.Context{ResponseWriter: recorder, Request: request}); err != nil {
		t.Fatal(err)
	}
	var config handlers.OAuthAuthorizationServerConfig
	if err := json.NewDecoder(recorder.Body).Decode(&config); err != nil {
		t.Fatal(err)
	}
	if got, want := config.AuthorizationEndpoint, "https://obot.example.com/oauth/authorize/versioned-mcp-connect/catalog-entry/3"; got != want {
		t.Fatalf("authorization endpoint = %q, want %q", got, want)
	}
	if got, want := config.TokenEndpoint, "https://obot.example.com/oauth/token/versioned-mcp-connect/catalog-entry/3"; got != want {
		t.Fatalf("token endpoint = %q, want %q", got, want)
	}
}
