package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestWrapAnonymousSystemMCPServerChallengeOnlyForExternallyAccessibleServers(t *testing.T) {
	limiter, err := ratelimiter.New(ratelimiter.Options{
		UnauthenticatedRateLimit: 100,
		AuthenticatedRateLimit:   100,
	})
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}

	s := &Server{
		authenticator: authn.NewAuthenticator(authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
			return &authenticator.Response{
				User: &user.DefaultInfo{
					Name:   "anonymous",
					UID:    "anonymous",
					Groups: []string{authz.UnauthenticatedGroup},
				},
			}, true, nil
		})),
		authorizer:    authz.NewAuthorizer(nil, nil, false, nil, false),
		rateLimiter:   limiter,
		baseURL:       "https://obot.example.com/api",
		mcpOAuthScope: `, scope="profile"`,
	}

	tests := []struct {
		name          string
		mcpID         string
		wantStatus    int
		wantChallenge bool
	}{
		{
			name:          "obot system MCP server gets OAuth challenge",
			mcpID:         system.ObotMCPServerName,
			wantStatus:    http.StatusUnauthorized,
			wantChallenge: true,
		},
		{
			name:       "other system MCP server is hidden",
			mcpID:      system.SystemMCPServerPrefix + "other",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "webhook system MCP server is hidden",
			mcpID:      system.SystemMCPServerPrefix + system.MCPWebhookValidationPrefix + "test",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp-connect/"+tt.mcpID, nil)
			resp := httptest.NewRecorder()
			mux := http.NewServeMux()
			mux.Handle("/mcp-connect/{mcp_id}", s.Wrap(func(api.Context) error {
				t.Fatal("handler should not be called for anonymous MCP connect requests")
				return nil
			}))

			mux.ServeHTTP(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.Code, tt.wantStatus)
			}

			challenge := resp.Header().Get("WWW-Authenticate")
			if tt.wantChallenge {
				if !strings.Contains(challenge, `resource_metadata="https://obot.example.com/.well-known/oauth-protected-resource/mcp-connect/`+tt.mcpID+`"`) {
					t.Fatalf("WWW-Authenticate = %q, want resource metadata for %s", challenge, tt.mcpID)
				}
				if !strings.Contains(challenge, `scope="profile"`) {
					t.Fatalf("WWW-Authenticate = %q, want profile scope", challenge)
				}
			} else if challenge != "" {
				t.Fatalf("WWW-Authenticate = %q, want empty", challenge)
			}
		})
	}
}
