package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
)

type registrySettings struct{ enabled bool }

func (s *registrySettings) AnonymousRegistryAccessEnabled(context.Context) (bool, error) {
	return s.enabled, nil
}

func TestRegistryAnonymousAccessAndOAuthChallenge(t *testing.T) {
	settings := &registrySettings{}
	limiter, err := ratelimiter.New(ratelimiter.Options{UnauthenticatedRateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		authenticator: authn.NewAuthenticator(authn.Anonymous{}),
		authorizer:    authz.NewAuthorizer(nil, nil, nil, false, nil, nil, settings),
		rateLimiter:   limiter,
		baseURL:       "https://obot.example.com",
	}
	handler := s.Wrap(func(req api.Context) error { return req.Write(map[string]bool{"discovered": true}) })
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, enabled := range []bool{false, true, false} {
			settings.enabled = enabled
			for _, path := range []string{"/v0.1/servers", "/v0.1/servers/example/versions", "/v0.1/servers/example/versions/latest"} {
				response := httptest.NewRecorder()
				handler(response, httptest.NewRequest(method, path, nil))
				if enabled {
					for header, want := range map[string]string{
						"Cache-Control": "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0",
						"Pragma":        "no-cache",
						"Expires":       "0",
					} {
						if got := response.Header().Get(header); got != want {
							t.Fatalf("%s %s: %s = %q, want %q", method, path, header, got, want)
						}
					}
					if response.Code != http.StatusOK || response.Header().Get("WWW-Authenticate") != "" {
						t.Fatalf("%s enabled: %d, %v", method, response.Code, response.Header())
					}
				} else if response.Code != http.StatusUnauthorized || !strings.Contains(response.Header().Get("WWW-Authenticate"), "oauth-protected-resource/v0.1/servers") {
					t.Fatalf("%s disabled: %d, %v", method, response.Code, response.Header())
				}
			}
		}
	}
}
