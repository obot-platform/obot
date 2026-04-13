package wellknown

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
)

func TestOAuthAuthorizationUsesInternalBaseURLForInternalRequests(t *testing.T) {
	h := &handler{
		baseURL:         "http://localhost:8080",
		internalBaseURL: "http://obot.default.svc.cluster.local",
		internalHost:    "obot.default.svc.cluster.local",
		config: handlers.OAuthAuthorizationServerConfig{
			Issuer:                "http://localhost:8080",
			AuthorizationEndpoint: "http://localhost:8080/oauth/authorize",
			TokenEndpoint:         "http://localhost:8080/oauth/token",
			RegistrationEndpoint:  "http://localhost:8080/oauth/register",
			JWKSURI:               "http://localhost:8080/oauth/jwks.json",
			UserInfoEndpoint:      "http://localhost:8080/oauth/userinfo",
		},
	}

	req := httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil)
	req.Host = "obot.default.svc.cluster.local"
	recorder := httptest.NewRecorder()

	if err := h.oauthAuthorization(api.Context{Request: req, ResponseWriter: recorder}); err != nil {
		t.Fatal(err)
	}

	var config handlers.OAuthAuthorizationServerConfig
	if err := json.Unmarshal(recorder.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}

	if config.Issuer != h.internalBaseURL {
		t.Fatalf("expected issuer %q, got %q", h.internalBaseURL, config.Issuer)
	}
	if config.AuthorizationEndpoint != h.internalBaseURL+"/oauth/authorize" {
		t.Fatalf("expected internal authorization endpoint, got %q", config.AuthorizationEndpoint)
	}
	if config.TokenEndpoint != h.internalBaseURL+"/oauth/token" {
		t.Fatalf("expected internal token endpoint, got %q", config.TokenEndpoint)
	}
	if config.RegistrationEndpoint != h.internalBaseURL+"/oauth/register" {
		t.Fatalf("expected internal registration endpoint, got %q", config.RegistrationEndpoint)
	}
	if config.JWKSURI != h.internalBaseURL+"/oauth/jwks.json" {
		t.Fatalf("expected internal JWKS URI, got %q", config.JWKSURI)
	}
	if config.UserInfoEndpoint != h.internalBaseURL+"/oauth/userinfo" {
		t.Fatalf("expected internal userinfo endpoint, got %q", config.UserInfoEndpoint)
	}
}
