package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestStateManagerExchangesAuthorizationCodeWithStoredResource(t *testing.T) {
	const (
		resourceURL = "https://resource.example.com/mcp"
		mcpURL      = "https://connection.example.com/mcp"
		redirectURL = "https://obot.example.com/oauth/mcp/callback"
		clientID    = "dynamic-client"
		verifier    = "pkce-verifier"
	)

	tokenRequest := make(chan url.Values, 1)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		tokenRequest <- req.Form
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"access_token":"access-token","token_type":"Bearer"}`))
	}))
	defer tokenServer.Close()

	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	gatewayClient := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gatewayClient.Close()) })
	stateManager := newStateManager(gatewayClient)
	conf := &oauth2.Config{
		ClientID:    clientID,
		RedirectURL: redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://auth.example.com/authorize",
			TokenURL:  tokenServer.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	require.NoError(t, stateManager.store(t.Context(), "user", "mcp", mcpURL, "request", "state", verifier, resourceURL, conf))
	_, _, err = stateManager.createToken(t.Context(), "state", "authorization-code", "", "")
	require.NoError(t, err)

	form := <-tokenRequest
	require.Equal(t, resourceURL, form.Get("resource"))
	require.Equal(t, clientID, form.Get("client_id"))
	require.Equal(t, redirectURL, form.Get("redirect_uri"))
	require.Equal(t, verifier, form.Get("code_verifier"))

	conf.Endpoint.AuthURL = "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize"
	require.NoError(t, stateManager.store(t.Context(), "user", "mcp", mcpURL, "request", "entra-state", verifier, resourceURL, conf))
	_, _, err = stateManager.createToken(t.Context(), "entra-state", "authorization-code", "", "")
	require.NoError(t, err)
	require.Empty(t, (<-tokenRequest).Get("resource"))
}
