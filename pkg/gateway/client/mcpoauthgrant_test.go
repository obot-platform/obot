package client

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const (
	grantTestURL = "https://provider.test/mcp"
)

// grantTestDatabase is a database the grant methods are tested on.
type grantTestDatabase struct {
	name string
	open func(tb testing.TB) *Client
}

// grantTestDatabases are SQLite, and PostgreSQL in a schema of its own when OBOT_TEST_POSTGRES_DSN is set, so that
// every query of the grant methods runs on both.
func grantTestDatabases() []grantTestDatabase {
	databases := []grantTestDatabase{{name: "sqlite", open: newTestClient}}
	dsn := os.Getenv(postgresUserLimitTestDSNEnv)
	if dsn == "" {
		return databases
	}
	return append(databases, grantTestDatabase{name: "postgres", open: func(tb testing.TB) *Client {
		tb.Helper()
		admin, err := sservices.New(sservices.Config{DSN: dsn})
		require.NoError(tb, err)
		schema := "obot_grants_" + strings.ReplaceAll(uuid.New().String(), "-", "")
		require.NoError(tb, admin.DB.DB.Exec("CREATE SCHEMA "+schema).Error)
		tb.Cleanup(func() {
			require.NoError(tb, admin.DB.DB.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
			require.NoError(tb, admin.DB.SQLDB.Close())
		})
		u, err := url.Parse(dsn)
		require.NoError(tb, err)
		query := u.Query()
		query.Set("search_path", schema)
		u.RawQuery = query.Encode()
		return newTestClientWithDSN(tb, u.String())
	}})
}

// TestMCPOAuthGrantWrites checks the writes of vMCP preconfigured OAuth grants: only a vMCP's grant owner can hold one,
// a grant keeps who authorized it across refreshes, an invalid grant is never refreshed back to life, and a new
// authorization replaces it and clears its state. None of them triggers an MCPServer.
func TestMCPOAuthGrantWrites(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			client := database.open(t)
			var triggered []string
			client.mcpOAuthTokenTrigger = func(_ context.Context, mcpID string) error {
				triggered = append(triggered, mcpID)
				return nil
			}
			owner, mcpID := principal.PreconfiguredOAuthOwner("vmcp1test"), "vmcp1test/component"
			conf := &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: "https://provider.test/token"}, Scopes: []string{"read", "write"}}

			// No user's or agent's token can be written or deleted as a grant.
			for _, notAGrant := range []string{"7", "agent:7", "vmcp-oauth:", "vmcp-profile:vmcp1test/vp1agents"} {
				require.ErrorIs(t, client.ReplaceMCPOAuthGrant(t.Context(), notAGrant, mcpID, grantTestURL, "grant", "1", conf, &oauth2.Token{AccessToken: "a"}), ErrNotPreconfiguredOAuth)
				require.ErrorIs(t, client.DeleteMCPOAuthGrantsForOwner(t.Context(), notAGrant), ErrNotPreconfiguredOAuth)
			}

			require.NoError(t, client.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, grantTestURL, "grant-1", "1", conf, &oauth2.Token{AccessToken: "first", RefreshToken: "refresh"}))
			stored, err := client.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
			require.NoError(t, err)
			require.Equal(t, "grant-1", stored.GrantID)
			require.Equal(t, "1", stored.AuthorizedBy)
			require.NotNil(t, stored.AuthorizedAt)
			authorizedAt := *stored.AuthorizedAt

			// A refresh keeps the grant's identity and who authorized it.
			require.NoError(t, client.RefreshMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL, "grant-1", conf, &oauth2.Token{AccessToken: "second", RefreshToken: "refresh-2"}))
			stored, err = client.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
			require.NoError(t, err)
			require.Equal(t, "second", stored.AccessToken)
			require.Equal(t, "grant-1", stored.GrantID)
			require.Equal(t, "1", stored.AuthorizedBy)
			require.True(t, authorizedAt.Equal(*stored.AuthorizedAt))

			// Marking another grant invalid does nothing; marking this one makes it invalid once.
			require.False(t, invalidateGrant(t, client, owner, mcpID, "grant-other", "refused"))
			require.True(t, invalidateGrant(t, client, owner, mcpID, "grant-1", "refused"))
			require.False(t, invalidateGrant(t, client, owner, mcpID, "grant-1", "refused again"))
			require.ErrorContains(t, client.RefreshMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL, "grant-1", conf, &oauth2.Token{AccessToken: "third"}), "invalid")

			metadata, err := client.MCPOAuthGrantMetadata(t.Context(), []string{owner}, []string{mcpID})
			require.NoError(t, err)
			require.Len(t, metadata, 1)
			require.NotNil(t, metadata[0].InvalidatedAt)
			require.Equal(t, "refused", metadata[0].InvalidReason)
			require.Equal(t, "read write", metadata[0].Scopes)

			// Authorizing again replaces the grant, and it works again.
			require.NoError(t, client.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, grantTestURL, "grant-2", "2", conf, &oauth2.Token{AccessToken: "fourth"}))
			stored, err = client.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
			require.NoError(t, err)
			require.Equal(t, "grant-2", stored.GrantID)
			require.Equal(t, "2", stored.AuthorizedBy)
			require.Nil(t, stored.InvalidatedAt)
			require.Empty(t, stored.InvalidReason)

			require.Empty(t, triggered, "a grant's MCPID names no MCPServer")
		})
	}
}

// TestMCPOAuthGrantDeletesAreExact checks that grants are deleted by the IDs listed for them, so a grant written since
// is kept, and that deleting a vMCP's grants leaves every other vMCP's grants and every user's and agent's token alone.
func TestMCPOAuthGrantDeletesAreExact(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			client := database.open(t)
			owner, other := principal.PreconfiguredOAuthOwner("vmcp1test"), principal.PreconfiguredOAuthOwner("vmcp1other")
			mcpID := "vmcp1test/component"
			conf := &oauth2.Config{ClientID: "client"}
			token := &oauth2.Token{AccessToken: "token"}
			require.NoError(t, client.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, grantTestURL, "grant-new", "1", conf, token))
			require.NoError(t, client.ReplaceMCPOAuthGrant(t.Context(), other, mcpID, grantTestURL, "grant-other", "1", conf, token))
			for _, userID := range []string{"7", "agent:7"} {
				require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), userID, mcpID, grantTestURL, "", conf, token))
			}

			require.NoError(t, client.DeleteMCPOAuthGrants(t.Context(), owner, mcpID, []string{"grant-listed-before"}))
			_, err := client.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
			require.NoError(t, err, "a grant written after the listing was deleted")

			require.NoError(t, client.DeleteMCPOAuthGrantsForOwner(t.Context(), owner))
			_, err = client.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
			require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
			for _, kept := range []string{other, "7", "agent:7"} {
				_, err := client.GetMCPOAuthToken(t.Context(), kept, mcpID, grantTestURL)
				require.NoError(t, err, "the token of %s was deleted with another vMCP's grants", kept)
			}
		})
	}
}

// TestMCPOAuthGrantPendingStateIsBound checks that the pending authorization of a vMCP's grant is stored only with
// the administrator and component definition it is bound to, and that the binding is read back with it.
func TestMCPOAuthGrantPendingStateIsBound(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			client := database.open(t)
			owner := principal.PreconfiguredOAuthOwner("vmcp1test")
			conf := &oauth2.Config{ClientID: "client", ClientSecret: "secret"}
			require.ErrorIs(t, client.CreateMCPOAuthGrantPendingState(t.Context(), "7", "vmcp1test/component", grantTestURL, "", "1", "hash", "state", "verifier", "", conf), ErrNotPreconfiguredOAuth)
			require.Error(t, client.CreateMCPOAuthGrantPendingState(t.Context(), owner, "vmcp1test/component", grantTestURL, "", "", "hash", "state", "verifier", "", conf))
			require.Error(t, client.CreateMCPOAuthGrantPendingState(t.Context(), owner, "vmcp1test/component", grantTestURL, "", "1", "", "state", "verifier", "", conf))

			require.NoError(t, client.CreateMCPOAuthGrantPendingState(t.Context(), owner, "vmcp1test/component", grantTestURL, "", "1", "hash", "state", "verifier", "", conf))
			pending, err := client.GetMCPOAuthPendingState(t.Context(), "state")
			require.NoError(t, err)
			require.Equal(t, owner, pending.UserID)
			require.Equal(t, "1", pending.GrantInitiatorUserID)
			require.Equal(t, "hash", pending.GrantDefinitionHash)
			require.Equal(t, "secret", pending.ClientSecret)
			require.Empty(t, pending.OAuthAuthRequestID)

			// It is consumed once.
			_, err = client.ConsumeMCPOAuthPendingState(t.Context(), "state")
			require.NoError(t, err)
			_, err = client.ConsumeMCPOAuthPendingState(t.Context(), "state")
			require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
		})
	}
}

// TestMCPOAuthGrantDiscoveryRevision checks that the discovery revision of an agent's grants changes when a grant is
// reauthorized, invalidated or removed, and not when it is refreshed.
func TestMCPOAuthGrantDiscoveryRevision(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			client := database.open(t)
			owner, mcpID := principal.PreconfiguredOAuthOwner("vmcp1test"), "vmcp1test/component"
			keys := []types.MCPOAuthTokenKey{{UserID: owner, MCPID: mcpID}}
			conf := &oauth2.Config{ClientID: "client"}
			revision := func() string {
				t.Helper()
				value, err := client.MCPOAuthGrantDiscoveryRevision(t.Context(), keys)
				require.NoError(t, err)
				return value
			}
			none := revision()
			require.NoError(t, client.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, grantTestURL, "grant-1", "1", conf, &oauth2.Token{AccessToken: "a"}))
			first := revision()
			require.NotEqual(t, none, first)
			require.NoError(t, client.RefreshMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL, "grant-1", conf, &oauth2.Token{AccessToken: "b"}))
			require.Equal(t, first, revision())
			invalidateGrant(t, client, owner, mcpID, "grant-1", "refused")
			invalid := revision()
			require.NotEqual(t, first, invalid)
			require.NoError(t, client.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, grantTestURL, "grant-2", "1", conf, &oauth2.Token{AccessToken: "c"}))
			require.NotEqual(t, invalid, revision())
			require.NoError(t, client.DeleteMCPOAuthGrantsForOwner(t.Context(), owner))
			require.Equal(t, none, revision())
		})
	}
}

// invalidateGrant marks the grant grantID of owner for mcpID invalid as a refused refresh does, holding the grant's
// refresh lock (MCPOAuthRefresh.Invalidate). It reports whether it marked the grant.
func invalidateGrant(t *testing.T, c *Client, owner, mcpID, grantID, reason string) bool {
	t.Helper()
	var marked bool
	require.NoError(t, c.WithMCPOAuthRefreshLock(t.Context(), owner, mcpID, func(_ context.Context, refresh *MCPOAuthRefresh) error {
		var err error
		marked, err = refresh.Invalidate(grantID, reason)
		return err
	}))
	return marked
}
