package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/stretchr/testify/require"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// TestWrapRefusesInvalidDelegations checks that a token with which an agent acts on behalf of a user that no longer
// does is answered with a 401 that names agent_delegation_invalid, wherever the chain refused it, in the LLM API's
// format on the LLM proxy, and without a challenge or a cookie on an MCP connection.
func TestWrapRefusesInvalidDelegations(t *testing.T) {
	invalid := &gclient.AgentDelegationInvalidError{Reason: "the delegation was revoked"}
	chainErr := utilerrors.NewAggregate([]error{utilerrors.NewAggregate([]error{invalid, persistent.ErrDelegatedToken}), errors.New("invalid agent token")})

	for _, path := range []string{"/mcp-connect/vmcp1test", "/api/skills"} {
		t.Run(path, func(t *testing.T) {
			s := &Server{authenticator: authn.NewAuthenticator(failingAuthenticator{err: chainErr})}
			rec := httptest.NewRecorder()
			s.Wrap(func(api.Context) error {
				t.Fatal("the handler ran for a refused delegation")
				return nil
			})(rec, httptest.NewRequest(http.MethodPost, path, nil))

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			var body api.ErrorBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, gclient.AgentDelegationInvalidCode, body.Error)
			require.Empty(t, rec.Header().Values("WWW-Authenticate"))
			require.Empty(t, rec.Result().Cookies())
			require.NotContains(t, rec.Body.String(), invalid.Reason, "the reason is for logs only")
		})
	}

	t.Run("LLM proxy", func(t *testing.T) {
		s := &Server{authenticator: authn.NewAuthenticator(failingAuthenticator{err: chainErr})}
		rec := httptest.NewRecorder()
		s.Wrap(func(api.Context) error {
			t.Fatal("the handler ran for a refused delegation")
			return nil
		})(rec, httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/chat/completions", nil))

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Contains(t, rec.Body.String(), gclient.AgentDelegationInvalidCode)
		require.True(t, json.Valid(rec.Body.Bytes()), "the LLM API's error format: %s", rec.Body.String())
	})
}
