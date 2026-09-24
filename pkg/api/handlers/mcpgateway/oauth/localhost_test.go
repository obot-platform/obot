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
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"

	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
)

func TestLocalhostRedirectURL(t *testing.T) {
	for _, tt := range []struct {
		name     string
		redirect string
		path     string
		want     string
		wantErr  bool
	}{
		{
			name:     "localhost",
			redirect: "http://localhost:1234/oauth/obot/callback?discard=true#fragment",
			want:     "http://localhost:1234/oauth/callback",
		},
		{
			name:     "IPv4 custom",
			redirect: "http://127.0.0.1:1234/oauth/obot/callback",
			path:     "/custom",
			want:     "http://127.0.0.1:1234/custom",
		},
		{
			name:     "IPv6",
			redirect: "http://[::1]:1234/oauth/obot/callback",
			want:     "http://[::1]:1234/oauth/callback",
		},
		{
			name:     "escaped custom path",
			redirect: "http://localhost:1234/oauth/obot/callback",
			path:     "/custom%20callback",
			want:     "http://localhost:1234/custom%20callback",
		},
		{
			name:     "remote",
			redirect: "http://example.com:1234/callback",
			wantErr:  true,
		},
		{
			name:     "userinfo",
			redirect: "http://user@localhost:1234/callback",
			wantErr:  true,
		},
		{
			name:     "no port",
			redirect: "http://localhost/callback",
			wantErr:  true,
		},
		{
			name:     "reserved path",
			redirect: "http://localhost:1234/oauth/obot/callback",
			path:     "/oauth/obot/callback",
			wantErr:  true,
		},
		{
			name:     "other loopback client",
			redirect: "http://localhost:1234/callback",
			wantErr:  true,
		},
		{
			name:     "HTTPS",
			redirect: "https://localhost:1234/oauth/obot/callback",
			wantErr:  true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := localhostRedirectURL(tt.redirect, tt.path)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestLocalCallbackDoesNotUseHostedClientMetadata(t *testing.T) {
	f := NewMCPOAuthHandlerFactory("https://obot.example", nil, nil, nil, nil, "", false)
	require.NotEmpty(t, f.clientMetadataForRedirect("https://obot.example/oauth/mcp/callback"))
	require.Empty(t, f.clientMetadataForRedirect("http://localhost:1234/oauth/callback"))
}

func TestUpstreamRedirectUsesOriginatingVMCPRequest(t *testing.T) {
	fixture := newVMCPOAuthFixture(t)
	req := fixture.request(t.Context())
	request := &v1.OAuthAuthRequest{
		Name: "auth1", Namespace: system.DefaultNamespace,
		Spec: v1.OAuthAuthRequestSpec{UserID: req.UserID(), MCPID: vmcpOAuthInstance, RedirectURI: "http://localhost:1234/oauth/obot/callback"},
	}
	require.NoError(t, fixture.storage.Create(t.Context(), request))
	config := mcp.ServerConfig{LocalhostCallbackEnabled: true}
	got, err := fixture.factory.upstreamRedirectURL(req, config, request.Name)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:1234/oauth/callback", got)
	config.LocalhostCallbackEnabled = false
	got, err = fixture.factory.upstreamRedirectURL(req, config, request.Name)
	require.NoError(t, err)
	require.Equal(t, "https://obot.example.com/oauth/mcp/callback", got)
	config.LocalhostCallbackEnabled = true
	_, err = fixture.factory.upstreamRedirectURL(req, config, "")
	require.ErrorContains(t, err, "obot mcp connect")
	request.Spec.UserID++
	require.NoError(t, fixture.storage.Update(t.Context(), request))
	_, err = fixture.factory.upstreamRedirectURL(req, config, request.Name)
	require.ErrorContains(t, err, "belonging to the user")
}

func TestLocalhostPendingStateExchangeAndDenial(t *testing.T) {
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	gw := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gw.Close()) })
	state := newStateManager(gw)
	tokenRequest := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		tokenRequest <- r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"upstream-token","token_type":"Bearer"}`))
	}))
	defer server.Close()
	conf := &oauth2.Config{ClientID: "provider-client", RedirectURL: "http://localhost:1234/custom/callback", Endpoint: oauth2.Endpoint{TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInParams}}
	require.NoError(t, state.store(t.Context(), "1", "connection", "https://provider.example/mcp", "auth-request", "upstream-state", "verifier", "https://provider.example/mcp", conf))
	requestID, connectionID, err := state.createToken(t.Context(), "upstream-state", "provider-code", "", "")
	require.NoError(t, err)
	require.Equal(t, "auth-request", requestID)
	require.Equal(t, "connection", connectionID)
	form := <-tokenRequest
	require.Equal(t, conf.RedirectURL, form.Get("redirect_uri"))
	require.Equal(t, "verifier", form.Get("code_verifier"))
	_, _, err = state.createToken(t.Context(), "upstream-state", "provider-code", "", "")
	require.Error(t, err)

	fixture := newVMCPOAuthFixture(t)
	req := fixture.request(t.Context())
	req.User = &user.DefaultInfo{UID: "1", Name: "user", Groups: []string{types.GroupAuthenticated}}
	authRequest := &v1.OAuthAuthRequest{Name: "auth-request", Namespace: system.DefaultNamespace, Spec: v1.OAuthAuthRequestSpec{UserID: 1, RedirectURI: "http://localhost:1234/oauth/obot/callback", State: "client-state"}}
	require.NoError(t, fixture.storage.Create(t.Context(), authRequest))
	require.NoError(t, state.store(t.Context(), "1", "connection", "https://provider.example/mcp", authRequest.Name, "denied-state", "verifier", "", conf))
	req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?state=denied-state&error=access_denied", nil)
	response := httptest.NewRecorder()
	req.ResponseWriter = response
	h := handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: state}}
	require.NoError(t, h.oauthCallback(req))
	require.Equal(t, http.StatusFound, response.Code)
	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "client-state", location.Query().Get("state"))
	require.Equal(t, "access_denied", location.Query().Get("error"))
	require.Equal(t, "localhost:1234", location.Host)
}
