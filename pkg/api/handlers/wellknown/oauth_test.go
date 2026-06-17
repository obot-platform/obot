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
	if got.Issuer != h.config.Issuer {
		t.Fatalf("issuer = %q", got.Issuer)
	}
	if got.JWKSURI != h.config.JWKSURI {
		t.Fatalf("jwks_uri = %q", got.JWKSURI)
	}
}
