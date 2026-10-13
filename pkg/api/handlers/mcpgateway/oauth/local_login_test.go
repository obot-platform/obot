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
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestLocalLoginURL(t *testing.T) {
	f := &MCPOAuthHandlerFactory{baseURL: "https://obot.example"}
	upstream := "https://provider.example/authorize?state=attempt"
	local := mcp.ServerConfig{LocalhostCallbackEnabled: true}
	got, err := f.localLoginURL(upstream, local)
	require.NoError(t, err)
	require.Equal(t, "https://obot.example/oauth/mcp/login/attempt", got)
	got, err = f.localLoginURL(upstream, mcp.ServerConfig{})
	require.NoError(t, err)
	require.Equal(t, upstream, got)
	got, err = f.localLoginURL("", local)
	require.NoError(t, err)
	require.Empty(t, got)
	_, err = f.localLoginURL("https://provider.example/authorize", local)
	require.Error(t, err)
}

func TestLocalLoginPreservesAttempt(t *testing.T) {
	for _, tc := range []struct {
		name            string
		userID          string
		denied          bool
		expired         bool
		hosted          bool
		authRequestID   string
		wrongUser       bool
		unauthenticated bool
		unapproved      bool
	}{
		{
			name:            "client callback without browser session",
			userID:          "1",
			authRequestID:   "auth1",
			unauthenticated: true,
		},
		{
			name:          "client callback wrong user",
			userID:        "1",
			authRequestID: "auth1",
			wrongUser:     true,
		},
		{
			name:          "client callback without consent",
			userID:        "1",
			authRequestID: "auth1",
			unapproved:    true,
		},
		{
			name:          "client connection",
			userID:        "1",
			authRequestID: "auth1",
		},
		{
			name:          "client denial",
			userID:        "1",
			authRequestID: "auth1",
			denied:        true,
		},
		{
			name:          "client expired",
			userID:        "1",
			authRequestID: "auth1",
			expired:       true,
		},
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
			sm := newStateManager(gw, fixedTokenClient(provider.Client()))
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
				stateMgr:           sm,
				userID:             tc.userID,
				mcpID:              "temporary-preview-id",
				mcpURL:             "https://provider.example/mcp",
				localLogin:         !tc.hosted,
				oauthAuthRequestID: tc.authRequestID,
			}
			state, _, err := oauthHandler.NewState(t.Context(), conf, "https://provider.example/mcp", "private-verifier")
			require.NoError(t, err)
			if tc.expired {
				require.NoError(t, services.DB.DB.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("mcp_id = ?", "temporary-preview-id").Update("created_at", time.Now().Add(-11*time.Minute)).Error)
			}
			h := handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: sm}}
			fixture := newVMCPOAuthFixture(t)
			req := fixture.request(t.Context())
			if tc.authRequestID != "" {
				req.User = &user.DefaultInfo{UID: "1", Name: "user", Groups: []string{types.GroupAuthenticated}}
				require.NoError(t, fixture.storage.Create(t.Context(), &v1.OAuthAuthRequest{
					Name: tc.authRequestID, Namespace: system.DefaultNamespace,
					Spec: v1.OAuthAuthRequestSpec{UserID: 1, ConsentApproved: !tc.unapproved, RedirectURI: "https://client.example/callback"},
				}))
			}
			req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/login/"+state, nil)
			req.SetPathValue("state", state)
			response := httptest.NewRecorder()
			req.ResponseWriter = response
			err = h.localLogin(req)
			if tc.expired || tc.hosted {
				require.Error(t, err)
				if tc.expired {
					var httpErr *types.ErrHTTP
					require.ErrorAs(t, err, &httpErr)
					require.Equal(t, http.StatusGone, httpErr.Code)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
				var metadata types.MCPLocalLogin
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &metadata))
				require.Equal(t, conf.RedirectURL, metadata.RedirectURL)
				require.InDelta(t, 600, metadata.ExpiresIn, 2)
				auth, err := url.Parse(metadata.AuthorizationURL)
				require.NoError(t, err)
				require.Equal(t, state, auth.Query().Get("state"))
				require.Equal(t, "S256", auth.Query().Get("code_challenge_method"))
				require.NotEmpty(t, auth.Query().Get("code_challenge"))
				require.NotContains(t, response.Body.String(), "private-secret")
				require.NotContains(t, response.Body.String(), "private-verifier")
				require.NotContains(t, response.Body.String(), "temporary-preview-id")
				// Reading the command again reports the original attempt's remaining
				// lifetime; polling must not grant another ten minutes.
				require.NoError(t, services.DB.DB.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("mcp_id = ?", "temporary-preview-id").Update("created_at", time.Now().Add(-7*time.Minute)).Error)
				response = httptest.NewRecorder()
				req.ResponseWriter = response
				require.NoError(t, h.localLogin(req))
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &metadata))
				require.InDelta(t, 180, metadata.ExpiresIn, 2)
			}
			callbackQuery := "?state=" + state + "&code=provider-code"
			if tc.denied {
				callbackQuery = "?state=" + state + "&error=access_denied"
			}
			req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback"+callbackQuery, nil)
			response = httptest.NewRecorder()
			req.ResponseWriter = response
			if tc.wrongUser {
				req.User = &user.DefaultInfo{UID: "2", Name: "other", Groups: []string{types.GroupAuthenticated}}
			}
			if tc.unauthenticated {
				req.User = &user.DefaultInfo{}
			}
			if tc.wrongUser || tc.unapproved || tc.unauthenticated {
				require.NoError(t, h.oauthCallback(req))
				require.Equal(t, http.StatusFound, response.Code)
				completion, err := url.Parse(response.Header().Get("Location"))
				require.NoError(t, err)
				require.Equal(t, "localhost:54321", completion.Host)
				require.Equal(t, "/oauth/obot/callback", completion.Path)
				require.Equal(t, state, completion.Query().Get("state"))
				require.Equal(t, "authentication_failed", completion.Query().Get("error"))
				require.Empty(t, completion.Query().Get("code"))
				require.Zero(t, exchanges)
				_, err = gw.GetMCPOAuthPendingState(t.Context(), state)
				require.NoError(t, err, "unauthorized callbacks must not consume the user's attempt")
				return
			}
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
				require.Equal(t, tc.authRequestID, token.OAuthAuthRequestID)
				_, err = gw.GetMCPOAuthToken(t.Context(), "different-user", "temporary-preview-id", "https://provider.example/mcp")
				require.Error(t, err)
			}
			_, err = gw.GetMCPOAuthPendingState(t.Context(), state)
			require.Error(t, err)
			require.Error(t, h.oauthCallback(req), "attempt must not be replayable")
		})
	}
}

func TestLocalLoginURLIdentifiesServer(t *testing.T) {
	f := &MCPOAuthHandlerFactory{baseURL: "https://obot.example"}
	got, err := f.localLoginURL("https://provider.example/authorize?state=attempt", mcp.ServerConfig{
		LocalhostCallbackEnabled: true,
		MCPServerDisplayName:     "Acme & Co / Tools",
	})
	require.NoError(t, err)
	u, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "/oauth/mcp/login/attempt", u.Path)
	require.Equal(t, "Acme & Co / Tools", u.Query().Get("serverName"))
}
