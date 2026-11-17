package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

type stubAgentAuthenticator struct {
	resp *authenticator.Response
}

func (s stubAgentAuthenticator) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	return s.resp, true, nil
}

// TestWrapForbidsAgentsOutsideTheirRoutes checks that an authenticated agent is answered 403, and never reaches the
// handler, on administration, user, device and API key management routes, while it reaches the routes its key
// grants. The server has no gateway client, so recording daily activity for the agent would fail the test: agents
// are not recorded as active users.
func TestWrapForbidsAgentsOutsideTheirRoutes(t *testing.T) {
	limiter, err := ratelimiter.New(ratelimiter.Options{
		UnauthenticatedRateLimit: 1000,
		AuthenticatedRateLimit:   1000,
	})
	require.NoError(t, err)
	auditLogger, err := audit.New(t.Context(), audit.Options{AuditLogsMode: audit.ModeOff})
	require.NoError(t, err)

	scopes := gatewaytypes.APIKeyScopes{
		CanAccessSkills:   true,
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"*"},
	}
	agent := gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7, DisplayName: "nightly build"}, scopes.Groups(nil), map[string][]string{
		"authorized_mcp_ids": scopes.MCPServerIDs,
	})
	s := &Server{
		authenticator: authn.NewAuthenticator(authn.NewAdmissionCheck(stubAgentAuthenticator{
			resp: &authenticator.Response{
				User: agent,
			},
		}, nil)),
		authorizer:  authz.NewAuthorizer(nil, nil, nil, false, nil, nil, nil),
		rateLimiter: limiter,
		auditLogger: auditLogger,
	}

	for _, tt := range []struct {
		method     string
		path       string
		wantStatus int
	}{
		{
			method:     http.MethodPost,
			path:       "/api/llm-proxy/v1/messages",
			wantStatus: http.StatusOK,
		},
		{
			method:     http.MethodGet,
			path:       "/api/skills",
			wantStatus: http.StatusOK,
		},
		{
			method:     http.MethodGet,
			path:       "/api/me",
			wantStatus: http.StatusForbidden,
		},
		{
			method:     http.MethodPost,
			path:       "/api/api-keys",
			wantStatus: http.StatusForbidden,
		},
		{
			method:     http.MethodGet,
			path:       "/api/admin-api-keys",
			wantStatus: http.StatusForbidden,
		},
		{
			method:     http.MethodGet,
			path:       "/api/users/7",
			wantStatus: http.StatusForbidden,
		},
		{
			method:     http.MethodPost,
			path:       "/api/devices/scans",
			wantStatus: http.StatusForbidden,
		},
		{
			method:     http.MethodGet,
			path:       "/api/model-providers",
			wantStatus: http.StatusForbidden,
		},
		{
			method:     http.MethodPost,
			path:       "/api/token-request/verify",
			wantStatus: http.StatusForbidden,
		},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var reached bool
			handler := s.Wrap(func(req api.Context) error {
				reached = true
				return req.Write(map[string]bool{"ok": true})
			})
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(tt.method, tt.path, nil))
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			require.Equal(t, tt.wantStatus == http.StatusOK, reached, "handler reached")
		})
	}
}

// TestWrapWritesStructuredErrors checks that an error with reasons is answered as a JSON body with its status, while
// a plain error stays plain text.
func TestWrapWritesStructuredErrors(t *testing.T) {
	limiter, err := ratelimiter.New(ratelimiter.Options{UnauthenticatedRateLimit: 1000, AuthenticatedRateLimit: 1000})
	require.NoError(t, err)
	auditLogger, err := audit.New(t.Context(), audit.Options{AuditLogsMode: audit.ModeOff})
	require.NoError(t, err)

	scopes := gatewaytypes.APIKeyScopes{CanAccessSkills: true, MCPServerIDs: []string{"*"}}
	agent := gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7, DisplayName: "nightly build"}, scopes.Groups(nil), map[string][]string{
		"authorized_mcp_ids": scopes.MCPServerIDs,
	})
	s := &Server{
		authenticator: authn.NewAuthenticator(authn.NewAdmissionCheck(stubAgentAuthenticator{resp: &authenticator.Response{User: agent}}, nil)),
		authorizer:    authz.NewAuthorizer(nil, nil, nil, false, nil, nil, nil),
		rateLimiter:   limiter,
		auditLogger:   auditLogger,
	}

	reasons := []types.AgentReadinessReason{{ComponentID: "github", Code: types.AgentReadinessConfigurationRequired, Field: "__url"}}
	for _, tt := range []struct {
		name            string
		err             error
		wantContentType string
		wantBody        string
	}{
		{
			name:            "with reasons",
			err:             api.NewErrHTTPWithReasons(http.StatusBadRequest, "needs __url", reasons),
			wantContentType: "application/json",
			wantBody:        `{"error":"needs __url","reasons":[{"componentID":"github","code":"configuration_required","field":"__url"}]}`,
		},
		{
			name:            "plain",
			err:             types.NewErrBadRequest("needs __url"),
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "needs __url\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// An agent reaches its skills route, which is enough to get a handler to return the error.
			handler := s.Wrap(func(api.Context) error { return tt.err })
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, tt.wantContentType, rec.Header().Get("Content-Type"))
			if tt.wantContentType == "application/json" {
				require.JSONEq(t, tt.wantBody, rec.Body.String())
			} else {
				require.Equal(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}
