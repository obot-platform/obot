package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/proxy"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type stubAuthenticator struct{}

type noAuditLogger struct{}

func (noAuditLogger) LogEntry(audit.LogEntry) error { return nil }
func (noAuditLogger) Close() error                  { return nil }

func (stubAuthenticator) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "7",
			UID:    "7",
			Groups: []string{types.GroupAdmin, types.GroupMCP, types.GroupAuthenticated},
			Extra: map[string][]string{
				principal.UserStatusExtra: {string(types.UserStatusActive)},
				"set-cookies":             {"platform-session=must-not-leak"},
			},
		},
	}, true, nil
}

func TestAPIProxyBusinessResponseIsNotPolluted(t *testing.T) {
	server := &Server{
		authenticator: authn.NewAuthenticator(stubAuthenticator{}),
		authorizer:    &authz.Authorizer{},
		auditLogger:   noAuditLogger{},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api-proxy/vmcp1abc/component/items/a%2Fb?z=2&a=1&a=1", nil)
	request.Pattern = "/"
	wrapped := server.Wrap(func(req api.Context) error {
		require.Equal(t, "/api-proxy/vmcp1abc/component/items/a%2Fb", req.URL.EscapedPath())
		req.ResponseWriter.Header().Add("X-Business", "one")
		req.ResponseWriter.Header().Add("X-Business", "two")
		req.ResponseWriter.WriteHeader(http.StatusTeapot)
		_, err := req.ResponseWriter.Write([]byte("binary\x00body"))
		return err
	})

	wrapped(recorder, request)

	require.Equal(t, http.StatusTeapot, recorder.Code)
	require.Equal(t, []string{"one", "two"}, recorder.Header().Values("X-Business"))
	require.Empty(t, recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("X-Content-Type-Options"))
	require.Empty(t, recorder.Header().Get("Cache-Control"))
	require.Empty(t, recorder.Header().Values("Set-Cookie"))
	require.Equal(t, "binary\x00body", recorder.Body.String())
}

func TestAPIProxyAuthenticationFailureDoesNotRedirectOrSetCookie(t *testing.T) {
	server := &Server{
		authenticator: authn.NewAuthenticator(failingAuthenticator{err: proxy.ErrInvalidSession}),
	}
	server.HandleAPIProxy(func(api.Context) error {
		return errors.New("must not run")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api-proxy/vmcp1abc/component/items/a%2Fb", nil)
	server.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Empty(t, recorder.Header().Get("Location"))
	require.Empty(t, recorder.Header().Values("Set-Cookie"))
	require.Equal(t, "invalid_session", recorder.Header().Get("X-Obot-Error-Code"))
	require.NotEmpty(t, recorder.Header().Get("X-Obot-Request-ID"))
	var body struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestID"`
		Retryable bool   `json:"retryable"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "invalid_session", body.Code)
	require.Equal(t, "invalid platform session", body.Message)
	require.Equal(t, recorder.Header().Get("X-Obot-Request-ID"), body.RequestID)
	require.False(t, body.Retryable)
}
