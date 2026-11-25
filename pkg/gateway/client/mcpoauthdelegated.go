package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// The upstream OAuth tokens of what a user sets up through an agent are MCP OAuth tokens whose owner (UserID) is a
// principal.DelegatedOwner, keyed by the connection of the pair's instance as a user's are keyed by the user's
// connection. They are read, refreshed and replaced as a user's are. These methods add what only they need: a pending
// authorization bound to the user who started it and to the page it returns to, and their deletion by pair when a
// delegation is revoked or an agent or a user is deleted.

const (
	// delegatedOwnerPattern matches, with LIKE, every owner that may be a principal.DelegatedOwner's encoding. Every
	// match is parsed again before anything is deleted.
	delegatedOwnerPattern = "agent:%/user:%"
)

// CreateMCPOAuthDelegatedSetupPendingState stores the pending authorization of an upstream account for the pair owner,
// a principal.DelegatedOwner, and its connection mcpID, started by the user with the encoded principal initiatorUserID,
// who must be the pair's user, from the Obot page at returnPath. Only that user can complete it, and the callback sends
// them back to returnPath (MCPOAuthPendingState.ReturnPath).
func (c *Client) CreateMCPOAuthDelegatedSetupPendingState(ctx context.Context, owner, mcpID, mcpURL, tunnelName, initiatorUserID, returnPath, state, verifier, resourceURL string, oauthConf *oauth2.Config) error {
	pair, ok := principal.ParseDelegatedOwner(owner)
	if !ok {
		return fmt.Errorf("owner %q is not a user's setup through an agent", owner)
	}
	if initiatorUserID != principal.User(pair.UserID).String() {
		return fmt.Errorf("a user's setup through an agent is authorized only by that user")
	}
	if !strings.HasPrefix(returnPath, "/") || strings.HasPrefix(returnPath, "//") {
		return fmt.Errorf("the pending authorization of a user's setup through an agent must return to an Obot page")
	}
	ps := newBoundMCPOAuthPendingState(owner, mcpID, mcpURL, tunnelName, initiatorUserID, state, verifier, resourceURL, oauthConf)
	ps.ReturnPath = returnPath
	return c.createBoundMCPOAuthPendingState(ctx, ps)
}

// DeleteDelegatedOwnerMCPOAuthState deletes the upstream OAuth tokens, and the pending authorizations, of every pair
// (principal.DelegatedOwner) that match reports true for. No other owner's are ever deleted. Deleting what is already
// gone succeeds.
func (c *Client) DeleteDelegatedOwnerMCPOAuthState(ctx context.Context, match func(principal.DelegatedOwner) bool) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&types.MCPOAuthToken{}, &types.MCPOAuthPendingState{}} {
			var owners []string
			if err := tx.Model(model).Distinct("user_id").Where("user_id LIKE ?", delegatedOwnerPattern).Pluck("user_id", &owners).Error; err != nil {
				return err
			}
			var matching []string
			for _, owner := range owners {
				if pair, ok := principal.ParseDelegatedOwner(owner); ok && match(pair) {
					matching = append(matching, owner)
				}
			}
			if len(matching) == 0 {
				continue
			}
			if err := tx.Delete(model, "user_id IN ?", matching).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
