package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestLocalLoginURL(t *testing.T) {
	f := &MCPOAuthHandlerFactory{baseURL: "https://obot.example"}
	upstream := "https://provider.example/authorize?state=attempt"
	local := mcp.ServerConfig{LocalhostCallbackEnabled: true}
	got, err := f.localLoginURL(upstream, local, "")
	require.NoError(t, err)
	require.Equal(t, "https://obot.example/oauth/mcp/login/attempt", got)
	got, err = f.localLoginURL(upstream, local, "native-client-request")
	require.NoError(t, err)
	require.Equal(t, upstream, got)
	got, err = f.localLoginURL(upstream, mcp.ServerConfig{}, "")
	require.NoError(t, err)
	require.Equal(t, upstream, got)
	got, err = f.localLoginURL("", local, "")
	require.NoError(t, err)
	require.Empty(t, got)
	_, err = f.localLoginURL("https://provider.example/authorize", local, "")
	require.Error(t, err)
}

func TestLocalLoginPreservesUIAttempt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		userID  string
		denied  bool
		expired bool
		hosted  bool
	}{
		{
			name:   "temporary tool preview",
			userID: "system",
		},
		{
			name:   "inspector",
			userID: "1",
		},
		{
			name:   "denied",
			userID: "system",
			denied: true,
		},
		{
			name:    "expired",
			userID:  "system",
			expired: true,
		},
		{
			name:   "hosted OAuth on localhost",
			userID: "1",
			hosted: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
			require.NoError(t, err)
			db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate())
			gw := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
			t.Cleanup(func() { require.NoError(t, gw.Close()) })
			sm := newStateManager(gw)
			exchanges := 0
			redirectURL := "http://localhost:54321/custom/callback"
			if tc.hosted {
				redirectURL = "http://localhost:8080/oauth/mcp/callback"
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				exchanges++
				require.NoError(t, r.ParseForm())
				require.Equal(t, "provider-code", r.Form.Get("code"))
				require.Equal(t, "private-verifier", r.Form.Get("code_verifier"))
				require.Equal(t, redirectURL, r.Form.Get("redirect_uri"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"upstream-token","token_type":"Bearer"}`))
			}))
			defer provider.Close()
			conf := &oauth2.Config{
				ClientID:     "provider-client",
				ClientSecret: "private-secret",
				RedirectURL:  redirectURL,
				Scopes:       []string{"read"},
				Endpoint: oauth2.Endpoint{
					AuthURL:   provider.URL + "/authorize",
					TokenURL:  provider.URL + "/token",
					AuthStyle: oauth2.AuthStyleInParams,
				},
			}
			oauthHandler := &mcpOAuthHandler{
				stateMgr:     sm,
				userID:       tc.userID,
				mcpID:        "temporary-preview-id",
				mcpURL:       "https://provider.example/mcp",
				uiLocalLogin: !tc.hosted,
			}
			state, _, err := oauthHandler.NewState(t.Context(), conf, "https://provider.example/mcp", "private-verifier")
			require.NoError(t, err)
			if tc.expired {
				require.NoError(t, services.DB.DB.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("mcp_id = ?", "temporary-preview-id").Update("created_at", time.Now().Add(-11*time.Minute)).Error)
			}
			h := handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: sm}}
			fixture := newVMCPOAuthFixture(t)
			req := fixture.request(t.Context())
			req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/login/"+state, nil)
			req.SetPathValue("state", state)
			response := httptest.NewRecorder()
			req.ResponseWriter = response
			err = h.localLogin(req)
			if tc.expired || tc.hosted {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
				var metadata types.MCPLocalLogin
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &metadata))
				require.Equal(t, conf.RedirectURL, metadata.RedirectURL)
				auth, err := url.Parse(metadata.AuthorizationURL)
				require.NoError(t, err)
				require.Equal(t, state, auth.Query().Get("state"))
				require.Equal(t, "S256", auth.Query().Get("code_challenge_method"))
				require.NotEmpty(t, auth.Query().Get("code_challenge"))
				require.NotContains(t, response.Body.String(), "private-secret")
				require.NotContains(t, response.Body.String(), "private-verifier")
				require.NotContains(t, response.Body.String(), "temporary-preview-id")
			}
			callbackQuery := "?state=" + state + "&code=provider-code"
			if tc.denied {
				callbackQuery = "?state=" + state + "&error=access_denied"
			}
			req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback"+callbackQuery, nil)
			response = httptest.NewRecorder()
			req.ResponseWriter = response
			require.NoError(t, h.oauthCallback(req))
			require.Equal(t, http.StatusFound, response.Code)
			completed, err := url.Parse(response.Header().Get("Location"))
			require.NoError(t, err)
			if tc.hosted {
				require.Equal(t, "/auth/oauth/complete", response.Header().Get("Location"))
				require.Equal(t, 1, exchanges)
				return
			}
			require.Equal(t, "localhost:54321", completed.Host)
			require.Equal(t, "/oauth/obot/callback", completed.Path)
			require.Equal(t, state, completed.Query().Get("state"))
			if tc.denied || tc.expired {
				require.Equal(t, "authentication_failed", completed.Query().Get("error"))
				require.Zero(t, exchanges)
			} else {
				require.Equal(t, "complete", completed.Query().Get("status"))
				require.Equal(t, 1, exchanges)
				token, err := gw.GetMCPOAuthToken(t.Context(), tc.userID, "temporary-preview-id", "https://provider.example/mcp")
				require.NoError(t, err)
				require.NotNil(t, token)
				_, err = gw.GetMCPOAuthToken(t.Context(), "different-user", "temporary-preview-id", "https://provider.example/mcp")
				require.Error(t, err)
			}
			_, err = gw.GetMCPOAuthPendingState(t.Context(), state)
			require.Error(t, err)
			require.Error(t, h.oauthCallback(req), "attempt must not be replayable")
		})
	}
}
