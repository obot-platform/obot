package server

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/api/authn"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/require"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	delegatedTestGroup = "okta/eng"
	delegatedTestMCP   = "/mcp-connect/vmcp1test"
)

// delegatedFixture is an agent whose profiles let it act for the users in delegatedTestGroup, a user in that group
// who is an administrator, the agent's key, and the user's delegation of the agent.
type delegatedFixture struct {
	s          *Server
	client     *gatewayclient.Client
	tokens     *persistent.TokenService
	auth       *DelegatedTokenAuthenticator
	user       *gatewaytypes.User
	agent      *gatewaytypes.Agent
	key        *gatewaytypes.APIKeyCreateResponse
	delegation *gatewaytypes.AgentDelegation
}

func newDelegatedFixture(t *testing.T, scopes gatewaytypes.AgentAPIKeyCreateRequest) delegatedFixture {
	t.Helper()
	ctx := t.Context()

	s, client := newTokenRequestTestServer(t)
	tokens := newChainTokenService(t, client)

	user, err := client.EnsureIdentityWithRole(ctx, &gatewaytypes.Identity{
		AuthProviderName:      apiKeyLifecycleTestProvider.Name,
		AuthProviderNamespace: apiKeyLifecycleTestProvider.Namespace,
		ProviderUsername:      "ann",
		ProviderUserID:        "00u-ann",
		Email:                 "ann@example.com",
	}, "", types2.RoleAdmin, gatewayclient.UserLimit{Unlimited: true})
	require.NoError(t, err)
	require.NoError(t, s.db.WithContext(ctx).Create(&gatewaytypes.Group{
		ID:                    delegatedTestGroup,
		Name:                  "eng",
		AuthProviderName:      apiKeyLifecycleTestProvider.Name,
		AuthProviderNamespace: apiKeyLifecycleTestProvider.Namespace,
	}).Error)
	require.NoError(t, s.db.WithContext(ctx).Create(&gatewaytypes.GroupMemberships{UserID: user.ID, GroupID: delegatedTestGroup}).Error)

	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{
		DisplayName: "assistant",
		Profiles: []types2.AgentProfile{{
			Name:     "engineering",
			Subjects: []types2.Subject{{Type: types2.SubjectTypeGroup, ID: delegatedTestGroup}},
		}},
		AllowedReturnHosts: []string{"agent.example.com"},
	})
	require.NoError(t, err)
	if scopes.Name == "" {
		scopes.Name = "web"
	}
	key, err := client.CreateAgentAPIKey(ctx, agent.ID, scopes)
	require.NoError(t, err)

	request, err := client.CreateAgentDelegationRequest(ctx, gatewayclient.AgentDelegationRequestInput{
		AgentID:    agent.ID,
		APIKeyID:   key.ID,
		ReturnURL:  "https://agent.example.com/callback",
		SecretHash: secretHash("the session's secret"),
	})
	require.NoError(t, err)
	approval, _, err := client.ApproveAgentDelegationRequest(ctx, request.ID, subjects.Caller{
		Principal: principal.User(user.ID),
		Groups:    subjects.Groups{AuthProvider: map[string]struct{}{delegatedTestGroup: {}}},
	})
	require.NoError(t, err)

	return delegatedFixture{
		s:          s,
		client:     client,
		tokens:     tokens,
		auth:       NewDelegatedTokenAuthenticator(client, tokens),
		user:       user,
		agent:      agent,
		key:        key,
		delegation: approval.Delegation,
	}
}

// secretHash returns the S256 hash an agent sends for secret.
func secretHash(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (f delegatedFixture) token(t *testing.T) string {
	t.Helper()
	token, _, err := f.tokens.NewDelegatedToken(t.Context(), f.agent.ID, f.user.ID, f.delegation.ID, f.key.ID, 0)
	require.NoError(t, err)
	return token
}

// authenticate runs the authenticator on a GET request to path that presents token.
func (f delegatedFixture) authenticate(t *testing.T, path, token string) (user.Info, bool, error) {
	t.Helper()
	req := noAuthTestRequest(t, path, map[string]string{"Authorization": "Bearer " + token})
	// Every route that accepts the token accepts GET.
	req.Method = http.MethodGet
	response, ok, err := f.auth.AuthenticateRequest(req)
	if response != nil {
		return response.User, ok, err
	}
	return nil, ok, err
}

// requireDelegationInvalid fails unless err refuses a delegated token, as the API server finds the refusal in the
// errors of the authenticator chain, whose unions aggregate their members' errors without wrapping them.
func requireDelegationInvalid(t *testing.T, err error) {
	t.Helper()
	require.True(t, delegationInvalidIn(err), "error = %v, want the delegation refused", err)
}

func delegationInvalidIn(err error) bool {
	if invalid, ok := errors.AsType[*gatewayclient.AgentDelegationInvalidError](err); ok {
		return strings.HasPrefix(invalid.Error(), gatewayclient.AgentDelegationInvalidCode)
	}
	if aggregate, ok := errors.AsType[utilerrors.Aggregate](err); ok {
		return slices.ContainsFunc(aggregate.Errors(), delegationInvalidIn)
	}
	return false
}

// TestDelegatedTokenPrincipal checks the principal of an agent acting on behalf of a user: the agent's, with what the
// minting key grants, and the user recorded only as the user it acts for, with the user's groups beside it for
// matching. It never has the user's role or groups.
func TestDelegatedTokenPrincipal(t *testing.T) {
	f := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}})

	info, ok, err := f.authenticate(t, delegatedTestMCP, f.token(t))
	require.NoError(t, err)
	require.True(t, ok)

	agent := principal.Agent(f.agent.ID).String()
	require.Equal(t, agent, info.GetUID())
	require.Equal(t, agent, info.GetName())
	require.ElementsMatch(t, agentKeyGroups(gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}}), info.GetGroups())
	require.NotContains(t, info.GetGroups(), types2.GroupAdmin)
	require.Equal(t, []string{"*"}, info.GetExtra()["authorized_mcp_ids"])
	require.Empty(t, info.GetExtra()["obot_groups"])
	require.Empty(t, info.GetExtra()["auth_provider_groups"])
	require.Empty(t, info.GetExtra()["email"])
	attribution, ok := principal.APIKeyAttributionFromUser(info)
	require.True(t, ok)
	require.Equal(t, f.key.ID, attribution.ID)

	onBehalfOf, ok := principal.OnBehalfOf(info)
	require.True(t, ok)
	require.Equal(t, principal.User(f.user.ID).ID, onBehalfOf)
	status, recorded := principal.UserStatus(info)
	require.True(t, recorded)
	require.Equal(t, types2.UserStatusActive, status)

	// The caller has no groups, whatever the user's are; the user's are beside it, for matching.
	caller, ok := subjects.CallerFromUserInfo(info)
	require.True(t, ok)
	require.Empty(t, caller.Groups.AuthProvider)
	require.Empty(t, caller.Groups.Obot)
	delegation, ok := agentprofile.DelegationOf(info)
	require.True(t, ok)
	require.Equal(t, f.delegation.ID, delegation.ID)
	require.Equal(t, principal.User(f.user.ID), delegation.User.Principal)
	require.Contains(t, delegation.User.Groups.AuthProvider, delegatedTestGroup)
	require.Equal(t, "ann@example.com", delegation.Email)

	// The admission check admits it.
	response, ok, err := authn.NewAdmissionCheck(f.auth, nil).AuthenticateRequest(noAuthTestRequest(t, delegatedTestMCP, map[string]string{"Authorization": "Bearer " + f.token(t)}))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, agent, response.User.GetUID())
}

// TestDelegatedTokenRoutes checks that the authenticator opts in to tokens that act on behalf of a user only on the
// routes that accept them, and only to those tokens.
func TestDelegatedTokenRoutes(t *testing.T) {
	f := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true, CanAccessSkills: true, MCPServerIDs: []string{"*"}})
	token := f.token(t)

	for _, path := range []string{noAuthTestLLMPath, delegatedTestMCP, "/mcp-connect-composite/vmcpi1test", "/api/skills"} {
		info, ok, err := f.authenticate(t, path, token)
		require.NoError(t, err, path)
		require.True(t, ok, path)
		require.Equal(t, principal.Agent(f.agent.ID).String(), info.GetUID())
	}
	for _, path := range []string{"/api/me", "/api/agent-delegations/token", "/api/api-keys/auth", "/api/vmcps"} {
		_, ok, err := f.authenticate(t, path, token)
		require.NoError(t, err, path)
		require.False(t, ok, "%s: declined, for the token service to refuse", path)
	}

	// Any other credential is left to the rest of the chain.
	_, ok, err := f.authenticate(t, delegatedTestMCP, f.key.Key)
	require.NoError(t, err)
	require.False(t, ok)
	_, agentToken, err := f.tokens.NewToken(t.Context(), persistent.TokenContext{
		Audience:   "https://obot.example.com/mcp-connect-composite/vmcpi1test",
		ExpiresAt:  persistent.NewTime(time.Now().Add(time.Minute)),
		UserID:     principal.Agent(f.agent.ID).String(),
		UserGroups: []string{types2.GroupMCP, types2.GroupCompositeMCP, types2.GroupAuthenticated},
		MCPID:      "vmcpi1test",
	})
	require.NoError(t, err)
	_, ok, err = f.authenticate(t, delegatedTestMCP, agentToken)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestDelegatedTokenRefusals checks every request against the delegation, the agent, the user, the agent profiles that
// matches them and the minting key, and that each failure refuses the token rather than declining it.
func TestDelegatedTokenRefusals(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, f delegatedFixture)
	}{
		{
			name: "the delegation is revoked",
			change: func(t *testing.T, f delegatedFixture) {
				t.Helper()
				_, err := f.client.RevokeAgentDelegation(t.Context(), f.delegation.ID, principal.User(f.user.ID).String())
				require.NoError(t, err)
			},
		},
		{
			name: "the user is disabled",
			change: func(t *testing.T, f delegatedFixture) {
				t.Helper()
				// Written directly, as another replica would, so that only the live read of the user refuses it.
				require.NoError(t, f.s.db.WithContext(t.Context()).Model(&gatewaytypes.User{}).Where("id = ?", f.user.ID).Update("disabled_at", time.Now()).Error)
			},
		},
		{
			name: "the agent is disabled",
			change: func(t *testing.T, f delegatedFixture) {
				t.Helper()
				_, err := f.client.DisableAgent(t.Context(), f.agent.ID, "")
				require.NoError(t, err)
			},
		},
		{
			name: "the user left the group the profile names",
			change: func(t *testing.T, f delegatedFixture) {
				t.Helper()
				require.NoError(t, f.s.db.WithContext(t.Context()).Where("user_id = ?", f.user.ID).Delete(&gatewaytypes.GroupMemberships{}).Error)
			},
		},
		{
			name: "no profile names the user any more",
			change: func(t *testing.T, f delegatedFixture) {
				t.Helper()
				agent := *f.agent
				agent.Profiles = []types2.AgentProfile{{
					ID:       f.agent.Profiles[0].ID,
					Name:     "sales",
					Subjects: []types2.Subject{{Type: types2.SubjectTypeGroup, ID: "okta/sales"}},
				}}
				_, err := f.client.UpdateAgent(t.Context(), agent)
				require.NoError(t, err)
			},
		},
		{
			name: "the minting key is revoked",
			change: func(t *testing.T, f delegatedFixture) {
				t.Helper()
				require.NoError(t, f.client.RevokeAgentAPIKey(t.Context(), f.agent.ID, f.key.ID))
			},
		},
		{
			name: "the minting key is expired",
			change: func(t *testing.T, f delegatedFixture) {
				t.Helper()
				require.NoError(t, f.s.db.WithContext(t.Context()).Model(&gatewaytypes.APIKey{}).Where("id = ?", f.key.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}})
			token := f.token(t)
			_, ok, err := f.authenticate(t, delegatedTestMCP, token)
			require.NoError(t, err)
			require.True(t, ok)

			tt.change(t, f)
			info, ok, err := f.authenticate(t, delegatedTestMCP, token)
			requireDelegationInvalid(t, err)
			require.False(t, ok)
			require.Nil(t, info)

			// The chains, with and without authentication, refuse it too, and never take it as anyone.
			for _, chain := range []*authn.AdmissionCheck{noAuthChainWith(f.s, f.client, f.tokens), agentChainWith(f.client, f.tokens)} {
				response, ok, err := chain.AuthenticateRequest(noAuthTestRequest(t, delegatedTestMCP, map[string]string{"Authorization": "Bearer " + token}))
				requireDelegationInvalid(t, err)
				require.False(t, ok)
				require.Nil(t, response)
			}
		})
	}
}

// TestDelegatedTokensWithAnotherTokenService checks that a token another token service signed, or one that is
// malformed, is refused rather than declined.
func TestDelegatedTokensWithAnotherTokenService(t *testing.T) {
	f := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	other := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})

	_, ok, err := f.authenticate(t, noAuthTestLLMPath, other.token(t))
	requireDelegationInvalid(t, err)
	require.False(t, ok)
}

// TestDelegatedLoopbackTokens checks the tokens of a composite vMCP's loopback for an agent acting on behalf of a user:
// the principal has the loopback's groups and connections, acts on behalf of the same user, and is refused when the
// key no longer grants MCP access.
func TestDelegatedLoopbackTokens(t *testing.T) {
	f := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}})
	delegation := persistent.DelegatedToken{AgentID: f.agent.ID, UserID: f.user.ID, DelegationID: f.delegation.ID, APIKeyID: f.key.ID}
	loopback := persistent.DelegatedLoopback{
		Audience:          "https://obot.example.com/mcp-connect-composite/vmcpi1test",
		Groups:            []string{types2.GroupMCP, types2.GroupCompositeMCP, types2.GroupAuthenticated},
		MCPID:             "vmcpi1test",
		AuthorizedMCPIDs:  []string{"msi1component"},
		OAuthConnectionID: "vmcp1test",
		APIKeyName:        "web",
		APIKeyRawName:     "web",
	}
	token, _, err := f.tokens.NewDelegatedLoopbackToken(t.Context(), delegation, loopback, 0)
	require.NoError(t, err)

	info, ok, err := f.authenticate(t, "/mcp-connect/msi1component", token)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, principal.Agent(f.agent.ID).String(), info.GetUID())
	require.ElementsMatch(t, loopback.Groups, info.GetGroups())
	require.True(t, principal.IsComponentHop(info))
	require.Equal(t, []string{"msi1component", "vmcpi1test"}, info.GetExtra()["authorized_mcp_ids"])
	require.Equal(t, []string{"vmcp1test"}, info.GetExtra()["oauth_connection_id"])
	onBehalfOf, ok := principal.OnBehalfOf(info)
	require.True(t, ok)
	require.Equal(t, principal.User(f.user.ID).ID, onBehalfOf)
	d, ok := agentprofile.DelegationOf(info)
	require.True(t, ok)
	require.Equal(t, f.delegation.ID, d.ID)

	// Only on the MCP routes.
	_, ok, err = f.authenticate(t, "/api/me", token)
	require.NoError(t, err)
	require.False(t, ok)

	// A key that does not grant MCP access does not authenticate a loopback.
	noMCP := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	token, _, err = noMCP.tokens.NewDelegatedLoopbackToken(t.Context(), persistent.DelegatedToken{
		AgentID:      noMCP.agent.ID,
		UserID:       noMCP.user.ID,
		DelegationID: noMCP.delegation.ID,
		APIKeyID:     noMCP.key.ID,
	}, loopback, 0)
	require.NoError(t, err)
	_, ok, err = noMCP.authenticate(t, "/mcp-connect/msi1component", token)
	requireDelegationInvalid(t, err)
	require.False(t, ok)
}

// TestChainsAcceptDelegatedTokensOnlyOnTheirRoutes checks, across the authenticator chains as the server composes
// them, with and without authentication, that a token with which an agent acts on behalf of a user authenticates the
// agent acting on behalf of the user on the routes that accept it, and on every other route, no one: not the user it
// names as its subject, not the agent, not the anonymous caller and not the "nobody" owner.
func TestChainsAcceptDelegatedTokensOnlyOnTheirRoutes(t *testing.T) {
	f := newDelegatedFixture(t, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true, CanAccessSkills: true, MCPServerIDs: []string{"*"}})
	token := f.token(t)
	for _, chain := range []struct {
		name  string
		chain *authn.AdmissionCheck
	}{
		{
			name:  "without authentication",
			chain: noAuthChainWith(f.s, f.client, f.tokens),
		},
		{
			name:  "with authentication",
			chain: agentChainWith(f.client, f.tokens),
		},
	} {
		for _, header := range []string{"Authorization", "X-API-Key"} {
			value := token
			if header == "Authorization" {
				value = "Bearer " + token
			}
			for _, path := range []string{noAuthTestLLMPath, delegatedTestMCP, "/api/skills"} {
				t.Run(chain.name+" "+header+" accepted on "+path, func(t *testing.T) {
					req := noAuthTestRequest(t, path, map[string]string{header: value})
					req.Method = http.MethodGet
					response, ok, err := chain.chain.AuthenticateRequest(req)
					require.NoError(t, err)
					require.True(t, ok)
					require.Equal(t, principal.Agent(f.agent.ID).String(), response.User.GetUID())
					_, onBehalfOf := principal.OnBehalfOf(response.User)
					require.True(t, onBehalfOf)
				})
			}
			for _, path := range []string{"/api/me", "/api/agent-delegations/token", "/api/vmcps"} {
				t.Run(chain.name+" "+header+" refused on "+path, func(t *testing.T) {
					req := noAuthTestRequest(t, path, map[string]string{header: value})
					req.Method = http.MethodGet
					response, ok, err := chain.chain.AuthenticateRequest(req)
					require.Error(t, err)
					require.False(t, ok)
					require.Nil(t, response)
					if chain.name == "with authentication" {
						require.ErrorIs(t, err, persistent.ErrDelegatedToken)
					}
				})
			}
		}
	}
}
