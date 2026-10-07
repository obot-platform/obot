package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type oauthCallbackTokenServer struct {
	url   string
	count atomic.Int32
}

func TestOAuthCallbackDebuggerState(t *testing.T) {
	for _, tt := range []struct {
		name  string
		query url.Values
		want  url.Values
	}{
		{
			name: "authorization code",
			query: url.Values{
				"state": {"debugger-state"},
				"code":  {"authorization-code"},
			},
			want: url.Values{
				"state": {"debugger-state"},
				"code":  {"authorization-code"},
			},
		},
		{
			name: "error with description",
			query: url.Values{
				"state":             {"debugger-state"},
				"error":             {"access_denied"},
				"error_description": {"user denied access"},
			},
			want: url.Values{
				"state":             {"debugger-state"},
				"error":             {"access_denied"},
				"error_description": {"user denied access"},
			},
		},
		{
			name: "error without description",
			query: url.Values{
				"state": {"debugger-state"},
				"error": {"access_denied"},
			},
			want: url.Values{
				"state": {"debugger-state"},
				"error": {"access_denied"},
			},
		},
		{
			name: "error takes precedence over code",
			query: url.Values{
				"state": {"debugger-state"},
				"code":  {"authorization-code"},
				"error": {"server_error"},
			},
			want: url.Values{
				"state": {"debugger-state"},
				"error": {"server_error"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, stateMgr, tokenRequests := newOAuthCallbackTestHandler(t)
			require.NoError(t, stateMgr.store(t.Context(), "1", "ms1debugger", "https://upstream.example/mcp", handlers.OAuthDebuggerPendingStateMarker, "debugger-state", "verifier", "", tokenRequests.config()))

			response := httptest.NewRecorder()
			require.NoError(t, h.oauthCallback(newOAuthCallbackTestContext(t, response, tt.query, nil)))

			require.Equal(t, http.StatusFound, response.Code)
			location, err := url.Parse(response.Header().Get("Location"))
			require.NoError(t, err)
			assert.Equal(t, "/oauth-debugger/callback", location.Path)
			assert.Equal(t, tt.want, location.Query())

			// The debugger exchanges the code itself in a later step, so the callback must neither
			// exchange it nor consume the pending state.
			assert.Zero(t, tokenRequests.count.Load())
			_, err = stateMgr.gatewayClient.GetMCPOAuthPendingState(t.Context(), "debugger-state")
			assert.NoError(t, err)
		})
	}
}

func TestOAuthCallbackNonDebuggerStateUsesExistingFlow(t *testing.T) {
	t.Run("state without an auth request", func(t *testing.T) {
		h, stateMgr, tokenRequests := newOAuthCallbackTestHandler(t)
		require.NoError(t, stateMgr.store(t.Context(), "1", "ms1server", "https://upstream.example/mcp", "", "ordinary-state", "verifier", "", tokenRequests.config()))

		response := httptest.NewRecorder()
		require.NoError(t, h.oauthCallback(newOAuthCallbackTestContext(t, response, url.Values{
			"state": {"ordinary-state"},
			"code":  {"authorization-code"},
		}, nil)))

		require.Equal(t, http.StatusFound, response.Code)
		assert.Equal(t, "/auth/oauth/complete", response.Header().Get("Location"))
		assert.EqualValues(t, 1, tokenRequests.count.Load())
		_, err := stateMgr.gatewayClient.GetMCPOAuthPendingState(t.Context(), "ordinary-state")
		assert.Error(t, err, "pending state should be consumed by the token exchange")
	})

	t.Run("state with an auth request", func(t *testing.T) {
		h, stateMgr, tokenRequests := newOAuthCallbackTestHandler(t)
		server := &v1.MCPServer{Name: "ms1server", Namespace: system.DefaultNamespace}
		authRequest := &v1.OAuthAuthRequest{Name: "oar1request", Namespace: system.DefaultNamespace, Spec: v1.OAuthAuthRequestSpec{RedirectURI: "https://client.example/callback"}}
		require.NoError(t, stateMgr.store(t.Context(), "1", server.Name, "https://upstream.example/mcp", authRequest.Name, "ordinary-state", "verifier", "", tokenRequests.config()))

		response := httptest.NewRecorder()
		require.NoError(t, h.oauthCallback(newOAuthCallbackTestContext(t, response, url.Values{
			"state": {"ordinary-state"},
			"code":  {"authorization-code"},
		}, []kclient.Object{server, authRequest})))

		require.Equal(t, http.StatusFound, response.Code)
		assert.Equal(t, oauthCompletionURL(authRequest.Name), response.Header().Get("Location"))
		assert.EqualValues(t, 1, tokenRequests.count.Load())
	})

	t.Run("unknown state", func(t *testing.T) {
		h, _, tokenRequests := newOAuthCallbackTestHandler(t)

		response := httptest.NewRecorder()
		err := h.oauthCallback(newOAuthCallbackTestContext(t, response, url.Values{
			"state": {"unknown-state"},
			"code":  {"authorization-code"},
		}, nil))

		var httpErr *types.ErrHTTP
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Empty(t, response.Header().Get("Location"))
		assert.Zero(t, tokenRequests.count.Load())
	})

	t.Run("missing state", func(t *testing.T) {
		h, _, tokenRequests := newOAuthCallbackTestHandler(t)

		response := httptest.NewRecorder()
		err := h.oauthCallback(newOAuthCallbackTestContext(t, response, url.Values{
			"code": {"authorization-code"},
		}, nil))

		var httpErr *types.ErrHTTP
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Empty(t, response.Header().Get("Location"))
		assert.Zero(t, tokenRequests.count.Load())
	})
}

func (s *oauthCallbackTokenServer) config() *oauth2.Config {
	return &oauth2.Config{
		ClientID: "client",
		Endpoint: oauth2.Endpoint{
			TokenURL:  s.url,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

func newOAuthCallbackTestHandler(t *testing.T) (handler, *stateManager, *oauthCallbackTokenServer) {
	t.Helper()

	tokenRequests := &oauthCallbackTokenServer{}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tokenRequests.count.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer"}`))
	}))
	t.Cleanup(tokenServer.Close)
	tokenRequests.url = tokenServer.URL

	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	stateMgr := newStateManager(client)
	return handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: stateMgr}}, stateMgr, tokenRequests
}

func newOAuthCallbackTestContext(t *testing.T, response http.ResponseWriter, query url.Values, objects []kclient.Object) api.Context {
	t.Helper()

	builder := clientfake.NewClientBuilder().WithScheme(scheme.Scheme)
	for _, obj := range objects {
		builder = builder.WithObjects(obj)
	}
	return api.Context{
		Request:        httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?"+query.Encode(), nil),
		ResponseWriter: response,
		Storage:        builder.Build(),
		User:           &user.DefaultInfo{UID: "1", Name: "user", Groups: []string{types.GroupAuthenticated}},
	}
}
