package persistent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/request/union"
)

const (
	testDelegationID = "adl1test"
)

// delegationFixture is an agent with a key, and a user with the agent's numeric ID, so that a token read as the wrong
// kind of principal would name an identity that exists.
type delegationFixture struct {
	tokens  *TokenService
	gateway *client.Client
	user    *gatewaytypes.User
	agent   *gatewaytypes.Agent
	keyID   uint
}

func newDelegationFixture(t *testing.T) delegationFixture {
	t.Helper()

	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "erin", types.RoleAdmin)
	agent := createAgentWithUsersID(t, gatewayClient, user)
	key, err := gatewayClient.CreateAgentAPIKey(t.Context(), agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "assistant", CanAccessLLMProxy: true})
	require.NoError(t, err)
	return delegationFixture{
		tokens:  tokenService,
		gateway: gatewayClient,
		user:    user,
		agent:   agent,
		keyID:   key.ID,
	}
}

func (f delegationFixture) token(t *testing.T) string {
	t.Helper()

	token, _, err := f.tokens.NewDelegatedToken(t.Context(), f.agent.ID, f.user.ID, testDelegationID, f.keyID, 0)
	require.NoError(t, err)
	return token
}

// delegatedClaims are the claims NewDelegatedToken issues for the fixture, for tests that sign variants of them.
func (f delegationFixture) delegatedClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"aud":                 testServerURL,
		"sub":                 principal.User(f.user.ID).String(),
		"obot_principal_kind": string(principal.KindUser),
		"act":                 map[string]any{"sub": principal.Agent(f.agent.ID).String()},
		"obot_delegation_id":  testDelegationID,
		"api_key_id":          fmt.Sprint(f.keyID),
	}
}

func TestDelegatedTokenRoundTrip(t *testing.T) {
	f := newDelegationFixture(t)

	before := time.Now()
	token, expiresAt, err := f.tokens.NewDelegatedToken(t.Context(), f.agent.ID, f.user.ID, testDelegationID, f.keyID, 0)
	require.NoError(t, err)
	// The lifetime defaults to 15 minutes.
	assert.WithinDuration(t, before.Add(DelegatedTokenTTL), expiresAt, 2*time.Second)

	// The user is the subject, recorded as a user, and the agent is the actor. The token grants no group and names
	// no one by name or email.
	claims, parsed := unverifiedClaims(token)
	require.True(t, parsed)
	assert.Equal(t, principal.User(f.user.ID).String(), claims["sub"])
	assert.Equal(t, string(principal.KindUser), claims["obot_principal_kind"])
	assert.Equal(t, map[string]any{"sub": principal.Agent(f.agent.ID).String()}, claims["act"])
	assert.Equal(t, testDelegationID, claims["obot_delegation_id"])
	assert.Equal(t, fmt.Sprint(f.keyID), claims["api_key_id"])
	assert.Equal(t, testServerURL, claims["aud"])
	assert.Empty(t, claims["UserGroups"])
	assert.Empty(t, claims["name"])
	assert.Empty(t, claims["email"])

	decoded, err := f.tokens.DecodeDelegatedToken(t.Context(), token)
	require.NoError(t, err)
	assert.Equal(t, &DelegatedToken{
		AgentID:      f.agent.ID,
		UserID:       f.user.ID,
		DelegationID: testDelegationID,
		APIKeyID:     f.keyID,
		ExpiresAt:    expiresAt,
	}, decoded)

	// A shorter lifetime may be asked for, and a longer one is refused.
	_, expiresAt, err = f.tokens.NewDelegatedToken(t.Context(), f.agent.ID, f.user.ID, testDelegationID, f.keyID, time.Minute)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(time.Minute), expiresAt, 2*time.Second)
	_, _, err = f.tokens.NewDelegatedToken(t.Context(), f.agent.ID, f.user.ID, testDelegationID, f.keyID, DelegatedTokenTTL+time.Second)
	require.Error(t, err)
}

// TestDelegatedTokensAreRefusedByEveryOtherDecoder checks that a token with which an agent acts on behalf of a user,
// whose subject is the user, is never read as the user's own, or as anyone's, by any decoder or authenticator but
// DecodeDelegatedToken: it is refused, valid or not, in either header the token service reads.
func TestDelegatedTokensAreRefusedByEveryOtherDecoder(t *testing.T) {
	f := newDelegationFixture(t)
	otherService, _ := newLifecycleTestTokenService(t)

	// only returns the claims of a user's token, with those of the fixture's delegated token that keep names.
	only := func(keep ...string) jwt.MapClaims {
		claims := f.delegatedClaims()
		for key := range claims {
			if !slices.Contains(append(keep, "aud", "sub", "obot_principal_kind"), key) {
				delete(claims, key)
			}
		}
		return claims
	}
	with := func(claims jwt.MapClaims, key string, value any) jwt.MapClaims {
		claims[key] = value
		return claims
	}

	for _, tt := range []struct {
		name  string
		token string
		// valid is set for a token that only fails because it acts on behalf of a user.
		valid bool
	}{
		{
			name:  "minted",
			token: f.token(t),
			valid: true,
		},
		{
			name:  "with an actor only",
			token: signedToken(t, f.tokens, only("act")),
			valid: true,
		},
		{
			name:  "with a delegation only",
			token: signedToken(t, f.tokens, only("obot_delegation_id")),
			valid: true,
		},
		{
			name:  "with a null actor",
			token: signedToken(t, f.tokens, with(only(), "act", nil)),
			valid: true,
		},
		{
			name:  "with an actor that names the user",
			token: signedToken(t, f.tokens, with(only(), "act", map[string]any{"sub": principal.User(f.user.ID).String()})),
			valid: true,
		},
		{
			name:  "with the user's groups",
			token: signedToken(t, f.tokens, with(f.delegatedClaims(), "UserGroups", "admin,authenticated")),
			valid: true,
		},
		{
			name:  "with an agent's subject",
			token: signedToken(t, f.tokens, with(with(f.delegatedClaims(), "sub", principal.Agent(f.agent.ID).String()), "obot_principal_kind", "agent")),
			valid: true,
		},
		{
			name:  "expired",
			token: signedToken(t, f.tokens, with(f.delegatedClaims(), "exp", time.Now().Add(-time.Minute).Unix())),
		},
		{
			name:  "signed with another key",
			token: signedToken(t, otherService, f.delegatedClaims()),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The shared decode step.
			decoded, err := f.tokens.DecodeToken(t.Context(), tt.token)
			require.Error(t, err)
			require.Nil(t, decoded)
			if tt.valid {
				require.ErrorIs(t, err, ErrDelegatedToken)
			}

			assert.True(t, ClaimsDelegatedToken(tt.token))
			// It is an agent's credential: a refused one is never sent into a user's login flow.
			assert.True(t, ClaimsAgentToken(tt.token))

			for _, header := range []string{"Authorization", "X-API-Key"} {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testServerURL, nil)
				if header == "Authorization" {
					req.Header.Set(header, "Bearer "+tt.token)
				} else {
					req.Header.Set(header, tt.token)
				}

				// The token service's authenticator refuses it rather than declining it.
				response, ok, err := f.tokens.AuthenticateRequest(req)
				require.ErrorIs(t, err, ErrDelegatedToken, header)
				require.False(t, ok)
				require.Nil(t, response)

				// The guard of a server that does not authenticate callers refuses it, alone and after the token
				// service.
				response, ok, err = f.tokens.AgentTokens().AuthenticateRequest(req)
				require.Error(t, err, header)
				require.False(t, ok)
				require.Nil(t, response)
				response, ok, err = union.New(f.tokens, union.NewFailOnError(f.tokens.AgentTokens())).AuthenticateRequest(req)
				require.Error(t, err, header)
				require.False(t, ok)
				require.Nil(t, response)
			}
		})
	}
}

// TestDecodeDelegatedTokenReadsNoOtherToken checks that the one decoder that accepts tokens acting on behalf of a user
// accepts nothing else: not a user's or an agent's own token, and not a token that acts on behalf of a user that
// NewDelegatedToken would not issue.
func TestDecodeDelegatedTokenReadsNoOtherToken(t *testing.T) {
	f := newDelegationFixture(t)

	_, userToken, err := f.tokens.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL,
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     principal.User(f.user.ID).String(),
		UserGroups: []string{types.GroupAuthenticated},
		APIKeyID:   fmt.Sprint(f.keyID),
	})
	require.NoError(t, err)
	_, agentToken, err := f.tokens.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL,
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     principal.Agent(f.agent.ID).String(),
		UserGroups: agentLoopbackGroups,
	})
	require.NoError(t, err)

	variant := func(change func(jwt.MapClaims)) string {
		claims := f.delegatedClaims()
		change(claims)
		return signedToken(t, f.tokens, claims)
	}
	otherService, _ := newLifecycleTestTokenService(t)

	for _, tt := range []struct {
		name  string
		token string
	}{
		{
			name:  "a user's token",
			token: userToken,
		},
		{
			name:  "an agent's token",
			token: agentToken,
		},
		{
			name:  "not a token",
			token: "oak1-1-2-secret",
		},
		{
			name:  "expired",
			token: variant(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }),
		},
		{
			name:  "without an expiry",
			token: unexpiringToken(t, f.tokens, f.delegatedClaims()),
		},
		{
			name:  "signed with another key",
			token: signedToken(t, otherService, f.delegatedClaims()),
		},
		{
			name:  "without an actor",
			token: variant(func(c jwt.MapClaims) { delete(c, "act") }),
		},
		{
			name:  "with an actor that is not an agent",
			token: variant(func(c jwt.MapClaims) { c["act"] = map[string]any{"sub": principal.User(f.user.ID).String()} }),
		},
		{
			name:  "with an actor that is not an object",
			token: variant(func(c jwt.MapClaims) { c["act"] = principal.Agent(f.agent.ID).String() }),
		},
		{
			name: "acting for an agent",
			token: variant(func(c jwt.MapClaims) {
				c["sub"] = principal.Agent(f.agent.ID).String()
				c["obot_principal_kind"] = string(principal.KindAgent)
			}),
		},
		{
			name:  "acting for a subject without a kind",
			token: variant(func(c jwt.MapClaims) { delete(c, "obot_principal_kind") }),
		},
		{
			name:  "acting for something that is not a principal",
			token: variant(func(c jwt.MapClaims) { c["sub"] = "system" }),
		},
		{
			name: "acting for the owner of a user's setup through the agent",
			token: variant(func(c jwt.MapClaims) {
				c["sub"] = principal.DelegatedOwner{AgentID: f.agent.ID, UserID: f.user.ID}.String()
			}),
		},
		{
			name:  "without a delegation",
			token: variant(func(c jwt.MapClaims) { delete(c, "obot_delegation_id") }),
		},
		{
			name:  "without the minting key",
			token: variant(func(c jwt.MapClaims) { delete(c, "api_key_id") }),
		},
		{
			name:  "with a malformed minting key",
			token: variant(func(c jwt.MapClaims) { c["api_key_id"] = "key" }),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := f.tokens.DecodeDelegatedToken(t.Context(), tt.token)
			require.Error(t, err)
			require.Nil(t, decoded)
		})
	}
}

// unexpiringToken signs claims as the token service does, with no expiry, which signedToken always adds.
func unexpiringToken(t *testing.T, tokenService *TokenService, claims jwt.MapClaims) string {
	t.Helper()

	claims["iss"] = testServerURL
	claims["iat"] = time.Now().Add(-time.Minute).Unix()
	token, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(tokenService.privateKey)
	require.NoError(t, err)
	return token
}

// TestNewTokenNeverIssuesADelegatedToken checks that only NewDelegatedToken issues a token that acts on behalf of a
// user, and that no token is issued for the owner of a user's setup through an agent.
func TestNewTokenNeverIssuesADelegatedToken(t *testing.T) {
	f := newDelegationFixture(t)

	for _, tt := range []struct {
		name    string
		context TokenContext
	}{
		{
			name: "with an actor",
			context: TokenContext{
				Audience: testServerURL,
				UserID:   principal.User(f.user.ID).String(),
				Actor:    &TokenActor{Subject: principal.Agent(f.agent.ID).String()},
			},
		},
		{
			name: "with an empty actor",
			context: TokenContext{
				Audience: testServerURL,
				UserID:   principal.User(f.user.ID).String(),
				Actor:    &TokenActor{},
			},
		},
		{
			name: "with a delegation",
			context: TokenContext{
				Audience:     testServerURL,
				UserID:       principal.User(f.user.ID).String(),
				DelegationID: testDelegationID,
			},
		},
		{
			name: "for the owner of a user's setup through an agent",
			context: TokenContext{
				Audience:   testServerURL,
				UserID:     principal.DelegatedOwner{AgentID: f.agent.ID, UserID: f.user.ID}.String(),
				UserGroups: agentLoopbackGroups,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			token, signed, err := f.tokens.NewToken(t.Context(), tt.context)
			require.Error(t, err)
			require.Nil(t, token)
			require.Empty(t, signed)
		})
	}
}

// TestNewDelegatedTokenRefuses checks that a token that acts on behalf of a user is issued only for an active agent
// and an active user, with a key of that agent that is neither revoked nor expired.
func TestNewDelegatedTokenRefuses(t *testing.T) {
	f := newDelegationFixture(t)
	ctx := t.Context()

	otherAgent := createTestAgent(t, f.gateway)
	otherKey, err := f.gateway.CreateAgentAPIKey(ctx, otherAgent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "other", CanAccessLLMProxy: true})
	require.NoError(t, err)
	userKey, err := f.gateway.CreateAPIKey(ctx, f.user.ID, "erin", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	require.NoError(t, err)
	revokedKey, err := f.gateway.CreateAgentAPIKey(ctx, f.agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "revoked", CanAccessLLMProxy: true})
	require.NoError(t, err)
	require.NoError(t, f.gateway.RevokeAgentAPIKey(ctx, f.agent.ID, revokedKey.ID))
	expiredKey, err := f.gateway.CreateAgentAPIKey(ctx, f.agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "expired", CanAccessLLMProxy: true, ExpiresAt: new(time.Now().Add(-time.Minute))})
	require.NoError(t, err)

	disabledAgent := createTestAgent(t, f.gateway)
	disabledAgentKey, err := f.gateway.CreateAgentAPIKey(ctx, disabledAgent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "ci", CanAccessLLMProxy: true})
	require.NoError(t, err)
	_, err = f.gateway.DisableAgent(ctx, disabledAgent.ID, "compromised")
	require.NoError(t, err)

	disabledUser := createLifecycleTestUser(t, f.gateway, "fay", types.RoleBasic)
	require.NoError(t, deactivateThroughSCIM(t, f.gateway, lifecycleTestProvider, "00u-"+disabledUser.Username, disabledUser.Email))

	for _, tt := range []struct {
		name         string
		agentID      uint
		userID       uint
		delegationID string
		keyID        uint
		check        func(t *testing.T, err error)
	}{
		{
			name:         "without an agent",
			userID:       f.user.ID,
			delegationID: testDelegationID,
			keyID:        f.keyID,
		},
		{
			name:         "without a user",
			agentID:      f.agent.ID,
			delegationID: testDelegationID,
			keyID:        f.keyID,
		},
		{
			name:    "without a delegation",
			agentID: f.agent.ID,
			userID:  f.user.ID,
			keyID:   f.keyID,
		},
		{
			name:         "without a key",
			agentID:      f.agent.ID,
			userID:       f.user.ID,
			delegationID: testDelegationID,
		},
		{
			name:         "for a disabled agent",
			agentID:      disabledAgent.ID,
			userID:       f.user.ID,
			delegationID: testDelegationID,
			keyID:        disabledAgentKey.ID,
			check: func(t *testing.T, err error) {
				t.Helper()
				denied, ok := errors.AsType[*client.AgentAccessDeniedError](err)
				require.True(t, ok, "error = %v, want a denial", err)
				require.Equal(t, types.UserStatusDisabled, denied.Status)
			},
		},
		{
			name:         "for a missing agent",
			agentID:      otherAgent.ID + 100,
			userID:       f.user.ID,
			delegationID: testDelegationID,
			keyID:        f.keyID,
		},
		{
			name:         "for a disabled user",
			agentID:      f.agent.ID,
			userID:       disabledUser.ID,
			delegationID: testDelegationID,
			keyID:        f.keyID,
			check: func(t *testing.T, err error) {
				t.Helper()
				denied, ok := errors.AsType[*client.UserAccessDeniedError](err)
				require.True(t, ok, "error = %v, want a denial", err)
				require.Equal(t, types.UserStatusDisabled, denied.Status)
			},
		},
		{
			name:         "for a missing user",
			agentID:      f.agent.ID,
			userID:       disabledUser.ID + 100,
			delegationID: testDelegationID,
			keyID:        f.keyID,
			check: func(t *testing.T, err error) {
				t.Helper()
				denied, ok := errors.AsType[*client.UserAccessDeniedError](err)
				require.True(t, ok, "error = %v, want a denial", err)
				require.Equal(t, types.UserStatusDeleted, denied.Status)
			},
		},
		{
			name:         "with another agent's key",
			agentID:      f.agent.ID,
			userID:       f.user.ID,
			delegationID: testDelegationID,
			keyID:        otherKey.ID,
		},
		{
			name:         "with the user's key",
			agentID:      f.agent.ID,
			userID:       f.user.ID,
			delegationID: testDelegationID,
			keyID:        userKey.ID,
		},
		{
			name:         "with a revoked key",
			agentID:      f.agent.ID,
			userID:       f.user.ID,
			delegationID: testDelegationID,
			keyID:        revokedKey.ID,
		},
		{
			name:         "with an expired key",
			agentID:      f.agent.ID,
			userID:       f.user.ID,
			delegationID: testDelegationID,
			keyID:        expiredKey.ID,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			token, expiresAt, err := f.tokens.NewDelegatedToken(ctx, tt.agentID, tt.userID, tt.delegationID, tt.keyID, 0)
			require.Error(t, err)
			require.Empty(t, token)
			require.True(t, expiresAt.IsZero())
			if tt.check != nil {
				tt.check(t, err)
			}
		})
	}

	// Without the gateway database, nothing can be checked, and no token is issued.
	noGateway := newTestTokenService(t)
	_, _, err = noGateway.NewDelegatedToken(ctx, f.agent.ID, f.user.ID, testDelegationID, f.keyID, 0)
	require.Error(t, err)
}

// TestDelegatedTokenClaimsAreTheTokenContexts checks that the claims NewDelegatedToken issues are the JSON names of
// TokenContext's fields, so that a TokenContext decoded from such a token, as DecodeDelegatedToken does, carries them.
func TestDelegatedTokenClaimsAreTheTokenContexts(t *testing.T) {
	data, err := json.Marshal(TokenContext{
		Actor:        &TokenActor{Subject: "agent:5"},
		DelegationID: testDelegationID,
	})
	require.NoError(t, err)
	var claims map[string]any
	require.NoError(t, json.Unmarshal(data, &claims))
	assert.Equal(t, map[string]any{"sub": "agent:5"}, claims[actorClaim])
	assert.Equal(t, testDelegationID, claims[delegationIDClaim])
}
