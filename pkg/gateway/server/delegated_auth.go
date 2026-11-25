package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

// delegatedTokenDecoder reads tokens with which an agent acts on behalf of a user (persistent.TokenService).
type delegatedTokenDecoder interface {
	DecodeDelegatedToken(ctx context.Context, token string) (*persistent.DelegatedToken, error)
}

// DelegatedTokenAuthenticator authenticates tokens with which an agent acts on behalf of a user: those the agent mints
// from a delegation (persistent.TokenService.NewDelegatedToken), and those of a composite vMCP's loopback acting for
// it (persistent.TokenService.NewDelegatedLoopbackToken). It is the one authenticator that opts in to them, and only
// on the routes an agent may act on behalf of a user on (authz.AcceptsDelegatedTokens). Everywhere else it declines,
// and the token service refuses the token (persistent.ErrDelegatedToken).
//
// The caller is the agent's principal, as its API key authenticator builds it, with the user recorded as the user it
// acts for (principal.RecordOnBehalfOf). It has the agent's own grants, within what the key that minted the token
// grants, and never a role or group of the user's. What authentication established about the user, such as their
// current groups, is recorded beside it (agentprofile.RecordDelegation), for guardrails that cover the user and the
// identity passed on to MCP servers.
//
// On every request, it checks that the delegation exists and both its agent and its user are active
// (client.DelegationValid, which trusts a valid delegation for 15 seconds), that the key that minted the token is the
// agent's and neither revoked nor expired, and that a profile of the agent still matches the user with their
// current groups. A token that fails any of these is refused with *client.AgentDelegationInvalidError, rather than
// declined, so that no later authenticator takes the request as anyone.
type DelegatedTokenAuthenticator struct {
	client *client.Client
	tokens delegatedTokenDecoder
}

// NewDelegatedTokenAuthenticator returns the authenticator of tokens with which an agent acts on behalf of a user.
func NewDelegatedTokenAuthenticator(client *client.Client, tokens *persistent.TokenService) *DelegatedTokenAuthenticator {
	return &DelegatedTokenAuthenticator{
		client: client,
		tokens: tokens,
	}
}

// AuthenticateRequest implements authenticator.Request.
func (a *DelegatedTokenAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	// The token is read from the headers the token service reads (persistent.RequestToken), so that one this
	// authenticator refuses is one that the guards of a server that does not authenticate callers see as an agent's
	// too (persistent.TokenService.AgentTokens), and is never taken for the "nobody" owner.
	token := persistent.RequestToken(req)
	if token == "" || !persistent.ClaimsDelegatedToken(token) || !authz.AcceptsDelegatedTokens(req) {
		return nil, false, nil
	}
	ctx := req.Context()

	delegated, err := a.tokens.DecodeDelegatedToken(ctx, token)
	if err != nil {
		return nil, false, delegationInvalid("the token is malformed, expired, or not one Obot issued")
	}

	valid, err := a.client.DelegationValid(ctx, delegated.DelegationID, delegated.AgentID, delegated.UserID)
	if err != nil {
		return nil, false, &client.AgentAccessLookupError{AgentID: delegated.AgentID, Err: err}
	}
	if !valid {
		return nil, false, delegationInvalid("the delegation was revoked, or its agent or user is not active")
	}

	agent, err := a.client.ActiveAgent(ctx, delegated.AgentID)
	if denied, ok := errors.AsType[*client.AgentAccessDeniedError](err); ok {
		return nil, false, delegationInvalid(fmt.Sprintf("the agent is %s", denied.Status))
	} else if err != nil {
		return nil, false, err
	}

	key, err := a.mintingKey(ctx, delegated)
	if err != nil {
		return nil, false, err
	}

	user, groupIDs, err := a.client.UserByIDWithEffectiveRole(ctx, delegated.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, delegationInvalid("the user was deleted")
	} else if err != nil {
		return nil, false, &client.UserAccessLookupError{UserID: delegated.UserID, Err: err}
	}
	if status := user.Status(); status != types2.UserStatusActive {
		return nil, false, delegationInvalid(fmt.Sprintf("the user is %s", status))
	}
	// The user's groups are read for matching only: they decide whether an agent profile matches the user, and never
	// become the caller's.
	userCaller, ok := subjects.CallerFromUserInfo(client.UserInfo(user, groupIDs))
	if !ok {
		return nil, false, delegationInvalid("the user is not a well-formed principal")
	}
	if !agentprofile.Matches(agent.Profiles, userCaller) {
		return nil, false, delegationInvalid("no profile of the agent includes the user any more")
	}

	groups, extra, err := delegatedGrants(delegated, key)
	if err != nil {
		return nil, false, err
	}
	info := client.AgentUserInfo(agent, groups, extra)
	// AgentUserInfo records the agent's kind, which clears any on-behalf-of user, so the user is recorded after it.
	principal.RecordOnBehalfOf(info.Extra, delegated.UserID)
	if err := agentprofile.RecordDelegation(info.Extra, agentprofile.Delegation{
		ID:          delegated.DelegationID,
		User:        userCaller,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	}); err != nil {
		return nil, false, fmt.Errorf("failed to record the delegation of agent %d: %w", delegated.AgentID, err)
	}

	return &authenticator.Response{User: info}, true, nil
}

// mintingKey returns the agent's key that minted the token, and refuses the token if the key is no longer valid: it is
// not the agent's, or it is revoked or expired.
func (a *DelegatedTokenAuthenticator) mintingKey(ctx context.Context, delegated *persistent.DelegatedToken) (*gatewaytypes.APIKey, error) {
	key, err := a.client.GetAgentAPIKey(ctx, delegated.AgentID, delegated.APIKeyID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, delegationInvalid("the key that minted the token is not the agent's")
	} else if err != nil {
		return nil, &client.AgentAccessLookupError{AgentID: delegated.AgentID, Err: err}
	}
	if key.RevokedAt != nil {
		return nil, delegationInvalid("the key that minted the token is revoked")
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now()) {
		return nil, delegationInvalid("the key that minted the token is expired")
	}
	return key, nil
}

// delegatedGrants returns the groups and the extra of the principal of an agent that acts on behalf of a user with
// delegated, minted with key. They are what the key grants, as for the agent's own requests with the key: its scopes'
// groups and the MCP servers it names (agentKeyGroups). A composite loopback's token instead carries the loopback's
// groups and connections, as the agent's own loopback token does, and only while the key still grants MCP access.
func delegatedGrants(delegated *persistent.DelegatedToken, key *gatewaytypes.APIKey) ([]string, map[string][]string, error) {
	keyGroups := agentKeyGroups(key.APIKeyScopes)
	extra := agentKeyExtra(delegated.AgentID, key)

	loopback := delegated.Loopback
	if loopback == nil {
		return keyGroups, extra, nil
	}

	if !slices.Contains(keyGroups, types2.GroupMCP) {
		return nil, nil, delegationInvalid("the key that minted the token no longer grants MCP access")
	}
	extra["mcp_id"] = []string{loopback.MCPID}
	extra["resource"] = []string{loopback.Audience}
	extra["oauth_client_id"] = []string{loopback.OAuthClientID}
	extra["oauth_connection_id"] = []string{loopback.OAuthConnectionID}
	extra["authorized_mcp_ids"] = append(slices.Clone(loopback.AuthorizedMCPIDs), loopback.MCPID)
	// The loopback carries the key's attribution as the request it serves recorded it.
	extra[principal.APIKeyNameExtra] = []string{loopback.APIKeyName}
	extra[principal.APIKeyRawNameExtra] = []string{loopback.APIKeyRawName}
	return slices.Clone(loopback.Groups), extra, nil
}

func delegationInvalid(reason string) error {
	return &client.AgentDelegationInvalidError{Reason: reason}
}
