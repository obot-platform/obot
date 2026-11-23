package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentdelegation"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gserver "github.com/obot-platform/obot/pkg/gateway/server"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	delegationTestServerURL = "http://obot.test"
	delegationTestReturnURL = "https://agent.example.com/callback?session=s1"
	delegationTestVMCP      = "vmcp1delegated"
	delegationTestProfile   = "vp1delegated"
	delegationTestPolicy    = "map1delegated"
	delegationTestSkillRule = "sar1delegated"
)

var (
	// The secrets of two browser sessions with the agent.
	delegationSecretA = strings.Repeat("A", 43)
	delegationSecretB = strings.Repeat("B", 43)
)

// recordingCleanup records the scopes it is asked to clean up, and fails with err if it is set.
type recordingCleanup struct {
	lock   sync.Mutex
	scopes []agentdelegation.Scope
	err    error
}

// delegationFixture serves the agent admin API and the delegation routes from a real gateway database and token
// service, behind the server's authorizer, as the server does. Its agent has a key, a profile for every user, the
// return host agent.example.com, and a vMCP profile, model access policy and skill access rule that name it. Alice and
// Bob are basic users.
type delegationFixture struct {
	*agentIdentityFixture
	tokens      *persistent.TokenService
	cleanup     *recordingCleanup
	authorizer  *authz.Authorizer
	delegations http.Handler
	routes      []string

	agent types.AgentIdentity
	key   types.AgentAPIKeyCreateResponse
	alice user.Info
	bob   user.Info
	admin user.Info
}

func (r *recordingCleanup) CleanupDelegations(_ context.Context, scope agentdelegation.Scope) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.scopes = append(r.scopes, scope)
	return r.err
}

func newDelegationFixture(t *testing.T) *delegationFixture {
	t.Helper()

	f := &delegationFixture{
		agentIdentityFixture: newAgentIdentityFixture(t),
		cleanup:              &recordingCleanup{},
	}
	f.admin = f.caller
	tokens, err := persistent.NewTokenService(delegationTestServerURL, f.gateway)
	require.NoError(t, err)
	require.NoError(t, tokens.EnsureJWK(t.Context()))
	f.tokens = tokens
	f.authorizer = authz.NewAuthorizer(f.gateway, f.storage, f.storage, false, nil, nil, nil)

	mux := http.NewServeMux()
	NewAgentDelegationHandler(tokens, f.cleanup, delegationTestServerURL+"/").RegisterRoutes(func(pattern string, handle api.HandlerFunc) {
		f.routes = append(f.routes, pattern)
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			// The server authorizes a request before its handler runs, and answers a refusal like this.
			if !f.authorizer.Authorize(r, f.caller) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			err := handle(api.Context{
				ResponseWriter: w,
				Request:        r,
				GatewayClient:  f.gateway,
				Storage:        f.storage,
				User:           f.caller,
			})
			if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
				http.Error(w, httpErr.Message, httpErr.Code)
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	})
	f.delegations = mux

	f.alice = f.createUser("alice", types.RoleBasic)
	f.bob = f.createUser("bob", types.RoleBasic)

	f.agent = f.createAgent(types.AgentIdentityManifest{
		DisplayName:        "assistant",
		Description:        "answers questions",
		Profiles:           []types.AgentProfile{{Name: "everyone", Subjects: everyUser()}},
		AllowedReturnHosts: []string{"Agent.Example.com", "localhost:3000"},
	})
	f.key = f.createKey(f.agent.ID, types.AgentAPIKeyCreateRequest{Name: "web", CanAccessLLMProxy: true})
	named := []types.Subject{{Type: types.SubjectTypeAgent, ID: f.agent.ID}}
	for _, obj := range []kclient.Object{
		&v1.VMCP{
			Name: delegationTestVMCP, Namespace: system.DefaultNamespace,
			Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
				DisplayName: "Docs",
				Profiles:    []types.VMCPProfile{{ID: delegationTestProfile, Name: "read only", Subjects: named}},
			}},
		},
		&v1.ModelAccessPolicy{
			Name: delegationTestPolicy, Namespace: system.DefaultNamespace,
			Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{DisplayName: "Models", Subjects: named}},
		},
		&v1.SkillAccessRule{
			Name: delegationTestSkillRule, Namespace: system.DefaultNamespace,
			Spec: v1.SkillAccessRuleSpec{Manifest: types.SkillAccessRuleManifest{DisplayName: "Skills", Subjects: named}},
		},
	} {
		require.NoError(t, f.storage.Create(t.Context(), obj))
	}
	return f
}

// createUser creates a user with role, and returns them as their browser session authenticates them.
func (f *delegationFixture) createUser(name string, role types.Role) user.Info {
	f.t.Helper()
	email := name + "@example.com"
	u := &gt.User{
		Username:       name,
		HashedUsername: hash.String(name),
		Email:          email,
		HashedEmail:    hash.String(email),
		DisplayName:    strings.ToUpper(name[:1]) + name[1:],
		Role:           role,
	}
	require.NoError(f.t, f.db.Create(u).Error)
	info, err := f.gateway.UserInfoByID(f.t.Context(), u.ID)
	require.NoError(f.t, err)
	return info
}

// serve makes a request of the delegation routes as caller.
func (f *delegationFixture) serve(caller user.Info, req *http.Request) *httptest.ResponseRecorder {
	f.t.Helper()
	f.caller = caller
	defer func() { f.caller = f.admin }()
	rec := httptest.NewRecorder()
	f.delegations.ServeHTTP(rec, req)
	return rec
}

func newJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		require.NoError(t, err)
	}
	return httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(data))
}

// asAgent makes a request as an agent with key, authenticated as the server's API key authenticator does.
func (f *delegationFixture) asAgent(key, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	req := newJSONRequest(f.t, method, path, body)
	req.Header.Set("Authorization", "Bearer "+key)
	response, ok, err := gserver.NewAPIKeyAuthenticator(f.gateway).AuthenticateRequest(req)
	require.NoError(f.t, err)
	require.True(f.t, ok, "the agent's key authenticates")
	return f.serve(response.User, req)
}

// asUser makes a request as a signed-in user's browser on Obot's own pages.
func (f *delegationFixture) asUser(caller user.Info, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	req := newJSONRequest(f.t, method, path, body)
	if method != http.MethodGet {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	return f.serve(caller, req)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	var out T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return out
}

func s256(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// requestApproval asks for approval as the fixture's agent, from the browser session with secret.
func (f *delegationFixture) requestApproval(secret string) types.AgentDelegationRequestCreated {
	f.t.Helper()
	return decode[types.AgentDelegationRequestCreated](f.t, f.asAgent(f.key.Key, http.MethodPost, "/api/agent-delegations", types.AgentDelegationRequestCreate{
		ReturnURL:  delegationTestReturnURL,
		SecretHash: s256(secret),
	}), http.StatusCreated)
}

// approve approves a request as approver, and returns the code from the URL their browser is sent to.
func (f *delegationFixture) approve(approver user.Info, requestID string) string {
	f.t.Helper()
	redirect := decode[types.AgentDelegationRedirect](f.t, f.asUser(approver, http.MethodPost, "/api/agent-delegation-requests/"+requestID+"/approve", nil), http.StatusOK)
	u, err := url.Parse(redirect.RedirectURL)
	require.NoError(f.t, err)
	require.Equal(f.t, "agent.example.com", u.Host)
	require.Equal(f.t, "/callback", u.Path)
	require.Equal(f.t, "s1", u.Query().Get("session"))
	require.Equal(f.t, requestID, u.Query().Get("requestID"))
	require.NotEmpty(f.t, u.Query().Get("code"))
	return u.Query().Get("code")
}

func (f *delegationFixture) redeem(key, requestID, code, secret string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.asAgent(key, http.MethodPost, "/api/agent-delegations/token", types.AgentDelegationTokenRequest{
		GrantType: types.AgentDelegationGrantCode,
		RequestID: requestID,
		Code:      code,
		Secret:    secret,
	})
}

func (f *delegationFixture) mint(key, delegationID string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.asAgent(key, http.MethodPost, "/api/agent-delegations/token", types.AgentDelegationTokenRequest{
		GrantType:    types.AgentDelegationGrantDelegation,
		DelegationID: delegationID,
	})
}

// delegate runs the whole approval for u in the session that asked, and returns the token the agent gets.
func (f *delegationFixture) delegate(u user.Info) types.AgentDelegationToken {
	f.t.Helper()
	request := f.requestApproval(delegationSecretA)
	code := f.approve(u, request.RequestID)
	return decode[types.AgentDelegationToken](f.t, f.redeem(f.key.Key, request.RequestID, code, delegationSecretA), http.StatusOK)
}

func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, status int, contains string) {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), contains)
}

func TestAgentDelegationOverHTTP(t *testing.T) {
	f := newDelegationFixture(t)

	// The agent's allowed return hosts are part of the agent.
	require.Equal(t, []string{"agent.example.com", "localhost:3000"}, f.agent.AllowedReturnHosts)

	// The agent asks for approval, with a login hint for sign-in.
	before := time.Now()
	created := decode[types.AgentDelegationRequestCreated](t, f.asAgent(f.key.Key, http.MethodPost, "/api/agent-delegations", types.AgentDelegationRequestCreate{
		ReturnURL:  delegationTestReturnURL,
		SecretHash: s256(delegationSecretA),
		LoginHint:  "alice@example.com",
	}), http.StatusCreated)
	require.True(t, strings.HasPrefix(created.RequestID, types.AgentDelegationRequestIDPrefix))
	require.Equal(t, delegationTestServerURL+"/agents/approve/"+created.RequestID+"?login_hint=alice%40example.com", created.ApprovalURL)
	require.WithinDuration(t, before.Add(gclient.AgentDelegationRequestTTL), created.ExpiresAt.Time, 5*time.Second)

	// Alice opens the link, and sees the agent and what it may do for her.
	request := decode[types.AgentDelegationApprovalRequest](t, f.asUser(f.alice, http.MethodGet, "/api/agent-delegation-requests/"+created.RequestID, nil), http.StatusOK)
	require.Equal(t, types.AgentDelegationApprovalRequest{
		ID: created.RequestID,
		Agent: types.AgentDelegationAgent{
			ID:          f.agent.ID,
			DisplayName: "assistant",
			Description: "answers questions",
			Status:      types.UserStatusActive,
		},
		ExpiresAt: created.ExpiresAt,
		Access: types.AgentDelegationAccess{
			VMCPs: []types.AgentDelegationVMCPAccess{{
				VMCPID:          delegationTestVMCP,
				VMCPDisplayName: "Docs",
				Profiles:        []types.AgentDelegationVMCPProfile{{ID: delegationTestProfile, Name: "read only"}},
			}},
			ModelAccessPolicies: []types.AgentIdentityPolicyAccess{{ID: delegationTestPolicy, DisplayName: "Models"}},
			SkillAccessRules:    []types.AgentIdentityPolicyAccess{{ID: delegationTestSkillRule, DisplayName: "Skills"}},
		},
	}, request)

	// She approves, and her browser goes back to the agent with a code, which the agent redeems with the secret it
	// kept in her session, for a token that acts on her behalf.
	code := f.approve(f.alice, created.RequestID)
	before = time.Now()
	token := decode[types.AgentDelegationToken](t, f.redeem(f.key.Key, created.RequestID, code, delegationSecretA), http.StatusOK)
	require.True(t, strings.HasPrefix(token.DelegationID, types.AgentDelegationIDPrefix))
	require.Equal(t, "Bearer", token.TokenType)
	require.WithinDuration(t, before.Add(persistent.DelegatedTokenTTL), token.ExpiresAt.Time, 5*time.Second)
	require.Equal(t, types.AgentDelegationUser{ID: f.alice.GetUID(), Email: "alice@example.com", DisplayName: "Alice", Status: types.UserStatusActive}, token.User)

	agentID, keyID := uintOf(t, f.agent.ID), f.key.ID
	decoded, err := f.tokens.DecodeDelegatedToken(t.Context(), token.AccessToken)
	require.NoError(t, err)
	require.Equal(t, agentID, decoded.AgentID)
	require.Equal(t, uintOf(t, f.alice.GetUID()), decoded.UserID)
	require.Equal(t, token.DelegationID, decoded.DelegationID)
	require.Equal(t, keyID, decoded.APIKeyID, "the calling key mints the token")
	// No other decoder accepts it.
	_, err = f.tokens.DecodeToken(t.Context(), token.AccessToken)
	require.ErrorIs(t, err, persistent.ErrDelegatedToken)

	// The code is used once.
	requireStatus(t, f.redeem(f.key.Key, created.RequestID, code, delegationSecretA), http.StatusBadRequest, "already used")

	// Later tokens come from the delegation, with any key of the agent, which mints them.
	stored, err := f.gateway.GetAgentDelegation(t.Context(), token.DelegationID)
	require.NoError(t, err)
	firstUse := *stored.LastUsedAt
	time.Sleep(5 * time.Millisecond)
	otherKey := f.createKey(f.agent.ID, types.AgentAPIKeyCreateRequest{Name: "worker", MCPServerIDs: []string{"*"}})
	again := decode[types.AgentDelegationToken](t, f.mint(otherKey.Key, token.DelegationID), http.StatusOK)
	require.Equal(t, token.DelegationID, again.DelegationID)
	require.NotEqual(t, token.AccessToken, again.AccessToken)
	decoded, err = f.tokens.DecodeDelegatedToken(t.Context(), again.AccessToken)
	require.NoError(t, err)
	require.Equal(t, otherKey.ID, decoded.APIKeyID)
	stored, err = f.gateway.GetAgentDelegation(t.Context(), token.DelegationID)
	require.NoError(t, err)
	require.True(t, stored.LastUsedAt.After(firstUse), "the delegation's last use moves")

	// Approving again keeps the delegation.
	require.Equal(t, token.DelegationID, f.delegate(f.alice).DelegationID)

	// Alice sees the agent acting for her, and an administrator sees Alice among the agent's delegations.
	mine := decode[types.AgentDelegationList](t, f.asUser(f.alice, http.MethodGet, "/api/me/agent-delegations", nil), http.StatusOK)
	require.Len(t, mine.Items, 1)
	require.Equal(t, token.DelegationID, mine.Items[0].ID)
	require.Equal(t, "assistant", mine.Items[0].Agent.DisplayName)
	require.NotNil(t, mine.Items[0].LastUsedAt)
	require.Empty(t, decode[types.AgentDelegationList](t, f.asUser(f.bob, http.MethodGet, "/api/me/agent-delegations", nil), http.StatusOK).Items)
	listed := decode[types.AgentDelegationList](t, f.asUser(f.admin, http.MethodGet, "/api/agents/"+f.agent.ID+"/delegations", nil), http.StatusOK)
	require.Len(t, listed.Items, 1)
	require.Equal(t, token.User, listed.Items[0].User)
	require.False(t, listed.Items[0].FirstApprovedAt.IsZero())
	require.True(t, listed.Items[0].LastApprovedAt.Time.After(listed.Items[0].FirstApprovedAt.Time))

	// Each step is in the admin change audit, with who did it.
	events, _, err := f.gateway.ListAdminAuditEvents(t.Context(), gclient.AdminAuditEventOptions{
		Actions: []string{
			gt.AdminAuditActionAgentDelegationRequestCreate,
			gt.AdminAuditActionAgentDelegationApprove,
			gt.AdminAuditActionAgentDelegationRequestRedeem,
		},
		Oldest: true,
	})
	require.NoError(t, err)
	var actions []string
	for _, event := range events {
		actions = append(actions, event.Action+" by "+event.ActorUserID)
	}
	agentActor, aliceActor := principal.Agent(agentID).String(), f.alice.GetUID()
	require.Equal(t, []string{
		gt.AdminAuditActionAgentDelegationRequestCreate + " by " + agentActor,
		gt.AdminAuditActionAgentDelegationApprove + " by " + aliceActor,
		gt.AdminAuditActionAgentDelegationRequestRedeem + " by " + agentActor,
		gt.AdminAuditActionAgentDelegationRequestCreate + " by " + agentActor,
		gt.AdminAuditActionAgentDelegationApprove + " by " + aliceActor,
		gt.AdminAuditActionAgentDelegationRequestRedeem + " by " + agentActor,
	}, actions)

	// The audit API names the agent as the actor of its steps.
	list, err := callAdminAuditEvents(t, f.gateway, "resourceKind="+gt.AdminAuditResourceAgentDelegationRequest)
	require.NoError(t, err)
	require.NotEmpty(t, list.Items)
	require.Equal(t, types.AuditLogActor{ActorType: types.AuditLogActorTypeAgent, ID: f.agent.ID, DisplayName: "assistant"}, list.Items[0].Actor)
	require.Equal(t, f.agent.ID, list.Items[0].ParentID)
}

// TestForwardedApprovalLinkOverHTTP is the attack Arcade calls COAT, end to end: a user of the agent forwards the
// approval link of their session to a victim, who approves it. The victim's browser takes the code back to the agent
// in the victim's own session, whose secret is not the one the request was made with, so the code is refused and used
// up. The one who forwarded the link never gets a token for the victim.
func TestForwardedApprovalLinkOverHTTP(t *testing.T) {
	f := newDelegationFixture(t)

	// Session A asks, and Bob approves the forwarded link.
	created := f.requestApproval(delegationSecretA)
	code := f.approve(f.bob, created.RequestID)

	// The code comes back in Bob's session B.
	requireStatus(t, f.redeem(f.key.Key, created.RequestID, code, delegationSecretB), http.StatusBadRequest, "secret does not match")
	// Session A, had it the code, could no longer redeem it.
	requireStatus(t, f.redeem(f.key.Key, created.RequestID, code, delegationSecretA), http.StatusBadRequest, "already used")
	// A session with no secret at all, or with one that is not even well formed, uses the code up the same way.
	for _, secret := range []string{"", "short"} {
		created = f.requestApproval(delegationSecretA)
		code = f.approve(f.bob, created.RequestID)
		requireStatus(t, f.redeem(f.key.Key, created.RequestID, code, secret), http.StatusBadRequest, "secret does not match")
		requireStatus(t, f.redeem(f.key.Key, created.RequestID, code, delegationSecretA), http.StatusBadRequest, "already used")
	}

	// Bob sees that he approved the agent, and can revoke it; the agent never learned the delegation's ID.
	mine := decode[types.AgentDelegationList](t, f.asUser(f.bob, http.MethodGet, "/api/me/agent-delegations", nil), http.StatusOK)
	require.Len(t, mine.Items, 1)
	require.Nil(t, mine.Items[0].LastUsedAt, "no token was ever minted")
}

func TestAgentDelegationRequestRefusalsOverHTTP(t *testing.T) {
	f := newDelegationFixture(t)
	create := func(key string, body any) *httptest.ResponseRecorder {
		return f.asAgent(key, http.MethodPost, "/api/agent-delegations", body)
	}
	valid := types.AgentDelegationRequestCreate{ReturnURL: delegationTestReturnURL, SecretHash: s256(delegationSecretA)}

	other := f.createAgent(types.AgentIdentityManifest{DisplayName: "other", AllowedReturnHosts: []string{"agent.example.com"}})
	otherKey := f.createKey(other.ID, types.AgentAPIKeyCreateRequest{Name: "web", CanAccessLLMProxy: true})
	noHosts := f.createAgent(types.AgentIdentityManifest{DisplayName: "no hosts"})
	noHostsKey := f.createKey(noHosts.ID, types.AgentAPIKeyCreateRequest{Name: "web", CanAccessLLMProxy: true})
	theirs := f.delegate(f.alice)

	for _, tt := range []struct {
		name     string
		key      string
		body     any
		contains string
	}{
		{
			name:     "another host",
			body:     types.AgentDelegationRequestCreate{ReturnURL: "https://evil.example.com/callback", SecretHash: valid.SecretHash},
			contains: "not an allowed return host",
		},
		{
			name:     "http to a host that is not a loopback",
			body:     types.AgentDelegationRequestCreate{ReturnURL: "http://agent.example.com/callback", SecretHash: valid.SecretHash},
			contains: "must use https",
		},
		{
			name:     "an agent with no allowed hosts",
			key:      noHostsKey.Key,
			body:     valid,
			contains: "no allowed return hosts",
		},
		{
			name:     "a malformed secret hash",
			body:     types.AgentDelegationRequestCreate{ReturnURL: delegationTestReturnURL, SecretHash: "not-a-hash"},
			contains: "secretHash",
		},
		{
			name:     "another agent's delegation",
			key:      otherKey.Key,
			body:     types.AgentDelegationRequestCreate{ReturnURL: delegationTestReturnURL, SecretHash: valid.SecretHash, DelegationID: theirs.DelegationID},
			contains: "is not a delegation of the agent",
		},
		{
			name:     "an unknown field",
			body:     map[string]string{"returnURL": delegationTestReturnURL, "secretHash": valid.SecretHash, "secret": delegationSecretA},
			contains: "unknown field",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.key
			if key == "" {
				key = f.key.Key
			}
			requireStatus(t, create(key, tt.body), http.StatusBadRequest, tt.contains)
		})
	}

	// Plain http to a loopback host on the list is allowed, for development.
	decode[types.AgentDelegationRequestCreated](t, create(f.key.Key, types.AgentDelegationRequestCreate{ReturnURL: "http://localhost:3000/callback", SecretHash: valid.SecretHash}), http.StatusCreated)

	// An agent with too many requests waiting is told to slow down.
	for range gclient.MaxPendingAgentDelegationRequests {
		_, err := f.gateway.CreateAgentDelegationRequest(t.Context(), gclient.AgentDelegationRequestInput{AgentID: uintOf(t, other.ID), APIKeyID: otherKey.ID, ReturnURL: delegationTestReturnURL, SecretHash: valid.SecretHash})
		require.NoError(t, err)
	}
	require.Equal(t, http.StatusTooManyRequests, create(otherKey.Key, valid).Code)

	// A disabled agent is refused, even with a principal authenticated before it was disabled.
	req := newJSONRequest(t, http.MethodPost, "/api/agent-delegations", valid)
	req.Header.Set("Authorization", "Bearer "+f.key.Key)
	response, ok, err := gserver.NewAPIKeyAuthenticator(f.gateway).AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+f.agent.ID+"/disable", nil, nil)
	requireStatus(t, f.serve(response.User, req), http.StatusForbidden, "")
}

// TestAgentDelegationRoutesNeedTheAgentsKey checks that only an agent's own API key creates requests and mints tokens:
// not a user, not a token the agent was issued for an internal connection, not a token that acts on behalf of a user,
// and not a principal whose request carries some other credential.
func TestAgentDelegationRoutesNeedTheAgentsKey(t *testing.T) {
	f := newDelegationFixture(t)
	token := f.delegate(f.alice)

	keyPrincipal := func() user.Info {
		req := newJSONRequest(t, http.MethodPost, "/api/agent-delegations", nil)
		req.Header.Set("Authorization", "Bearer "+f.key.Key)
		response, ok, err := gserver.NewAPIKeyAuthenticator(f.gateway).AuthenticateRequest(req)
		require.NoError(t, err)
		require.True(t, ok)
		return response.User
	}
	withExtra := func(info user.Info, groups []string, extra map[string][]string) user.Info {
		merged := maps.Clone(info.GetExtra())
		maps.Copy(merged, extra)
		return &user.DefaultInfo{Name: info.GetName(), UID: info.GetUID(), Groups: append(info.GetGroups(), groups...), Extra: merged}
	}
	agent := keyPrincipal()
	onBehalfOf := withExtra(agent, nil, nil)
	principal.RecordOnBehalfOf(onBehalfOf.GetExtra(), uintOf(t, f.alice.GetUID()))
	otherKey := f.createKey(f.agent.ID, types.AgentAPIKeyCreateRequest{Name: "other", CanAccessLLMProxy: true})

	for _, tt := range []struct {
		name          string
		caller        user.Info
		authorization string
	}{
		{
			name:          "a user",
			caller:        f.alice,
			authorization: "Bearer " + f.key.Key,
		},
		{
			name:          "the agent's composite loopback",
			caller:        withExtra(agent, []string{types.GroupCompositeMCP}, nil),
			authorization: "Bearer " + f.key.Key,
		},
		{
			name:          "the agent acting on behalf of a user",
			caller:        onBehalfOf,
			authorization: "Bearer " + token.AccessToken,
		},
		{
			name:          "a token in the request",
			caller:        agent,
			authorization: "Bearer " + token.AccessToken,
		},
		{
			name:          "another key of the agent in the request",
			caller:        agent,
			authorization: "Bearer " + otherKey.Key,
		},
		{
			name:   "no credential in the request",
			caller: agent,
		},
		{
			name: "an agent principal without a key",
			caller: gclient.AgentUserInfo(&gt.Agent{ID: uintOf(t, f.agent.ID)}, []string{types.GroupAuthenticated, types.GroupMCP}, map[string][]string{
				"authorized_mcp_ids": {"vmcp1"},
			}),
			authorization: "Bearer " + f.key.Key,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, body := range []any{
				types.AgentDelegationRequestCreate{ReturnURL: delegationTestReturnURL, SecretHash: s256(delegationSecretA)},
				types.AgentDelegationTokenRequest{GrantType: types.AgentDelegationGrantDelegation, DelegationID: token.DelegationID},
			} {
				path := "/api/agent-delegations"
				if _, ok := body.(types.AgentDelegationTokenRequest); ok {
					path += "/token"
				}
				req := newJSONRequest(t, http.MethodPost, path, body)
				if tt.authorization != "" {
					req.Header.Set("Authorization", tt.authorization)
				}
				requireStatus(t, f.serve(tt.caller, req), http.StatusForbidden, "forbidden")
			}
		})
	}
}

func TestAgentDelegationApprovalRefusalsOverHTTP(t *testing.T) {
	f := newDelegationFixture(t)
	path := func(id string) string { return "/api/agent-delegation-requests/" + id }

	// A user no profile matches.
	f.call(http.StatusOK, http.MethodPut, "/api/agents/"+f.agent.ID, types.AgentIdentityManifest{
		DisplayName:        "assistant",
		Profiles:           []types.AgentProfile{{Name: "bob", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: f.bob.GetUID()}}}},
		AllowedReturnHosts: f.agent.AllowedReturnHosts,
	}, nil)
	created := f.requestApproval(delegationSecretA)
	requireStatus(t, f.asUser(f.alice, http.MethodGet, path(created.RequestID), nil), http.StatusForbidden, "isn't allowed to act on your behalf")
	requireStatus(t, f.asUser(f.alice, http.MethodPost, path(created.RequestID)+"/approve", nil), http.StatusForbidden, "isn't allowed to act on your behalf")
	requireStatus(t, f.asUser(f.alice, http.MethodPost, path(created.RequestID)+"/cancel", nil), http.StatusForbidden, "isn't allowed to act on your behalf")

	// The approval page's POSTs are refused when another site makes them.
	for _, headers := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site"},
		{"Origin": "https://evil.example.com"},
	} {
		for _, action := range []string{"/approve", "/cancel"} {
			req := newJSONRequest(t, http.MethodPost, path(created.RequestID)+action, nil)
			for name, value := range headers {
				req.Header.Set(name, value)
			}
			requireStatus(t, f.serve(f.bob, req), http.StatusForbidden, "forbidden")
		}
	}
	// The same POST from Obot's own page goes through.
	f.approve(f.bob, created.RequestID)

	// A used request is refused, and so is a cancelled or an expired one, or one that does not exist.
	requireStatus(t, f.asUser(f.bob, http.MethodGet, path(created.RequestID), nil), http.StatusForbidden, "expired or was already used")
	requireStatus(t, f.asUser(f.bob, http.MethodPost, path(created.RequestID)+"/approve", nil), http.StatusForbidden, "expired or was already used")
	cancelled := f.requestApproval(delegationSecretA)
	redirect := decode[types.AgentDelegationRedirect](t, f.asUser(f.bob, http.MethodPost, path(cancelled.RequestID)+"/cancel", nil), http.StatusOK)
	u, err := url.Parse(redirect.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, url.Values{"session": {"s1"}, "requestID": {cancelled.RequestID}, "error": {"access_denied"}}, u.Query())
	requireStatus(t, f.asUser(f.bob, http.MethodPost, path(cancelled.RequestID)+"/approve", nil), http.StatusForbidden, "expired or was already used")
	expired := f.requestApproval(delegationSecretA)
	require.NoError(t, f.db.Model(&gt.AgentDelegationRequest{}).Where("id = ?", expired.RequestID).Update("expires_at", time.Now().Add(-time.Second)).Error)
	requireStatus(t, f.asUser(f.bob, http.MethodGet, path(expired.RequestID), nil), http.StatusForbidden, "expired or was already used")
	requireStatus(t, f.asUser(f.bob, http.MethodPost, path(expired.RequestID)+"/approve", nil), http.StatusForbidden, "expired or was already used")
	requireStatus(t, f.asUser(f.bob, http.MethodGet, path("adr1missing"), nil), http.StatusNotFound, "doesn't exist")

	// Approval is a browser's: not the bootstrap login's, which is recognized by its auth provider rather than its
	// username, and not an API key's.
	pending := f.requestApproval(delegationSecretA)
	bootstrap := &user.DefaultInfo{Name: system.BootstrapName, UID: f.bob.GetUID(), Groups: f.bob.GetGroups(), Extra: maps.Clone(f.bob.GetExtra())}
	bootstrap.Extra["auth_provider_name"] = []string{system.BootstrapName}
	delete(bootstrap.Extra, "auth_provider_namespace")
	requireStatus(t, f.asUser(bootstrap, http.MethodPost, path(pending.RequestID)+"/approve", nil), http.StatusForbidden, "forbidden")
	apiKey := &user.DefaultInfo{Name: f.bob.GetName(), UID: f.bob.GetUID(), Groups: f.bob.GetGroups(), Extra: maps.Clone(f.bob.GetExtra())}
	apiKey.Extra[principal.APIKeyIDExtra] = []string{"5"}
	requireStatus(t, f.asUser(apiKey, http.MethodPost, path(pending.RequestID)+"/approve", nil), http.StatusForbidden, "forbidden")
	agent := gclient.AgentUserInfo(&gt.Agent{ID: uintOf(t, f.agent.ID)}, []string{types.GroupAuthenticated}, nil)
	requireStatus(t, f.asUser(agent, http.MethodPost, path(pending.RequestID)+"/approve", nil), http.StatusForbidden, "forbidden")

	// A disabled agent can't be approved.
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+f.agent.ID+"/disable", nil, nil)
	requireStatus(t, f.asUser(f.bob, http.MethodGet, path(pending.RequestID), nil), http.StatusForbidden, "disabled")
	requireStatus(t, f.asUser(f.bob, http.MethodPost, path(pending.RequestID)+"/approve", nil), http.StatusForbidden, "disabled")
}

func TestAgentDelegationMintingRefusalsOverHTTP(t *testing.T) {
	f := newDelegationFixture(t)
	alice, bob := f.delegate(f.alice), f.delegate(f.bob)
	decode[types.AgentDelegationToken](t, f.mint(f.key.Key, alice.DelegationID), http.StatusOK)

	// Another agent's delegation is not found.
	other := f.createAgent(types.AgentIdentityManifest{DisplayName: "other"})
	otherKey := f.createKey(other.ID, types.AgentAPIKeyCreateRequest{Name: "web", CanAccessLLMProxy: true})
	requireStatus(t, f.mint(otherKey.Key, alice.DelegationID), http.StatusNotFound, "not found")
	requireStatus(t, f.mint(f.key.Key, "adl1missing"), http.StatusNotFound, "not found")

	// A grant that mixes the fields of the two, or names neither, is refused.
	requireStatus(t, f.asAgent(f.key.Key, http.MethodPost, "/api/agent-delegations/token", types.AgentDelegationTokenRequest{GrantType: types.AgentDelegationGrantDelegation, DelegationID: alice.DelegationID, Secret: delegationSecretA}), http.StatusBadRequest, "not part of")
	requireStatus(t, f.asAgent(f.key.Key, http.MethodPost, "/api/agent-delegations/token", types.AgentDelegationTokenRequest{GrantType: "refresh_token"}), http.StatusBadRequest, "grantType")

	// A disabled user is refused, and minting works again once they are enabled.
	require.NoError(t, f.db.Model(&gt.User{}).Where("id = ?", f.bob.GetUID()).Update("disabled_at", time.Now()).Error)
	requireStatus(t, f.mint(f.key.Key, bob.DelegationID), http.StatusForbidden, "disabled")
	require.NoError(t, f.db.Model(&gt.User{}).Where("id = ?", f.bob.GetUID()).Update("disabled_at", nil).Error)
	decode[types.AgentDelegationToken](t, f.mint(f.key.Key, bob.DelegationID), http.StatusOK)

	// A user no profile matches any more is refused.
	f.call(http.StatusOK, http.MethodPut, "/api/agents/"+f.agent.ID, types.AgentIdentityManifest{
		DisplayName:        "assistant",
		Profiles:           []types.AgentProfile{{Name: "eng", Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}}},
		AllowedReturnHosts: f.agent.AllowedReturnHosts,
	}, nil)
	requireStatus(t, f.mint(f.key.Key, alice.DelegationID), http.StatusForbidden, "no profile of the agent includes the user")
	// So is a code redeemed for her: the code is used up.
	f.call(http.StatusOK, http.MethodPut, "/api/agents/"+f.agent.ID, types.AgentIdentityManifest{
		DisplayName:        "assistant",
		Profiles:           []types.AgentProfile{{Name: "everyone", Subjects: everyUser()}},
		AllowedReturnHosts: f.agent.AllowedReturnHosts,
	}, nil)
	created := f.requestApproval(delegationSecretA)
	code := f.approve(f.alice, created.RequestID)
	f.call(http.StatusOK, http.MethodPut, "/api/agents/"+f.agent.ID, types.AgentIdentityManifest{
		DisplayName:        "assistant",
		Profiles:           []types.AgentProfile{{Name: "eng", Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}}},
		AllowedReturnHosts: f.agent.AllowedReturnHosts,
	}, nil)
	requireStatus(t, f.redeem(f.key.Key, created.RequestID, code, delegationSecretA), http.StatusForbidden, "no profile")
	f.call(http.StatusOK, http.MethodPut, "/api/agents/"+f.agent.ID, types.AgentIdentityManifest{
		DisplayName:        "assistant",
		Profiles:           []types.AgentProfile{{Name: "everyone", Subjects: everyUser()}},
		AllowedReturnHosts: f.agent.AllowedReturnHosts,
	}, nil)
	requireStatus(t, f.redeem(f.key.Key, created.RequestID, code, delegationSecretA), http.StatusBadRequest, "already used")
	decode[types.AgentDelegationToken](t, f.mint(f.key.Key, alice.DelegationID), http.StatusOK)

	// A revoked delegation is not found.
	require.Equal(t, http.StatusNoContent, f.asUser(f.admin, http.MethodDelete, "/api/agents/"+f.agent.ID+"/delegations/"+alice.DelegationID, nil).Code)
	requireStatus(t, f.mint(f.key.Key, alice.DelegationID), http.StatusNotFound, "not found")

	// A disabled agent is refused, even with a principal authenticated before it was disabled.
	req := newJSONRequest(t, http.MethodPost, "/api/agent-delegations/token", types.AgentDelegationTokenRequest{GrantType: types.AgentDelegationGrantDelegation, DelegationID: bob.DelegationID})
	req.Header.Set("Authorization", "Bearer "+f.key.Key)
	response, ok, err := gserver.NewAPIKeyAuthenticator(f.gateway).AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+f.agent.ID+"/disable", nil, nil)
	requireStatus(t, f.serve(response.User, req), http.StatusForbidden, "")
}

func TestAgentDelegationRevocationOverHTTP(t *testing.T) {
	f := newDelegationFixture(t)
	alice, bob := f.delegate(f.alice), f.delegate(f.bob)
	agentID := uintOf(t, f.agent.ID)

	// Neither can revoke the other's delegation, and an administrator revokes only through its agent.
	requireStatus(t, f.asUser(f.alice, http.MethodDelete, "/api/me/agent-delegations/"+bob.DelegationID, nil), http.StatusForbidden, "forbidden")
	other := f.createAgent(types.AgentIdentityManifest{DisplayName: "other"})
	requireStatus(t, f.asUser(f.admin, http.MethodDelete, "/api/agents/"+other.ID+"/delegations/"+bob.DelegationID, nil), http.StatusNotFound, "agent delegation not found")
	require.Empty(t, f.cleanup.scopes)

	// Alice revokes hers, which removes what she set up through the agent.
	rec := f.asUser(f.alice, http.MethodDelete, "/api/me/agent-delegations/"+alice.DelegationID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Equal(t, []agentdelegation.Scope{{AgentID: agentID, UserID: uintOf(t, f.alice.GetUID())}}, f.cleanup.scopes)
	requireStatus(t, f.mint(f.key.Key, alice.DelegationID), http.StatusNotFound, "not found")
	require.Empty(t, decode[types.AgentDelegationList](t, f.asUser(f.alice, http.MethodGet, "/api/me/agent-delegations", nil), http.StatusOK).Items)

	// An administrator revokes Bob's. A failure to remove his setup is reported, but the delegation is revoked.
	f.cleanup.err = errors.New("storage is down")
	rec = f.asUser(f.admin, http.MethodDelete, "/api/agents/"+f.agent.ID+"/delegations/"+bob.DelegationID, nil)
	requireStatus(t, rec, http.StatusInternalServerError, "the delegation was revoked")
	require.Equal(t, agentdelegation.Scope{AgentID: agentID, UserID: uintOf(t, f.bob.GetUID())}, f.cleanup.scopes[1])
	requireStatus(t, f.mint(f.key.Key, bob.DelegationID), http.StatusNotFound, "not found")
	require.Empty(t, decode[types.AgentDelegationList](t, f.asUser(f.admin, http.MethodGet, "/api/agents/"+f.agent.ID+"/delegations", nil), http.StatusOK).Items)

	// Both revocations are recorded, with who revoked.
	events, _, err := f.gateway.ListAdminAuditEvents(t.Context(), gclient.AdminAuditEventOptions{Actions: []string{gt.AdminAuditActionAgentDelegationRevoke}, Oldest: true})
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, []string{f.alice.GetUID(), f.admin.GetUID()}, []string{events[0].ActorUserID, events[1].ActorUserID})
	require.Equal(t, []string{alice.DelegationID, bob.DelegationID}, []string{events[0].ResourceID, events[1].ResourceID})
}

// TestAgentDelegationWithAuthenticationOff checks that without authentication, the "nobody" owner, who is every user,
// approves an agent through the same routes.
func TestAgentDelegationWithAuthenticationOff(t *testing.T) {
	f := newDelegationFixture(t)
	response, ok, err := authn.NewNoAuth(f.gateway).AuthenticateRequest(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	require.NoError(t, err)
	require.True(t, ok)
	nobody := response.User

	token := f.delegate(nobody)
	require.Equal(t, nobody.GetUID(), token.User.ID)
	mine := decode[types.AgentDelegationList](t, f.asUser(nobody, http.MethodGet, "/api/me/agent-delegations", nil), http.StatusOK)
	require.Len(t, mine.Items, 1)
	require.Equal(t, http.StatusNoContent, f.asUser(nobody, http.MethodDelete, "/api/me/agent-delegations/"+token.DelegationID, nil).Code)
}

func TestAgentAllowedReturnHostsOverHTTP(t *testing.T) {
	f := newAgentIdentityFixture(t)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "assistant", AllowedReturnHosts: []string{"Agent.Example.com", "localhost:3000"}})
	require.Equal(t, []string{"agent.example.com", "localhost:3000"}, agent.AllowedReturnHosts)

	var got types.AgentIdentity
	f.call(http.StatusOK, http.MethodGet, "/api/agents/"+agent.ID, nil, &got)
	require.Equal(t, agent.AllowedReturnHosts, got.AllowedReturnHosts)

	for _, hosts := range [][]string{{"https://agent.example.com"}, {"agent.example.com/callback"}, {"*.example.com"}, {"a.example.com", "A.example.com"}} {
		rec := f.do(http.MethodPut, "/api/agents/"+agent.ID, types.AgentIdentityManifest{DisplayName: "assistant", AllowedReturnHosts: hosts})
		require.Equal(t, http.StatusBadRequest, rec.Code, "%v: %s", hosts, rec.Body.String())
		require.Contains(t, rec.Body.String(), "allowedReturnHosts")
	}

	// An update replaces the list, so one that leaves it out clears it. A new value is decoded into, since decoding
	// into one that holds hosts would keep them.
	var cleared types.AgentIdentity
	f.call(http.StatusOK, http.MethodPut, "/api/agents/"+agent.ID, types.AgentIdentityManifest{DisplayName: "assistant"}, &cleared)
	require.Empty(t, cleared.AllowedReturnHosts)
}

// TestAgentDelegationRoutesAuthorization checks every delegation route against the server's authorizer, with the
// principals the server's authenticators build: the agent's routes are reached by an agent's own API key only, the
// approval routes by users' browser sessions, a delegation under /api/me by its own user only, and the routes under an
// agent by administrators, and reads by auditors.
func TestAgentDelegationRoutesAuthorization(t *testing.T) {
	f := newDelegationFixture(t)
	require.Len(t, f.routes, 9)
	alices := f.delegate(f.alice)
	admin := f.createUser("root", types.RoleAdmin)
	auditor := f.createUser("audit", types.RoleBasic|types.RoleAuditor)

	keyPrincipal := func(key string) user.Info {
		req := newJSONRequest(t, http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		response, ok, err := gserver.NewAPIKeyAuthenticator(f.gateway).AuthenticateRequest(req)
		require.NoError(t, err)
		require.True(t, ok)
		return response.User
	}
	agentKey := keyPrincipal(f.key.Key)
	// A key with only an MCP scope reaches the agent's routes too: every key of the agent can ask for approval.
	mcpOnly := f.createKey(f.agent.ID, types.AgentAPIKeyCreateRequest{Name: "mcp", MCPServerIDs: []string{"*"}})
	mcpOnlyKey := keyPrincipal(mcpOnly.Key)
	// The composite loopback's token carries the key of the connection it serves.
	composite := gclient.AgentUserInfo(&gt.Agent{ID: uintOf(t, f.agent.ID)}, []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, map[string][]string{
		"authorized_mcp_ids":         {"vmcp1"},
		principal.APIKeyIDExtra:      {strconv.FormatUint(uint64(f.key.ID), 10)},
		principal.APIKeyNameExtra:    {"web"},
		principal.APIKeyRawNameExtra: {"web"},
	})
	agentToken := gclient.AgentUserInfo(&gt.Agent{ID: uintOf(t, f.agent.ID)}, []string{types.GroupMCP, types.GroupAuthenticated}, map[string][]string{"authorized_mcp_ids": {"vmcp1"}})
	onBehalfOf := gclient.AgentUserInfo(&gt.Agent{ID: uintOf(t, f.agent.ID)}, agentKey.GetGroups(), maps.Clone(agentKey.GetExtra()))
	principal.RecordOnBehalfOf(onBehalfOf.GetExtra(), uintOf(t, f.alice.GetUID()))

	const (
		agentRoutes = "agent"
		approval    = "approval"
		meList      = "me list"
		meRevoke    = "me revoke"
		adminList   = "admin list"
		adminRevoke = "admin revoke"
	)
	kind := func(pattern string) string {
		switch {
		case strings.Contains(pattern, "/api/agent-delegations"):
			return agentRoutes
		case strings.Contains(pattern, "/api/agent-delegation-requests/"):
			return approval
		case pattern == "GET /api/me/agent-delegations":
			return meList
		case strings.HasPrefix(pattern, "DELETE /api/me/"):
			return meRevoke
		case strings.HasPrefix(pattern, "GET /api/agents/"):
			return adminList
		default:
			return adminRevoke
		}
	}
	for _, tt := range []struct {
		name  string
		user  user.Info
		key   string
		allow []string
	}{
		{
			name:  "an agent's key",
			user:  agentKey,
			key:   f.key.Key,
			allow: []string{agentRoutes},
		},
		{
			name:  "an agent's key with only an MCP scope",
			user:  mcpOnlyKey,
			key:   mcpOnly.Key,
			allow: []string{agentRoutes},
		},
		{
			name: "the agent's key with another of its keys presented",
			user: agentKey,
			key:  mcpOnly.Key,
		},
		{
			name: "the agent's composite loopback",
			user: composite,
			key:  f.key.Key,
		},
		{
			name: "a token issued to the agent",
			user: agentToken,
			key:  f.key.Key,
		},
		{
			name: "the agent acting on behalf of a user",
			user: onBehalfOf,
			key:  f.key.Key,
		},
		{
			name:  "the user of the delegation",
			user:  f.alice,
			allow: []string{approval, meList, meRevoke},
		},
		{
			name:  "another user",
			user:  f.bob,
			allow: []string{approval, meList},
		},
		{
			name:  "an administrator",
			user:  admin,
			allow: []string{approval, meList, adminList, adminRevoke},
		},
		{
			name:  "an auditor",
			user:  auditor,
			allow: []string{approval, meList, adminList},
		},
		{
			name: "unauthenticated",
			user: &user.DefaultInfo{Groups: []string{authz.UnauthenticatedGroup}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, pattern := range f.routes {
				method, path, _ := strings.Cut(pattern, " ")
				path = strings.NewReplacer("{agent_id}", f.agent.ID, "{request_id}", "adr1x", "{delegation_id}", alices.DelegationID).Replace(path)
				req := httptest.NewRequest(method, path, nil)
				if tt.key != "" {
					req.Header.Set("Authorization", "Bearer "+tt.key)
				}
				require.Equal(t, slices.Contains(tt.allow, kind(pattern)), f.authorizer.Authorize(req, tt.user), "%s %s", method, path)
			}
		})
	}
}
