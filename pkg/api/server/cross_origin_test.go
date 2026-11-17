package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

// countingAuthenticator records whether a request reached authentication.
type countingAuthenticator struct {
	calls int
}

func (c *countingAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	c.calls++
	return authn.Anonymous{}.AuthenticateRequest(req)
}

func TestCrossOriginCookieRequests(t *testing.T) {
	limiter, err := ratelimiter.New(ratelimiter.Options{UnauthenticatedRateLimit: 1000})
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		method   string
		host     string
		headers  map[string]string
		cookies  []string
		rejected bool
	}{
		{
			name:   "same-site tenant posts with the session cookie",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Origin":         "https://tenant-b.example.ai",
				"Content-Type":   "text/plain",
			},
			cookies:  []string{auth.ObotAccessTokenCookie},
			rejected: true,
		},
		{
			name:   "cross-site page posts with the bootstrap cookie",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://evil.example",
			},
			cookies:  []string{auth.ObotBootstrapCookie},
			rejected: true,
		},
		{
			name:   "cross-origin delete without fetch metadata",
			method: http.MethodDelete,
			headers: map[string]string{
				"Origin": "https://tenant-b.example.ai",
			},
			cookies:  []string{auth.ObotAccessTokenCookie},
			rejected: true,
		},
		{
			name:   "same-origin UI request",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "same-origin",
				"Origin":         "https://obot.example.ai",
			},
			cookies: []string{auth.ObotAccessTokenCookie},
		},
		{
			name:    "non-browser client with a cookie",
			method:  http.MethodPost,
			cookies: []string{auth.ObotAccessTokenCookie},
		},
		{
			name:   "reverse proxy rewrites the host",
			method: http.MethodPost,
			host:   "obot.internal:8080",
			headers: map[string]string{
				"Origin": "https://obot.example.ai",
			},
			cookies: []string{auth.ObotAccessTokenCookie},
		},
		{
			name:   "UI served from its own hostname",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Origin":         "https://ui.example.ai",
			},
			cookies: []string{auth.ObotAccessTokenCookie},
		},
		{
			name:   "UI hostname on another port is not trusted",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Origin":         "https://ui.example.ai:8443",
			},
			cookies:  []string{auth.ObotAccessTokenCookie},
			rejected: true,
		},
		{
			name:   "MCP client with a bearer token",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://claude.ai",
				"Authorization":  "Bearer token",
			},
		},
		{
			name:   "bearer token alongside a cookie",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://claude.ai",
				"Authorization":  "Bearer token",
			},
			cookies: []string{auth.ObotBootstrapCookie},
		},
		{
			name:   "API key client",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://example.com",
				"X-API-Key":      "key",
			},
		},
		{
			name:   "anonymous cross-origin OAuth token request",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://claude.ai",
			},
		},
		{
			name:   "cross-site navigation with a cookie",
			method: http.MethodGet,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
			},
			cookies: []string{auth.ObotAccessTokenCookie},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authenticator := &countingAuthenticator{}
			s := &Server{
				mux:           http.NewServeMux(),
				authenticator: authn.NewAuthenticator(authenticator),
				authorizer:    authz.NewAuthorizer(nil, nil, nil, false, nil, nil, &registrySettings{enabled: true}),
				rateLimiter:   limiter,
				crossOrigin:   newCrossOriginProtection("https://obot.example.ai", "https://ui.example.ai/"),
			}
			s.HandleFunc("/mcp-connect/example", func(api.Context) error { return nil })

			req := httptest.NewRequest(tc.method, "https://obot.example.ai/mcp-connect/example", nil)
			if tc.host != "" {
				req.Host = tc.host
			}
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}
			for _, name := range tc.cookies {
				req.AddCookie(&http.Cookie{Name: name, Value: "value"})
			}
			response := httptest.NewRecorder()
			s.mux.ServeHTTP(response, req)

			if tc.rejected {
				require.Equal(t, http.StatusForbidden, response.Code)
				require.Contains(t, response.Body.String(), "cross-origin request rejected")
				require.Zero(t, authenticator.calls, "a rejected request must not be authenticated")
			} else {
				require.NotContains(t, response.Body.String(), "cross-origin request rejected")
				require.Equal(t, 1, authenticator.calls)
			}
		})
	}
}
