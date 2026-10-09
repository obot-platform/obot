package mcpconnect

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestTokenRefreshAndRejectedCredentials(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "refresh persists token", true: "rejected refresh permits reauthentication"}[rejected], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseForm())
				require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
				require.Equal(t, "old-refresh", r.Form.Get("refresh_token"))
				w.Header().Set("Content-Type", "application/json")
				if rejected {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
					return
				}
				_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`))
			}))
			defer server.Close()
			store := newTokenStore(t.TempDir(), "https://obot.example/mcp-connect/id")
			err := store.save(&oauth2.Config{
				ClientID: "client",
				Endpoint: oauth2.Endpoint{TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInParams},
			}, &oauth2.Token{AccessToken: "old-access", RefreshToken: "old-refresh", Expiry: time.Now().Add(-time.Hour)})
			require.NoError(t, err)
			source, err := store.load(t.Context(), server.Client())
			require.NoError(t, err)
			token, err := source.Token()
			require.NoError(t, err)
			if rejected {
				require.Empty(t, token.AccessToken)
				return
			}
			loaded, err := store.load(t.Context(), server.Client())
			require.NoError(t, err)
			token, err = loaded.Token()
			require.NoError(t, err)
			require.Equal(t, "new-access", token.AccessToken)
			require.Equal(t, "new-refresh", token.RefreshToken)
		})
	}
}
