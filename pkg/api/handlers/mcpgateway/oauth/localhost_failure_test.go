package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestLocalhostCallbackFailuresPreserveRequest(t *testing.T) {
	for _, tt := range []struct {
		name            string
		status          int
		body            string
		failPersistence bool
	}{
		{
			name:   "expired code",
			status: http.StatusBadRequest,
			body:   `{"error":"invalid_grant"}`,
		},
		{
			name:   "provider outage",
			status: http.StatusServiceUnavailable,
			body:   `{"error":"temporarily_unavailable"}`,
		},
		{
			name:            "token persistence failure",
			status:          http.StatusOK,
			body:            `{"access_token":"upstream-token","token_type":"Bearer"}`,
			failPersistence: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
			require.NoError(t, err)
			db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate())
			gw := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
			t.Cleanup(func() { require.NoError(t, gw.Close()) })
			state := newStateManager(gw)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer provider.Close()
			conf := &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: provider.URL, AuthStyle: oauth2.AuthStyleInParams}}
			if tt.failPersistence {
				require.NoError(t, services.DB.DB.Migrator().DropTable(&gatewaytypes.MCPOAuthToken{}))
			}
			require.NoError(t, state.store(t.Context(), "1", "connection", provider.URL, "auth-request", "state", "verifier", "", conf))
			requestID, mcpID, err := state.createToken(t.Context(), "state", "code", "", "")
			require.Error(t, err)
			require.Equal(t, "auth-request", requestID)
			require.Equal(t, "connection", mcpID)
			// Failed attempts consume pending state; replay cannot recover a redirect target.
			requestID, mcpID, err = state.createToken(t.Context(), "state", "code", "", "")
			require.Error(t, err)
			require.Empty(t, requestID)
			require.Empty(t, mcpID)

			fixture := newVMCPOAuthFixture(t)
			req := fixture.request(t.Context())
			req.User = &user.DefaultInfo{UID: "1", Name: "user", Groups: []string{types.GroupAuthenticated}}
			authRequest := &v1.OAuthAuthRequest{
				Name:      "auth-request",
				Namespace: system.DefaultNamespace,
				Spec: v1.OAuthAuthRequestSpec{
					UserID:      1,
					RedirectURI: "http://localhost:1234/oauth/obot/callback",
					State:       "client-state",
				},
			}
			require.NoError(t, fixture.storage.Create(t.Context(), authRequest))
			require.NoError(t, state.store(t.Context(), "1", "connection", provider.URL, authRequest.Name, "callback-state", "verifier", "", conf))
			req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?state=callback-state&code=code", nil)
			response := httptest.NewRecorder()
			req.ResponseWriter = response
			h := handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: state}}
			require.NoError(t, h.oauthCallback(req))
			require.Equal(t, http.StatusFound, response.Code)
			location, err := url.Parse(response.Header().Get("Location"))
			require.NoError(t, err)
			require.Equal(t, "localhost:1234", location.Host)
			require.Equal(t, "/oauth/obot/callback", location.Path)
			require.Equal(t, "client-state", location.Query().Get("state"))
			require.Equal(t, "access_denied", location.Query().Get("error"))
			require.NotContains(t, location.String(), "upstream-token")
		})
	}
}
