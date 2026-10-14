package oauth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

func TestLocalLoginStorageFailure(t *testing.T) {
	for _, callback := range []bool{false, true} {
		name := "status"
		if callback {
			name = "callback"
		}
		t.Run(name, func(t *testing.T) {
			services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
			require.NoError(t, err)
			db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate())
			gw := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
			t.Cleanup(func() { require.NoError(t, gw.Close()) })
			sm := newStateManager(gw, fixedTokenClient(http.DefaultClient))
			oauthHandler := &mcpOAuthHandler{
				stateMgr:   sm,
				userID:     "1",
				mcpID:      "server",
				localLogin: true,
			}
			state, _, err := oauthHandler.NewState(t.Context(), &oauth2.Config{
				ClientID:    "client",
				RedirectURL: "http://localhost:54321/callback",
				Endpoint:    oauth2.Endpoint{AuthURL: "https://provider.example/authorize"},
			}, "https://provider.example/mcp", "verifier")
			require.NoError(t, err)
			storageErr := errors.New("temporary pending-state lookup failure")
			queries := 0
			failAt := 1
			if callback {
				failAt = 2
			} // The debugger check performs the first lookup.
			require.NoError(t, services.DB.DB.Callback().Query().Before("gorm:query").Register("fail_pending_lookup", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "MCPOAuthPendingState" {
					queries++
					if queries == failAt {
						_ = tx.AddError(storageErr)
					}
				}
			}))
			h := handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: sm}}
			fixture := newVMCPOAuthFixture(t)
			req := fixture.request(t.Context())
			req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?state="+state+"&code=code", nil)
			req.SetPathValue("state", state)
			response := httptest.NewRecorder()
			req.ResponseWriter = response
			if callback {
				err = h.oauthCallback(req)
			} else {
				err = h.localLogin(req)
			}
			require.ErrorIs(t, err, storageErr)
			var httpErr *types.ErrHTTP
			require.False(t, errors.As(err, &httpErr), "storage failures must not become a consumed-attempt 400")
			require.Equal(t, failAt, queries, "do not retry the lookup through the hosted callback path")
			require.Empty(t, response.Header().Get("Location"))
			_, err = gw.GetMCPOAuthPendingState(t.Context(), state)
			require.NoError(t, err, "the original attempt remains available after storage recovers")
		})
	}
}
