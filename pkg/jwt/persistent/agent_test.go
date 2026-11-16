package persistent

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
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
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

var (
	agentLoopbackGroups = []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}
)

func createTestAgent(t *testing.T, gatewayClient *client.Client) *gatewaytypes.Agent {
	t.Helper()

	agent, err := gatewayClient.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "nightly build"})
	require.NoError(t, err)
	return agent
}

// createAgentWithUsersID creates an agent with the same numeric ID as user.
func createAgentWithUsersID(t *testing.T, gatewayClient *client.Client, user *gatewaytypes.User) *gatewaytypes.Agent {
	t.Helper()

	agent := createTestAgent(t, gatewayClient)
	for agent.ID < user.ID {
		agent = createTestAgent(t, gatewayClient)
	}
	require.Equal(t, user.ID, agent.ID, "agent ID")
	return agent
}

// signedToken signs claims as the token service does, without NewToken's checks, as a token issued by an older
// server or with claims NewToken would refuse.
func signedToken(t *testing.T, tokenService *TokenService, claims jwt.MapClaims) string {
	t.Helper()

	now := time.Now()
	base := jwt.MapClaims{
		"iss": testServerURL,
		"aud": testServerURL + "/mcp-connect/server-id",
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}
	maps.Copy(base, claims)
	token, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, base).SignedString(tokenService.privateKey)
	require.NoError(t, err)
	return token
}

func bearerRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, testServerURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestNewTokenRecordsThePrincipalKind(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "ada", types.RoleBasic)
	agent := createAgentWithUsersID(t, gatewayClient, user)

	for _, tt := range []struct {
		subject string
		want    string
	}{
		{
			subject: principal.User(user.ID).String(),
			want:    string(principal.KindUser),
		},
		{
			subject: principal.Agent(agent.ID).String(),
			want:    string(principal.KindAgent),
		},
		{
			// A subject that is not a principal is issued a token with no kind, as before.
			subject: "non-numeric-user",
		},
	} {
		t.Run(tt.subject, func(t *testing.T) {
			_, token, err := tokenService.NewToken(t.Context(), TokenContext{
				Audience:   testServerURL,
				ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
				UserID:     tt.subject,
				UserGroups: agentLoopbackGroups,
			})
			require.NoError(t, err)
			decoded, err := tokenService.DecodeToken(t.Context(), token)
			require.NoError(t, err)
			assert.Equal(t, tt.subject, decoded.UserID)
			assert.Equal(t, tt.want, decoded.PrincipalKind)
		})
	}
}

func TestNewTokenRefusesAKindThatDisagreesWithTheSubject(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "bea", types.RoleBasic)
	agent := createAgentWithUsersID(t, gatewayClient, user)

	for _, tt := range []struct {
		subject string
		kind    principal.Kind
	}{
		{
			subject: principal.User(user.ID).String(),
			kind:    principal.KindAgent,
		},
		{
			subject: principal.Agent(agent.ID).String(),
			kind:    principal.KindUser,
		},
		{
			subject: "system",
			kind:    principal.KindUser,
		},
	} {
		t.Run(tt.subject+" as "+string(tt.kind), func(t *testing.T) {
			_, _, err := tokenService.NewToken(t.Context(), TokenContext{
				Audience:      testServerURL,
				UserID:        tt.subject,
				PrincipalKind: string(tt.kind),
			})
			require.Error(t, err)
		})
	}
}

// TestAuthenticateRequestNeverReadsATokenAsAnotherKind checks that a token authenticates only as the kind of
// principal it records, and that a token from before the kind was recorded authenticates only a user, even when an
// agent has the same numeric ID.
func TestAuthenticateRequestNeverReadsATokenAsAnotherKind(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "cy", types.RoleBasic)
	agent := createAgentWithUsersID(t, gatewayClient, user)

	for _, tt := range []struct {
		name   string
		claims jwt.MapClaims
		want   principal.Principal
		wantOK bool
	}{
		{
			name: "legacy user token",
			claims: jwt.MapClaims{
				"sub":        fmt.Sprint(user.ID),
				"UserGroups": "mcp,authenticated",
			},
			want:   principal.User(user.ID),
			wantOK: true,
		},
		{
			name: "agent subject without a kind",
			claims: jwt.MapClaims{
				"sub":        principal.Agent(agent.ID).String(),
				"UserGroups": "mcp,authenticated",
			},
		},
		{
			name: "user subject recorded as an agent",
			claims: jwt.MapClaims{
				"sub":                 fmt.Sprint(user.ID),
				"obot_principal_kind": "agent",
				"UserGroups":          "mcp,authenticated",
			},
		},
		{
			name: "agent subject recorded as a user",
			claims: jwt.MapClaims{
				"sub":                 principal.Agent(agent.ID).String(),
				"obot_principal_kind": "user",
				"UserGroups":          "mcp,authenticated",
			},
		},
		{
			name: "unknown kind",
			claims: jwt.MapClaims{
				"sub":                 fmt.Sprint(user.ID),
				"obot_principal_kind": "robot",
				"UserGroups":          "mcp,authenticated",
			},
		},
		{
			name: "subject that is not a principal",
			claims: jwt.MapClaims{
				"sub":                 "system",
				"obot_principal_kind": "user",
				"UserGroups":          "mcp,authenticated",
			},
		},
		{
			name: "agent token",
			claims: jwt.MapClaims{
				"sub":                 principal.Agent(agent.ID).String(),
				"obot_principal_kind": "agent",
				"UserGroups":          "mcp,authenticated",
			},
			want:   principal.Agent(agent.ID),
			wantOK: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, ok, err := tokenService.AuthenticateRequest(bearerRequest(signedToken(t, tokenService, tt.claims)))
			require.NoError(t, err)
			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				assert.Nil(t, response)
				return
			}
			got, ok := principal.FromUserInfo(response.User)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestAgentTokenRoundTrip checks the principal of a composite loopback token issued to an agent. It is built as the
// API key authenticator builds an agent's: the agent's encoded ID as UID and name, its kind, status and display
// name, the token's groups and MCP scope, the originating key, and nothing that only users have.
func TestAgentTokenRoundTrip(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	agent := createTestAgent(t, gatewayClient)
	subject := principal.Agent(agent.ID).String()

	req, _ := authenticateToken(t, tokenService, TokenContext{
		UserID: subject,
		// A name and an email in the token are never an agent's.
		UserName:              "mallory",
		UserEmail:             "mallory@example.com",
		AuthProviderName:      "github",
		AuthProviderNamespace: "default",
		UserGroups:            agentLoopbackGroups,
		MCPID:                 "ms1component",
		AuthorizedMCPIDs:      []string{"vmcp1shared"},
		OAuthConnectionID:     "vmcpi1connection",
		APIKeyID:              "42",
		APIKeyName:            strings.Repeat("k", maxAPIKeyNameCharacters+1),
	})
	response, ok, err := tokenService.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)

	info := response.User
	assert.Equal(t, subject, info.GetUID())
	assert.Equal(t, subject, info.GetName())
	assert.Equal(t, agentLoopbackGroups, info.GetGroups())
	got, ok := principal.FromUserInfo(info)
	require.True(t, ok)
	assert.Equal(t, principal.Agent(agent.ID), got)
	status, recorded := principal.UserStatus(info)
	assert.True(t, recorded)
	assert.Equal(t, types.UserStatusActive, status)

	extra := info.GetExtra()
	assert.Equal(t, []string{"vmcp1shared", "ms1component"}, extra["authorized_mcp_ids"])
	assert.Equal(t, []string{"vmcpi1connection"}, extra["oauth_connection_id"])
	for _, key := range []string{"email", "obot_groups", "auth_provider_groups", "auth_provider_name", "auth_provider_namespace"} {
		assert.NotContains(t, extra, key)
	}
	// An overlong key name is replaced with the masked name of an agent's key.
	attribution, ok := principal.APIKeyAttributionFromUser(info)
	require.True(t, ok)
	assert.Equal(t, principal.APIKeyAttribution{ID: 42, Name: fmt.Sprintf("oak1-%d-42-*****", agent.ID)}, attribution)
}

// TestAgentTokensAreDeniedOnceTheAgentIsInactive checks that a token issued before an agent was disabled or deleted
// is denied from its next use on.
func TestAgentTokensAreDeniedOnceTheAgentIsInactive(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	agent := createTestAgent(t, gatewayClient)
	req, _ := authenticateToken(t, tokenService, TokenContext{
		UserID:     principal.Agent(agent.ID).String(),
		UserGroups: agentLoopbackGroups,
	})
	assertDenied := func(want types.UserStatus) {
		t.Helper()
		response, ok, err := tokenService.AuthenticateRequest(req)
		assert.Nil(t, response)
		assert.False(t, ok)
		denied, isDenied := errors.AsType[*client.AgentAccessDeniedError](err)
		require.True(t, isDenied, "error = %v, want a denial", err)
		assert.Equal(t, agent.ID, denied.AgentID)
		assert.Equal(t, want, denied.Status)
	}

	_, err := gatewayClient.DisableAgent(t.Context(), agent.ID, "compromised")
	require.NoError(t, err)
	assertDenied(types.UserStatusDisabled)

	_, err = gatewayClient.EnableAgent(t.Context(), agent.ID)
	require.NoError(t, err)
	_, ok, err := tokenService.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, gatewayClient.DeleteAgent(t.Context(), agent.ID))
	assertDenied(types.UserStatusDeleted)
}

func TestAgentTokenAuthenticationFailsWhenItCannotReadTheAgent(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	agent := createTestAgent(t, gatewayClient)
	req, _ := authenticateToken(t, tokenService, TokenContext{
		UserID:     principal.Agent(agent.ID).String(),
		UserGroups: agentLoopbackGroups,
	})

	require.NoError(t, gatewayClient.Close())

	response, ok, err := tokenService.AuthenticateRequest(req)
	assert.Nil(t, response)
	assert.False(t, ok)
	lookupErr, isLookup := errors.AsType[*client.AgentAccessLookupError](err)
	require.True(t, isLookup, "error = %v, want a lookup failure", err)
	assert.Equal(t, agent.ID, lookupErr.AgentID)
}

func TestNewTokenRefusesInactiveAgents(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	disabled := createTestAgent(t, gatewayClient)
	_, err := gatewayClient.DisableAgent(t.Context(), disabled.ID, "compromised")
	require.NoError(t, err)
	deleted := createTestAgent(t, gatewayClient)
	require.NoError(t, gatewayClient.DeleteAgent(t.Context(), deleted.ID))

	for _, tt := range []struct {
		name    string
		agentID uint
		want    types.UserStatus
	}{
		{
			name:    "disabled",
			agentID: disabled.ID,
			want:    types.UserStatusDisabled,
		},
		{
			name:    "deleted",
			agentID: deleted.ID,
			want:    types.UserStatusDeleted,
		},
		{
			name:    "missing",
			agentID: deleted.ID + 100,
			want:    types.UserStatusDeleted,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := tokenService.NewToken(t.Context(), TokenContext{
				Audience:   testServerURL,
				UserID:     principal.Agent(tt.agentID).String(),
				UserGroups: agentLoopbackGroups,
			})
			denied, isDenied := errors.AsType[*client.AgentAccessDeniedError](err)
			require.True(t, isDenied, "error = %v, want a denial", err)
			assert.Equal(t, tt.agentID, denied.AgentID)
			assert.Equal(t, tt.want, denied.Status)
		})
	}
}

// TestAgentTokensCarryOnlyMCPGroups checks that no token issued to an agent grants it a group other than those of
// Obot's internal MCP connections, and that a signed token that does authenticates no one.
func TestAgentTokensCarryOnlyMCPGroups(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	agent := createTestAgent(t, gatewayClient)
	subject := principal.Agent(agent.ID).String()

	for _, group := range []string{types.GroupAPI, types.GroupAdmin, types.GroupOwner, types.GroupLLM, types.GroupDeviceScans, types.GroupClaudeEnvironment} {
		t.Run(group, func(t *testing.T) {
			_, _, err := tokenService.NewToken(t.Context(), TokenContext{
				Audience:   testServerURL,
				UserID:     subject,
				UserGroups: append([]string{types.GroupMCP, types.GroupAuthenticated}, group),
			})
			require.Error(t, err)

			response, ok, err := tokenService.AuthenticateRequest(bearerRequest(signedToken(t, tokenService, jwt.MapClaims{
				"sub":                 subject,
				"obot_principal_kind": "agent",
				"UserGroups":          "mcp,authenticated," + group,
			})))
			require.NoError(t, err)
			assert.False(t, ok)
			assert.Nil(t, response)
		})
	}
}

// TestAgentTokensFailClosed checks the authenticator that a chain ending in a catch-all, such as the one of a server
// that does not authenticate callers, runs after the token service and ahead of the catch-all (AgentTokens): a token
// that claims to be an agent's, in either header the token service reads, authenticates as that agent or fails,
// whatever is wrong with it, and every other request is authenticated or declined as before, so that it reaches the
// rest of the chain.
func TestAgentTokensFailClosed(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "dee", types.RoleBasic)
	active := createTestAgent(t, gatewayClient)
	disabled := createTestAgent(t, gatewayClient)
	deleted := createTestAgent(t, gatewayClient)
	agentToken := func(agent *gatewaytypes.Agent) string {
		t.Helper()
		_, token, err := tokenService.NewToken(t.Context(), TokenContext{
			Audience:   testServerURL + "/mcp-connect/server-id",
			ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
			UserID:     principal.Agent(agent.ID).String(),
			UserGroups: agentLoopbackGroups,
		})
		require.NoError(t, err)
		return token
	}
	activeToken := agentToken(active)
	disabledToken := agentToken(disabled)
	deletedToken := agentToken(deleted)
	_, err := gatewayClient.DisableAgent(t.Context(), disabled.ID, "compromised")
	require.NoError(t, err)
	require.NoError(t, gatewayClient.DeleteAgent(t.Context(), deleted.ID))
	_, userToken, err := tokenService.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL,
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     principal.User(user.ID).String(),
		UserGroups: []string{types.GroupAuthenticated},
	})
	require.NoError(t, err)

	// Another server, with its own key, and an active agent with the same ID.
	otherService, otherGateway := newLifecycleTestTokenService(t)
	require.Equal(t, active.ID, createTestAgent(t, otherGateway).ID)
	_, otherKeyToken, err := otherService.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL,
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     principal.Agent(active.ID).String(),
		UserGroups: agentLoopbackGroups,
	})
	require.NoError(t, err)

	for _, tt := range []struct {
		name  string
		token string
		// declined is set for a request that is not an agent's, which is left to the rest of the chain.
		declined bool
		// authenticatedAs is set for a token that the token service authenticates.
		authenticatedAs principal.Principal
	}{
		{
			name:     "no token",
			declined: true,
		},
		{
			name:     "not a token",
			token:    "ok1-1-2-secret",
			declined: true,
		},
		{
			name:            "a user's token",
			token:           userToken,
			authenticatedAs: principal.User(user.ID),
		},
		{
			name:            "an active agent's token",
			token:           activeToken,
			authenticatedAs: principal.Agent(active.ID),
		},
		{
			name:  "a disabled agent's token",
			token: disabledToken,
		},
		{
			name:  "a deleted agent's token",
			token: deletedToken,
		},
		{
			name: "an expired agent token",
			token: signedToken(t, tokenService, jwt.MapClaims{
				"sub":                 principal.Agent(active.ID).String(),
				"obot_principal_kind": "agent",
				"UserGroups":          strings.Join(agentLoopbackGroups, ","),
				"exp":                 time.Now().Add(-time.Minute).Unix(),
			}),
		},
		{
			name: "an agent token with a group agents are not issued",
			token: signedToken(t, tokenService, jwt.MapClaims{
				"sub":                 principal.Agent(active.ID).String(),
				"obot_principal_kind": "agent",
				"UserGroups":          "mcp,authenticated," + types.GroupAdmin,
			}),
		},
		{
			name:  "an agent token signed with another key",
			token: otherKeyToken,
		},
		{
			name: "a malformed agent subject",
			token: signedToken(t, tokenService, jwt.MapClaims{
				"sub":        "agent:not-a-number",
				"UserGroups": strings.Join(agentLoopbackGroups, ","),
			}),
		},
	} {
		for _, header := range []string{"Authorization", "X-API-Key"} {
			t.Run(tt.name+" in "+header, func(t *testing.T) {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testServerURL, nil)
				switch {
				case tt.token == "":
				case header == "Authorization":
					req.Header.Set(header, "Bearer "+tt.token)
				default:
					req.Header.Set(header, tt.token)
				}
				// The chain of a server that does not authenticate callers (server.WithNoAuth), without its catch-all.
				chain := union.New(tokenService, union.NewFailOnError(tokenService.AgentTokens()))
				response, ok, err := chain.AuthenticateRequest(req)
				switch {
				case tt.declined:
					require.NoError(t, err)
					require.False(t, ok)
					require.Nil(t, response)
				case tt.authenticatedAs != principal.Principal{}:
					require.NoError(t, err)
					require.True(t, ok)
					got, isPrincipal := principal.FromUserInfo(response.User)
					require.True(t, isPrincipal)
					require.Equal(t, tt.authenticatedAs, got)
				default:
					require.Error(t, err, "the token was not refused")
					require.False(t, ok)
					require.Nil(t, response)
				}
			})
		}
	}
}

// TestNewTokenTrustsTheAdmittedAgent checks that a token minted while handling an agent's admitted request is not
// checked again for that agent, and that this never extends to the user with the agent's numeric ID, or the reverse.
func TestNewTokenTrustsTheAdmittedAgent(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "dee", types.RoleBasic)
	agent := createAgentWithUsersID(t, gatewayClient, user)

	// Both are disabled after their requests were admitted.
	_, err := gatewayClient.DisableAgent(t.Context(), agent.ID, "compromised")
	require.NoError(t, err)
	require.NoError(t, deactivateThroughSCIM(t, gatewayClient, lifecycleTestProvider, "00u-"+user.Username, user.Email))

	admitted := func(p principal.Principal) kuser.Info {
		extra := map[string][]string{}
		principal.RecordKind(extra, p.Kind)
		principal.RecordUserStatus(extra, types.UserStatusActive)
		return &kuser.DefaultInfo{
			UID:   p.String(),
			Extra: extra,
		}
	}
	mint := func(admittedAs, subject principal.Principal) error {
		t.Helper()
		_, _, err := tokenService.NewToken(principal.WithAdmittedPrincipal(t.Context(), admitted(admittedAs)), TokenContext{
			Audience:   testServerURL,
			UserID:     subject.String(),
			UserGroups: agentLoopbackGroups,
		})
		return err
	}

	require.NoError(t, mint(principal.Agent(agent.ID), principal.Agent(agent.ID)))
	require.NoError(t, mint(principal.User(user.ID), principal.User(user.ID)))

	_, isDenied := errors.AsType[*client.UserAccessDeniedError](mint(principal.Agent(agent.ID), principal.User(user.ID)))
	require.True(t, isDenied, "a token for the user was issued on the agent's admission")
	_, isDenied = errors.AsType[*client.AgentAccessDeniedError](mint(principal.User(user.ID), principal.Agent(agent.ID)))
	require.True(t, isDenied, "a token for the agent was issued on the user's admission")
}
