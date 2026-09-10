package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOAuthCallbackComponentCompletion(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer"}`))
	}))
	defer tokenServer.Close()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	state := newStateManager(client)
	h := handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: state}}
	for _, tt := range []struct {
		name         string
		spec         v1.MCPServerSpec
		instanceSpec *v1.MCPServerInstanceSpec
		want         string
	}{
		{
			name: "shared vmcp",
			spec: v1.MCPServerSpec{VMCPID: "vmcp1shared"},
			want: "/auth/oauth/complete",
		},
		{
			name: "dedicated vmcp",
			spec: v1.MCPServerSpec{VMCPInstanceID: "vmcpi1dedicated"},
			want: "/auth/oauth/complete",
		},
		{
			name: "legacy composite",
			spec: v1.MCPServerSpec{CompositeName: "ms1composite"},
			want: "/auth/oauth/complete",
		},
		{
			name:         "shared vmcp connection",
			spec:         v1.MCPServerSpec{VMCPID: "vmcp1shared"},
			instanceSpec: &v1.MCPServerInstanceSpec{VMCPInstanceID: "vmcpi1selected"},
			want:         "/auth/oauth/complete",
		},
		{
			name:         "legacy composite connection",
			instanceSpec: &v1.MCPServerInstanceSpec{CompositeName: "ms1composite"},
			want:         "/auth/oauth/complete",
		},
		{
			name:         "standalone connection",
			instanceSpec: &v1.MCPServerInstanceSpec{},
			want:         oauthCompletionURL("oar1request"),
		},
		{
			name: "standalone",
			want: oauthCompletionURL("oar1request"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := &v1.MCPServer{Name: "ms1component", Namespace: system.DefaultNamespace, Spec: tt.spec}
			authRequest := &v1.OAuthAuthRequest{Name: "oar1request", Namespace: system.DefaultNamespace, Spec: v1.OAuthAuthRequestSpec{RedirectURI: "https://client.example/callback"}}
			storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(server, authRequest).Build()
			mcpID := server.Name
			if tt.instanceSpec != nil {
				connection := &v1.MCPServerInstance{
					Name:      "msi1connection",
					Namespace: system.DefaultNamespace,
					Spec:      *tt.instanceSpec,
				}
				connection.Spec.MCPServerName = server.Name
				connection.Spec.UserID = "1"
				require.NoError(t, storage.Create(t.Context(), connection))
				mcpID = connection.Name
			}
			require.NoError(t, state.store(t.Context(), "1", mcpID, "https://upstream.example/mcp", authRequest.Name, "state", "verifier", "", &oauth2.Config{
				ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: tokenServer.URL, AuthStyle: oauth2.AuthStyleInParams},
			}))
			response := httptest.NewRecorder()
			require.NoError(t, h.oauthCallback(api.Context{
				Request:        httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?state=state&code=code", nil),
				ResponseWriter: response,
				Storage:        storage,
				User:           &user.DefaultInfo{UID: "1", Name: "user", Groups: []string{types.GroupAuthenticated}},
			}))
			require.Equal(t, http.StatusFound, response.Code)
			require.Equal(t, tt.want, response.Header().Get("Location"))
		})
	}
}

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
