package mcpgateway

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"

	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
)

// TestCompositeComponentAuthRefusesDelegatedTokens checks the composite loopback with the server's token service: a
// token with which an agent acts on behalf of a user never authenticates a component connection, and a loopback token
// is never re-minted to act on behalf of one. The agent's own loopback token still does, so the refusal is of the
// token's kind.
func TestCompositeComponentAuthRefusesDelegatedTokens(t *testing.T) {
	gatewayClient, db := newHeadlessTestGatewayClient(t)
	ctx := t.Context()

	_, jwk, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	require.NoError(t, gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: system.JWKCredentialContext,
		Name:    system.JWKCredentialContext,
		Secrets: map[string]string{"JWK_KEY": base64.StdEncoding.EncodeToString(jwk)},
	}))
	tokens, err := persistent.NewTokenService("https://obot.example", gatewayClient)
	require.NoError(t, err)

	user := gatewaytypes.User{Username: "ann", Email: "ann@example.com", Role: types.RoleBasic}
	require.NoError(t, db.Create(&user).Error)
	agent, err := gatewayClient.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "assistant"})
	require.NoError(t, err)
	key, err := gatewayClient.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "assistant", CanAccessLLMProxy: true})
	require.NoError(t, err)

	delegated, _, err := tokens.NewDelegatedToken(ctx, agent.ID, user.ID, "adl1test", key.ID, 0)
	require.NoError(t, err)
	handler, err := compositeComponentAuth{ctx: ctx, tokens: tokens}.OAuthHandler(compositeTestAuthContext(ctx, "Bearer "+delegated), mmmcpconfig.Server{})
	require.NoError(t, err)
	require.Nil(t, handler, "a component connection authenticated with a token that acts on behalf of a user")

	own := persistent.TokenContext{
		Audience:   "https://obot.example/mcp-connect-composite/vmcpi1test",
		ExpiresAt:  persistent.NewTime(time.Now().Add(compositeLoopbackTokenTTL)),
		UserID:     principal.Agent(agent.ID).String(),
		UserGroups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
		MCPID:      "vmcpi1test",
	}
	_, ownToken, err := tokens.NewToken(ctx, own)
	require.NoError(t, err)
	handler, err = compositeComponentAuth{ctx: ctx, tokens: tokens}.OAuthHandler(compositeTestAuthContext(ctx, "Bearer "+ownToken), mmmcpconfig.Server{})
	require.NoError(t, err)
	require.NotNil(t, handler, "the agent's own composite loopback token was refused")

	// A loopback token is re-minted from the claims of the one it replaces, and never with a delegation.
	actingForUser := own
	actingForUser.UserID = principal.User(user.ID).String()
	actingForUser.Actor = &persistent.TokenActor{Subject: principal.Agent(agent.ID).String()}
	actingForUser.DelegationID = "adl1test"
	_, _, err = newCompositeLoopbackToken(ctx, tokens, actingForUser, time.Now())
	require.Error(t, err)
}
