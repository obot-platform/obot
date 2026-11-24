package client

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// TestDelegatedSetupPendingStateIsBoundToTheUser checks that the pending authorization of a user's account through an
// agent is stored only for the pair's own user, and only to return to a page of Obot's.
func TestDelegatedSetupPendingStateIsBoundToTheUser(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	pair := principal.DelegatedOwner{AgentID: 7, UserID: 9}.String()
	conf := &oauth2.Config{ClientID: "client"}

	for _, tt := range []struct {
		name, owner, initiator, returnPath string
	}{
		{name: "another user", owner: pair, initiator: "10", returnPath: "/agents/approve/adr1"},
		{name: "the agent", owner: pair, initiator: "agent:7", returnPath: "/agents/approve/adr1"},
		{name: "a user's own", owner: "9", initiator: "9", returnPath: "/agents/approve/adr1"},
		{name: "another site", owner: pair, initiator: "9", returnPath: "https://example.com/"},
		{name: "a scheme-relative URL", owner: pair, initiator: "9", returnPath: "//example.com/"},
	} {
		require.Error(t, c.CreateMCPOAuthDelegatedSetupPendingState(ctx, tt.owner, "msi1pair", "https://mcp.example.com", "", tt.initiator, tt.returnPath, "state-"+tt.name, "verifier", "", conf), tt.name)
	}

	require.NoError(t, c.CreateMCPOAuthDelegatedSetupPendingState(ctx, pair, "msi1pair", "https://mcp.example.com", "", "9", "/agents/approve/adr1", "state", "verifier", "", conf))
	pending, err := c.GetMCPOAuthPendingState(ctx, "state")
	require.NoError(t, err)
	require.Equal(t, pair, pending.UserID)
	require.Equal(t, "9", pending.GrantInitiatorUserID)
	require.Equal(t, "/agents/approve/adr1", pending.ReturnPath)
}

// TestDeleteDelegatedOwnerMCPOAuthState checks that the upstream tokens and pending authorizations of the pairs a
// cleanup names are deleted, and no one else's: not another pair's, a user's own, an agent's own, or a vMCP's grant.
func TestDeleteDelegatedOwnerMCPOAuthState(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	owners := []string{
		principal.DelegatedOwner{AgentID: 7, UserID: 9}.String(),
		principal.DelegatedOwner{AgentID: 7, UserID: 10}.String(),
		principal.DelegatedOwner{AgentID: 8, UserID: 9}.String(),
		"9",
		"agent:7",
		principal.PreconfiguredOAuthOwner("vmcp1"),
	}
	conf := &oauth2.Config{ClientID: "client"}
	for _, owner := range owners {
		require.NoError(t, c.db.WithContext(ctx).Create(&types.MCPOAuthToken{UserID: owner, MCPID: "msi1one", URL: "https://mcp.example.com", AccessToken: owner, Expiry: time.Now().Add(time.Hour)}).Error)
		require.NoError(t, c.CreateMCPOAuthPendingState(ctx, owner, owner, "msi1one", "https://mcp.example.com", "", "", "state-"+owner, "verifier", "", conf))
	}
	remaining := func() []string {
		var tokens, pending []string
		require.NoError(t, c.db.WithContext(ctx).Model(&types.MCPOAuthToken{}).Order("user_id").Pluck("user_id", &tokens).Error)
		require.NoError(t, c.db.WithContext(ctx).Model(&types.MCPOAuthPendingState{}).Order("user_id").Pluck("user_id", &pending).Error)
		require.Equal(t, tokens, pending)
		return tokens
	}

	// Revoking user 9's delegation of agent 7.
	require.NoError(t, c.DeleteDelegatedOwnerMCPOAuthState(ctx, func(pair principal.DelegatedOwner) bool {
		return pair == principal.DelegatedOwner{AgentID: 7, UserID: 9}
	}))
	require.Equal(t, []string{"9", "agent:7", "agent:7/user:10", "agent:8/user:9", "vmcp-oauth:vmcp1"}, remaining())
	// Deleting user 9.
	require.NoError(t, c.DeleteDelegatedOwnerMCPOAuthState(ctx, func(pair principal.DelegatedOwner) bool { return pair.UserID == 9 }))
	require.Equal(t, []string{"9", "agent:7", "agent:7/user:10", "vmcp-oauth:vmcp1"}, remaining())
	// Deleting agent 7, again.
	for range 2 {
		require.NoError(t, c.DeleteDelegatedOwnerMCPOAuthState(ctx, func(pair principal.DelegatedOwner) bool { return pair.AgentID == 7 }))
	}
	require.Equal(t, []string{"9", "agent:7", "vmcp-oauth:vmcp1"}, remaining())
}
