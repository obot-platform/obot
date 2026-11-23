package client

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/require"
)

const (
	delegationTestReturnURL = "https://agent.example.com/callback?session=s1"
)

var (
	// The secrets of two browser sessions with the agent: A's, whose user asked for approval, and B's.
	delegationTestSecretA = strings.Repeat("a", 43)
	delegationTestSecretB = strings.Repeat("b", 43)
)

// delegationTestFixture is an agent with a key, whose profiles let it act for every user and whose return host is
// agent.example.com, and two users, Alice and Bob.
type delegationTestFixture struct {
	c     *Client
	agent *types.Agent
	keyID uint
	alice *types.User
	bob   *types.User
}

func newDelegationTestFixture(t *testing.T, c *Client) *delegationTestFixture {
	t.Helper()

	agent, err := c.CreateAgent(t.Context(), types.Agent{
		DisplayName:        "assistant",
		Profiles:           []apitypes.AgentProfile{{Name: "everyone", Subjects: []apitypes.Subject{{Type: apitypes.SubjectTypeSelector, ID: apitypes.SubjectWildcardID}}}},
		AllowedReturnHosts: []string{"agent.example.com"},
	})
	require.NoError(t, err)
	key, err := c.CreateAgentAPIKey(t.Context(), agent.ID, types.AgentAPIKeyCreateRequest{Name: "web", CanAccessLLMProxy: true})
	require.NoError(t, err)
	return &delegationTestFixture{
		c:     c,
		agent: agent,
		keyID: key.ID,
		alice: createLifecycleTestUser(t, c, "alice", lifecycleTestProvider),
		bob:   createLifecycleTestUser(t, c, "bob", lifecycleTestProvider),
	}
}

// s256 returns the secret hash an agent sends for secret.
func s256(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// asUser returns a user as an approver, in groups.
func asUser(u *types.User, groups ...string) subjects.Caller {
	authProvider := map[string]struct{}{}
	for _, group := range groups {
		authProvider[group] = struct{}{}
	}
	return subjects.Caller{Principal: principal.User(u.ID), Groups: subjects.Groups{AuthProvider: authProvider}}
}

// request creates a request of the fixture's agent from the browser session with secret.
func (f *delegationTestFixture) request(t *testing.T, secret string) *types.AgentDelegationRequest {
	t.Helper()
	request, err := f.c.CreateAgentDelegationRequest(t.Context(), AgentDelegationRequestInput{
		AgentID:    f.agent.ID,
		APIKeyID:   f.keyID,
		ReturnURL:  delegationTestReturnURL,
		SecretHash: s256(secret),
	})
	require.NoError(t, err)
	return request
}

// approve approves a request as u, and returns the approval and its code.
func (f *delegationTestFixture) approve(t *testing.T, requestID string, u *types.User) (*AgentDelegationApproval, string) {
	t.Helper()
	approval, code, err := f.c.ApproveAgentDelegationRequest(t.Context(), requestID, asUser(u))
	require.NoError(t, err)
	return approval, code
}

func (f *delegationTestFixture) storedRequest(t *testing.T, id string) *types.AgentDelegationRequest {
	t.Helper()
	request, err := getAgentDelegationRequest(f.c.db.WithContext(t.Context()), id, false)
	require.NoError(t, err)
	return request
}

func (f *delegationTestFixture) redeem(t *testing.T, requestID, code, secret string) (*types.AgentDelegation, error) {
	t.Helper()
	return f.c.RedeemAgentDelegationCode(t.Context(), f.agent.ID, f.keyID, requestID, code, secret)
}

// delegationAuditEvents returns the admin audit events with action, oldest first.
func delegationAuditEvents(t *testing.T, c *Client, action string) []types.AdminAuditEvent {
	t.Helper()
	events, _, err := c.ListAdminAuditEvents(t.Context(), AdminAuditEventOptions{Actions: []string{action}, Oldest: true})
	require.NoError(t, err)
	return events
}

func auditDetails(t *testing.T, event types.AdminAuditEvent) map[string]any {
	t.Helper()
	var details map[string]any
	require.NoError(t, json.Unmarshal(event.Details, &details))
	return details
}

func TestAgentDelegationRequestCreation(t *testing.T) {
	c := newTestClient(t)
	f := newDelegationTestFixture(t, c)
	ctx := t.Context()

	before := time.Now()
	request := f.request(t, delegationTestSecretA)
	require.True(t, strings.HasPrefix(request.ID, apitypes.AgentDelegationRequestIDPrefix), request.ID)
	stored := f.storedRequest(t, request.ID)
	require.Equal(t, types.AgentDelegationRequestPending, stored.State)
	require.Equal(t, s256(delegationTestSecretA), stored.SecretHash)
	require.Empty(t, stored.CodeHash)
	require.Zero(t, stored.ApprovedUserID)
	require.WithinDuration(t, before.Add(AgentDelegationRequestTTL), stored.ExpiresAt, 5*time.Second)

	// The agent is the actor, and only the return host is recorded.
	events := delegationAuditEvents(t, c, types.AdminAuditActionAgentDelegationRequestCreate)
	require.Len(t, events, 1)
	require.Equal(t, principal.Agent(f.agent.ID).String(), events[0].ActorUserID)
	require.Equal(t, types.AdminAuditResourceAgentDelegationRequest, events[0].ResourceKind)
	require.Equal(t, request.ID, events[0].ResourceID)
	require.Equal(t, strconv.FormatUint(uint64(f.agent.ID), 10), events[0].ParentID)
	require.Equal(t, map[string]any{"apiKeyID": float64(f.keyID), "returnHost": "agent.example.com", "hasLoginHint": false}, auditDetails(t, events[0]))
	require.NotContains(t, string(events[0].Details), "session=s1")
	require.Equal(t, apitypes.AuditLogActorTypeAgent, types.ConvertAdminAuditEvent(events[0], "").Actor.ActorType)

	other, err := c.CreateAgent(ctx, types.Agent{DisplayName: "other", AllowedReturnHosts: []string{"agent.example.com"}})
	require.NoError(t, err)
	approval, _ := f.approve(t, request.ID, f.alice)
	noHosts, err := c.CreateAgent(ctx, types.Agent{DisplayName: "no hosts"})
	require.NoError(t, err)

	valid := AgentDelegationRequestInput{
		AgentID:    f.agent.ID,
		APIKeyID:   f.keyID,
		ReturnURL:  delegationTestReturnURL,
		SecretHash: s256(delegationTestSecretA),
	}
	for _, tt := range []struct {
		name    string
		change  func(*AgentDelegationRequestInput)
		wantErr string
	}{
		{
			name:    "another host",
			change:  func(in *AgentDelegationRequestInput) { in.ReturnURL = "https://evil.example.com/callback" },
			wantErr: "not an allowed return host",
		},
		{
			name:    "http to a host that is not a loopback",
			change:  func(in *AgentDelegationRequestInput) { in.ReturnURL = "http://agent.example.com/callback" },
			wantErr: "must use https",
		},
		{
			name:    "an agent with no allowed hosts",
			change:  func(in *AgentDelegationRequestInput) { in.AgentID = noHosts.ID },
			wantErr: "no allowed return hosts",
		},
		{
			name:    "a malformed secret hash",
			change:  func(in *AgentDelegationRequestInput) { in.SecretHash = delegationTestSecretA },
			wantErr: "secretHash",
		},
		{
			name:    "a delegation of another agent",
			change:  func(in *AgentDelegationRequestInput) { in.AgentID = other.ID },
			wantErr: "is not a delegation of the agent",
		},
		{
			name:    "a delegation that does not exist",
			change:  func(in *AgentDelegationRequestInput) { in.DelegationID = "adl1missing" },
			wantErr: "is not a delegation of the agent",
		},
		{
			name:    "a login hint with control characters",
			change:  func(in *AgentDelegationRequestInput) { in.LoginHint = "alice\n" },
			wantErr: "loginHint",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := valid
			in.DelegationID = approval.Delegation.ID
			tt.change(&in)
			_, err := c.CreateAgentDelegationRequest(ctx, in)
			require.ErrorIs(t, err, ErrInvalidAgentDelegationRequest)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}

	// The agent's own delegation can be named.
	in := valid
	in.DelegationID = approval.Delegation.ID
	in.LoginHint = "alice@example.com"
	_, err = c.CreateAgentDelegationRequest(ctx, in)
	require.NoError(t, err)

	// A disabled or deleted agent asks for nothing.
	_, err = c.DisableAgent(ctx, f.agent.ID, "")
	require.NoError(t, err)
	_, err = c.CreateAgentDelegationRequest(ctx, valid)
	denied, ok := errors.AsType[*AgentAccessDeniedError](err)
	require.True(t, ok, "%v", err)
	require.Equal(t, apitypes.UserStatusDisabled, denied.Status)
	require.NoError(t, c.DeleteAgent(ctx, f.agent.ID))
	_, err = c.CreateAgentDelegationRequest(ctx, valid)
	denied, ok = errors.AsType[*AgentAccessDeniedError](err)
	require.True(t, ok, "%v", err)
	require.Equal(t, apitypes.UserStatusDeleted, denied.Status)
}

func TestAgentDelegationRequestLimit(t *testing.T) {
	c := newTestClient(t)
	f := newDelegationTestFixture(t, c)

	var requests []*types.AgentDelegationRequest
	for range MaxPendingAgentDelegationRequests {
		requests = append(requests, f.request(t, delegationTestSecretA))
	}
	in := AgentDelegationRequestInput{AgentID: f.agent.ID, APIKeyID: f.keyID, ReturnURL: delegationTestReturnURL, SecretHash: s256(delegationTestSecretA)}
	_, err := c.CreateAgentDelegationRequest(t.Context(), in)
	require.ErrorIs(t, err, ErrAgentDelegationRequestLimit)

	// Another agent has its own limit.
	other, err := c.CreateAgent(t.Context(), types.Agent{DisplayName: "other", AllowedReturnHosts: []string{"agent.example.com"}})
	require.NoError(t, err)
	otherIn := in
	otherIn.AgentID = other.ID
	_, err = c.CreateAgentDelegationRequest(t.Context(), otherIn)
	require.NoError(t, err)

	// An expired request no longer counts, nor does one that was approved.
	require.NoError(t, c.db.WithContext(t.Context()).Model(&types.AgentDelegationRequest{}).Where("id = ?", requests[0].ID).
		Update("expires_at", time.Now().Add(-time.Second)).Error)
	_, err = c.CreateAgentDelegationRequest(t.Context(), in)
	require.NoError(t, err)
	_, err = c.CreateAgentDelegationRequest(t.Context(), in)
	require.ErrorIs(t, err, ErrAgentDelegationRequestLimit)
	f.approve(t, requests[1].ID, f.alice)
	_, err = c.CreateAgentDelegationRequest(t.Context(), in)
	require.NoError(t, err)
}

// TestForwardedApprovalLinkCannotBeRedeemed is the attack Arcade calls COAT: a user of the agent forwards the approval
// link of a request they started to a victim, who approves it. The victim's browser comes back to the agent with the
// code, in the victim's session, which does not hold the secret of the session that asked. The code cannot be
// redeemed with that session's secret, which uses it up, so the one who forwarded the link cannot redeem it either.
func TestForwardedApprovalLinkCannotBeRedeemed(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			c := database.open(t)
			f := newDelegationTestFixture(t, c)

			// Alice's session with the agent asks, and Bob approves the forwarded link.
			request := f.request(t, delegationTestSecretA)
			approval, code := f.approve(t, request.ID, f.bob)
			require.Equal(t, f.bob.ID, approval.Delegation.UserID)

			// The code came back in Bob's session, which has its own secret, or none.
			for _, secret := range []string{delegationTestSecretB, ""} {
				_, err := f.redeem(t, request.ID, code, secret)
				require.ErrorIs(t, err, ErrInvalidAgentDelegationGrant)
			}
			stored := f.storedRequest(t, request.ID)
			require.Equal(t, types.AgentDelegationRequestRedeemed, stored.State)
			require.Empty(t, stored.CodeHash)

			// The wrong secret used the code up, so Alice's secret cannot redeem it now.
			_, err := f.redeem(t, request.ID, code, delegationTestSecretA)
			require.ErrorIs(t, err, ErrInvalidAgentDelegationGrant)
			require.ErrorContains(t, err, "already used")

			// The refusal is recorded, with the agent as the actor.
			events := delegationAuditEvents(t, c, types.AdminAuditActionAgentDelegationRequestRedeem)
			require.Len(t, events, 1)
			require.Equal(t, principal.Agent(f.agent.ID).String(), events[0].ActorUserID)
			details := auditDetails(t, events[0])
			require.Equal(t, false, details["redeemed"])
			require.Equal(t, "secret_mismatch", details["refusal"])

			// The same approval in the session that asked is redeemed for the user who approved it.
			request = f.request(t, delegationTestSecretA)
			approval, code = f.approve(t, request.ID, f.bob)
			delegation, err := f.redeem(t, request.ID, code, delegationTestSecretA)
			require.NoError(t, err)
			require.Equal(t, approval.Delegation.ID, delegation.ID)
			require.Equal(t, f.bob.ID, delegation.UserID)
			require.NotNil(t, delegation.LastUsedAt)
		})
	}
}

func TestAgentDelegationCodes(t *testing.T) {
	c := newTestClient(t)
	f := newDelegationTestFixture(t, c)
	ctx := t.Context()

	// A code is redeemed once.
	request := f.request(t, delegationTestSecretA)
	approval, code := f.approve(t, request.ID, f.alice)
	stored := f.storedRequest(t, request.ID)
	require.NotEqual(t, code, stored.CodeHash, "only the code's digest is stored")
	require.NotContains(t, stored.CodeHash, code)
	require.WithinDuration(t, stored.ApprovedAt.Add(AgentDelegationCodeTTL), *stored.CodeExpiresAt, time.Second)
	delegation, err := f.redeem(t, request.ID, code, delegationTestSecretA)
	require.NoError(t, err)
	require.Equal(t, approval.Delegation.ID, delegation.ID)
	_, err = f.redeem(t, request.ID, code, delegationTestSecretA)
	require.ErrorContains(t, err, "already used")

	events := delegationAuditEvents(t, c, types.AdminAuditActionAgentDelegationRequestRedeem)
	require.Len(t, events, 1)
	require.Equal(t, principal.Agent(f.agent.ID).String(), events[0].ActorUserID)
	require.Equal(t, true, auditDetails(t, events[0])["redeemed"])

	// A code expires after two minutes, and is refused without being used up.
	request = f.request(t, delegationTestSecretA)
	_, code = f.approve(t, request.ID, f.alice)
	require.NoError(t, c.db.WithContext(ctx).Model(&types.AgentDelegationRequest{}).Where("id = ?", request.ID).
		Update("code_expires_at", time.Now().Add(-time.Second)).Error)
	_, err = f.redeem(t, request.ID, code, delegationTestSecretA)
	require.ErrorIs(t, err, ErrInvalidAgentDelegationGrant)
	require.ErrorContains(t, err, "expired")

	// Another agent cannot redeem a code, even with the right secret: the request is unknown to it, and the code is
	// left for its agent.
	other, err := c.CreateAgent(ctx, types.Agent{DisplayName: "other", AllowedReturnHosts: []string{"agent.example.com"}})
	require.NoError(t, err)
	otherKey, err := c.CreateAgentAPIKey(ctx, other.ID, types.AgentAPIKeyCreateRequest{Name: "web", CanAccessLLMProxy: true})
	require.NoError(t, err)
	request = f.request(t, delegationTestSecretA)
	_, code = f.approve(t, request.ID, f.alice)
	_, err = c.RedeemAgentDelegationCode(ctx, other.ID, otherKey.ID, request.ID, code, delegationTestSecretA)
	require.ErrorIs(t, err, ErrInvalidAgentDelegationGrant)
	require.ErrorContains(t, err, "unknown request")

	// A wrong code is refused, and leaves the right one.
	_, err = f.redeem(t, request.ID, "not-the-code", delegationTestSecretA)
	require.ErrorContains(t, err, "not the request's")
	_, err = f.redeem(t, request.ID, code, delegationTestSecretA)
	require.NoError(t, err)

	// A request that was not approved, or was cancelled, has no code to redeem.
	request = f.request(t, delegationTestSecretA)
	_, err = f.redeem(t, request.ID, "anything", delegationTestSecretA)
	require.ErrorContains(t, err, "not been approved")
	_, err = c.CancelAgentDelegationRequest(ctx, request.ID, asUser(f.alice))
	require.NoError(t, err)
	_, err = f.redeem(t, request.ID, "anything", delegationTestSecretA)
	require.ErrorContains(t, err, "cancelled")
	_, err = f.redeem(t, "adr1missing", "anything", delegationTestSecretA)
	require.ErrorContains(t, err, "unknown request")
}

func TestAgentDelegationApproval(t *testing.T) {
	c := newTestClient(t)
	f := newDelegationTestFixture(t, c)
	ctx := t.Context()

	// Only users that a profile matches may approve: here, those in the eng group.
	_, err := c.UpdateAgent(ctx, types.Agent{
		ID:                 f.agent.ID,
		DisplayName:        f.agent.DisplayName,
		Profiles:           []apitypes.AgentProfile{{Name: "eng", Subjects: []apitypes.Subject{{Type: apitypes.SubjectTypeGroup, ID: "eng"}}}},
		AllowedReturnHosts: f.agent.AllowedReturnHosts,
	})
	require.NoError(t, err)
	request := f.request(t, delegationTestSecretA)
	_, err = c.GetAgentDelegationApproval(ctx, request.ID, asUser(f.alice))
	require.ErrorIs(t, err, ErrAgentDelegationNoProfile)
	_, _, err = c.ApproveAgentDelegationRequest(ctx, request.ID, asUser(f.alice))
	require.ErrorIs(t, err, ErrAgentDelegationNoProfile)
	_, err = c.CancelAgentDelegationRequest(ctx, request.ID, asUser(f.alice))
	require.ErrorIs(t, err, ErrAgentDelegationNoProfile)
	// An agent is never acted for, whatever it names.
	_, err = c.GetAgentDelegationApproval(ctx, request.ID, subjects.Caller{Principal: principal.Agent(f.agent.ID)})
	require.ErrorIs(t, err, ErrAgentDelegationNoProfile)

	view, err := c.GetAgentDelegationApproval(ctx, request.ID, asUser(f.alice, "eng"))
	require.NoError(t, err)
	require.Equal(t, request.ID, view.Request.ID)
	require.Nil(t, view.Delegation)
	first, _, err := c.ApproveAgentDelegationRequest(ctx, request.ID, asUser(f.alice, "eng"))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(first.Delegation.ID, apitypes.AgentDelegationIDPrefix), first.Delegation.ID)
	// A delegation is found through its own agent only.
	_, err = c.GetAgentDelegationOfAgent(ctx, first.Delegation.ID, f.agent.ID)
	require.NoError(t, err)
	_, err = c.GetAgentDelegationOfAgent(ctx, first.Delegation.ID, f.agent.ID+1)
	require.ErrorIs(t, err, ErrAgentDelegationNotFound)

	// A used request is refused, approved or cancelled, and so is an expired one.
	_, _, err = c.ApproveAgentDelegationRequest(ctx, request.ID, asUser(f.alice, "eng"))
	require.ErrorIs(t, err, ErrAgentDelegationRequestUnusable)
	cancelled := f.request(t, delegationTestSecretA)
	_, err = c.CancelAgentDelegationRequest(ctx, cancelled.ID, asUser(f.alice, "eng"))
	require.NoError(t, err)
	require.Equal(t, types.AgentDelegationRequestCancelled, f.storedRequest(t, cancelled.ID).State)
	_, _, err = c.ApproveAgentDelegationRequest(ctx, cancelled.ID, asUser(f.alice, "eng"))
	require.ErrorIs(t, err, ErrAgentDelegationRequestUnusable)
	expired := f.request(t, delegationTestSecretA)
	require.NoError(t, c.db.WithContext(ctx).Model(&types.AgentDelegationRequest{}).Where("id = ?", expired.ID).
		Update("expires_at", time.Now().Add(-time.Second)).Error)
	_, err = c.GetAgentDelegationApproval(ctx, expired.ID, asUser(f.alice, "eng"))
	require.ErrorIs(t, err, ErrAgentDelegationRequestUnusable)
	_, _, err = c.ApproveAgentDelegationRequest(ctx, expired.ID, asUser(f.alice, "eng"))
	require.ErrorIs(t, err, ErrAgentDelegationRequestUnusable)
	_, err = c.GetAgentDelegationApproval(ctx, "adr1missing", asUser(f.alice, "eng"))
	require.ErrorIs(t, err, ErrAgentDelegationRequestNotFound)

	// Approving again keeps the delegation and its ID, and moves its last approval.
	time.Sleep(10 * time.Millisecond)
	again := f.request(t, delegationTestSecretA)
	view, err = c.GetAgentDelegationApproval(ctx, again.ID, asUser(f.alice, "eng"))
	require.NoError(t, err)
	require.Equal(t, first.Delegation.ID, view.Delegation.ID)
	second, _, err := c.ApproveAgentDelegationRequest(ctx, again.ID, asUser(f.alice, "eng"))
	require.NoError(t, err)
	require.Equal(t, first.Delegation.ID, second.Delegation.ID)
	stored, err := c.GetAgentDelegation(ctx, first.Delegation.ID)
	require.NoError(t, err)
	require.WithinDuration(t, first.Delegation.FirstApprovedAt, stored.FirstApprovedAt, time.Millisecond)
	require.True(t, stored.LastApprovedAt.After(stored.FirstApprovedAt), "%v is after %v", stored.LastApprovedAt, stored.FirstApprovedAt)
	var count int64
	require.NoError(t, c.db.WithContext(ctx).Model(&types.AgentDelegation{}).Count(&count).Error)
	require.EqualValues(t, 1, count)

	// The user is the actor of each approval.
	events := delegationAuditEvents(t, c, types.AdminAuditActionAgentDelegationApprove)
	require.Len(t, events, 2)
	for i, event := range events {
		require.Equal(t, principal.User(f.alice.ID).String(), event.ActorUserID)
		require.Equal(t, types.AdminAuditResourceAgentDelegation, event.ResourceKind)
		require.Equal(t, first.Delegation.ID, event.ResourceID)
		require.Equal(t, i == 0, auditDetails(t, event)["created"])
	}

	// A request to approve Alice's delegation again is hers to approve, not Bob's.
	_, err = c.UpdateAgent(ctx, types.Agent{
		ID:                 f.agent.ID,
		DisplayName:        f.agent.DisplayName,
		Profiles:           []apitypes.AgentProfile{{Name: "everyone", Subjects: []apitypes.Subject{{Type: apitypes.SubjectTypeSelector, ID: apitypes.SubjectWildcardID}}}},
		AllowedReturnHosts: f.agent.AllowedReturnHosts,
	})
	require.NoError(t, err)
	reapproval, err := c.CreateAgentDelegationRequest(ctx, AgentDelegationRequestInput{
		AgentID: f.agent.ID, APIKeyID: f.keyID, ReturnURL: delegationTestReturnURL, SecretHash: s256(delegationTestSecretA), DelegationID: first.Delegation.ID,
	})
	require.NoError(t, err)
	_, _, err = c.ApproveAgentDelegationRequest(ctx, reapproval.ID, asUser(f.bob))
	require.ErrorIs(t, err, ErrAgentDelegationOtherUser)
	approval, _ := f.approve(t, reapproval.ID, f.alice)
	require.Equal(t, first.Delegation.ID, approval.Delegation.ID)

	// A return host an administrator removed is no longer sent to.
	pending := f.request(t, delegationTestSecretA)
	_, err = c.UpdateAgent(ctx, types.Agent{
		ID:                 f.agent.ID,
		DisplayName:        f.agent.DisplayName,
		Profiles:           storedAgent(t, c, f.agent.ID).Profiles,
		AllowedReturnHosts: []string{"other.example.com"},
	})
	require.NoError(t, err)
	_, _, err = c.ApproveAgentDelegationRequest(ctx, pending.ID, asUser(f.alice))
	require.ErrorIs(t, err, ErrAgentDelegationReturnURLNotAllowed)
	_, err = c.CancelAgentDelegationRequest(ctx, pending.ID, asUser(f.alice))
	require.ErrorIs(t, err, ErrAgentDelegationReturnURLNotAllowed)

	// A disabled agent can't be approved.
	_, err = c.UpdateAgent(ctx, types.Agent{
		ID:                 f.agent.ID,
		DisplayName:        f.agent.DisplayName,
		Profiles:           storedAgent(t, c, f.agent.ID).Profiles,
		AllowedReturnHosts: []string{"agent.example.com"},
	})
	require.NoError(t, err)
	_, err = c.DisableAgent(ctx, f.agent.ID, "")
	require.NoError(t, err)
	_, err = c.GetAgentDelegationApproval(ctx, pending.ID, asUser(f.alice))
	_, ok := errors.AsType[*AgentAccessDeniedError](err)
	require.True(t, ok, "%v", err)
	_, _, err = c.ApproveAgentDelegationRequest(ctx, pending.ID, asUser(f.alice))
	_, ok = errors.AsType[*AgentAccessDeniedError](err)
	require.True(t, ok, "%v", err)
}

func TestDelegationValid(t *testing.T) {
	c := newTestClient(t)
	c.agentDelegationCacheTTL = agentDelegationValidationCacheTTL
	f := newDelegationTestFixture(t, c)
	ctx := t.Context()

	approve := func(u *types.User) *types.AgentDelegation {
		request := f.request(t, delegationTestSecretA)
		approval, _ := f.approve(t, request.ID, u)
		return approval.Delegation
	}
	alice, bob := approve(f.alice), approve(f.bob)
	valid := func(delegation *types.AgentDelegation, agentID, userID uint) bool {
		ok, err := c.DelegationValid(ctx, delegation.ID, agentID, userID)
		require.NoError(t, err)
		return ok
	}

	require.True(t, valid(alice, f.agent.ID, f.alice.ID))
	// The delegation, the agent and the user must agree.
	require.False(t, valid(alice, f.agent.ID, f.bob.ID))
	require.False(t, valid(alice, f.agent.ID+1, f.alice.ID))
	require.False(t, valid(&types.AgentDelegation{ID: "adl1missing"}, f.agent.ID, f.alice.ID))

	// A revocation elsewhere, which deletes the row, applies once the cached validation expires.
	require.True(t, valid(bob, f.agent.ID, f.bob.ID))
	require.NoError(t, c.db.WithContext(ctx).Where("id = ?", bob.ID).Delete(&types.AgentDelegation{}).Error)
	require.True(t, valid(bob, f.agent.ID, f.bob.ID), "cached")
	c.agentDelegationCacheLock.Lock()
	for key := range c.agentDelegationCache {
		c.agentDelegationCache[key] = time.Now().Add(-time.Second)
	}
	c.agentDelegationCacheLock.Unlock()
	require.False(t, valid(bob, f.agent.ID, f.bob.ID))

	// A revocation here applies at once.
	require.True(t, valid(alice, f.agent.ID, f.alice.ID))
	_, err := c.RevokeAgentDelegation(ctx, alice.ID, principal.User(f.alice.ID).String())
	require.NoError(t, err)
	require.False(t, valid(alice, f.agent.ID, f.alice.ID))

	// Disabling the user or the agent refuses the delegation at once here, and deletes nothing.
	alice = approve(f.alice)
	require.True(t, valid(alice, f.agent.ID, f.alice.ID))
	_, err = disableUser(t, c, lifecycleTestProvider, f.alice.ID, types.UserDisabledReasonSCIMInactive)
	require.NoError(t, err)
	require.False(t, valid(alice, f.agent.ID, f.alice.ID))
	_, err = c.GetAgentDelegation(ctx, alice.ID)
	require.NoError(t, err)

	bob = approve(f.bob)
	require.True(t, valid(bob, f.agent.ID, f.bob.ID))
	_, err = c.DisableAgent(ctx, f.agent.ID, "")
	require.NoError(t, err)
	require.False(t, valid(bob, f.agent.ID, f.bob.ID))
	_, err = c.EnableAgent(ctx, f.agent.ID)
	require.NoError(t, err)
	require.True(t, valid(bob, f.agent.ID, f.bob.ID))

	// A deleted user or agent is refused.
	require.NoError(t, c.DeleteUser(ctx, strconv.FormatUint(uint64(f.bob.ID), 10)))
	require.False(t, valid(bob, f.agent.ID, f.bob.ID))
}

func TestRevokeAgentDelegation(t *testing.T) {
	c := newTestClient(t)
	f := newDelegationTestFixture(t, c)
	ctx := t.Context()

	request := f.request(t, delegationTestSecretA)
	approval, _ := f.approve(t, request.ID, f.alice)
	delegation := approval.Delegation
	// An approved request whose code was not redeemed yet, a pending request to approve the delegation again, a
	// pending request no one approved yet, and Bob's approved request.
	unredeemed := f.request(t, delegationTestSecretA)
	_, unredeemedCode := f.approve(t, unredeemed.ID, f.alice)
	reapproval, err := c.CreateAgentDelegationRequest(ctx, AgentDelegationRequestInput{
		AgentID: f.agent.ID, APIKeyID: f.keyID, ReturnURL: delegationTestReturnURL, SecretHash: s256(delegationTestSecretA), DelegationID: delegation.ID,
	})
	require.NoError(t, err)
	unapproved := f.request(t, delegationTestSecretA)
	bobs := f.request(t, delegationTestSecretA)
	f.approve(t, bobs.ID, f.bob)

	// An administrator, who is not the user, revokes it.
	revoked, err := c.RevokeAgentDelegation(ctx, delegation.ID, "999")
	require.NoError(t, err)
	require.Equal(t, delegation.ID, revoked.ID)
	_, err = c.GetAgentDelegation(ctx, delegation.ID)
	require.ErrorIs(t, err, ErrAgentDelegationNotFound)

	for _, id := range []string{request.ID, unredeemed.ID, reapproval.ID} {
		_, err := getAgentDelegationRequest(c.db.WithContext(ctx), id, false)
		require.ErrorIs(t, err, ErrAgentDelegationRequestNotFound, id)
	}
	for _, id := range []string{unapproved.ID, bobs.ID} {
		f.storedRequest(t, id)
	}
	// The code of the approval that was not redeemed is gone with it.
	_, err = f.redeem(t, unredeemed.ID, unredeemedCode, delegationTestSecretA)
	require.ErrorContains(t, err, "unknown request")

	events := delegationAuditEvents(t, c, types.AdminAuditActionAgentDelegationRevoke)
	require.Len(t, events, 1)
	require.Equal(t, "999", events[0].ActorUserID)
	require.Equal(t, delegation.ID, events[0].ResourceID)
	require.Equal(t, strconv.FormatUint(uint64(f.agent.ID), 10), events[0].ParentID)
	require.Equal(t, false, auditDetails(t, events[0])["revokedByTheUser"])

	_, err = c.RevokeAgentDelegation(ctx, delegation.ID, "999")
	require.ErrorIs(t, err, ErrAgentDelegationNotFound)
}

func TestAgentDelegationsOfDeletedAgentsAndUsers(t *testing.T) {
	c := newTestClient(t)
	f := newDelegationTestFixture(t, c)
	ctx := t.Context()
	other, err := c.CreateAgent(ctx, types.Agent{
		DisplayName:        "other",
		Profiles:           []apitypes.AgentProfile{{Name: "everyone", Subjects: []apitypes.Subject{{Type: apitypes.SubjectTypeSelector, ID: apitypes.SubjectWildcardID}}}},
		AllowedReturnHosts: []string{"agent.example.com"},
	})
	require.NoError(t, err)

	approve := func(agentID uint, u *types.User) string {
		request, err := c.CreateAgentDelegationRequest(ctx, AgentDelegationRequestInput{AgentID: agentID, APIKeyID: f.keyID, ReturnURL: delegationTestReturnURL, SecretHash: s256(delegationTestSecretA)})
		require.NoError(t, err)
		approval, _, err := c.ApproveAgentDelegationRequest(ctx, request.ID, asUser(u))
		require.NoError(t, err)
		return approval.Delegation.ID
	}
	agentAlice, agentBob := approve(f.agent.ID, f.alice), approve(f.agent.ID, f.bob)
	otherAlice, otherBob := approve(other.ID, f.alice), approve(other.ID, f.bob)
	pending := f.request(t, delegationTestSecretA)

	list := func(delegations []types.AgentDelegation, err error) []string {
		require.NoError(t, err)
		var ids []string
		for _, delegation := range delegations {
			ids = append(ids, delegation.ID)
		}
		return ids
	}
	require.Equal(t, []string{agentAlice, agentBob}, list(c.ListAgentDelegations(ctx, f.agent.ID)))
	require.Equal(t, []string{agentAlice, otherAlice}, list(c.ListUserAgentDelegations(ctx, f.alice.ID)))

	// A deleted user's and agent's delegations are not listed while their cleanup is pending.
	require.NoError(t, c.DeleteUser(ctx, strconv.FormatUint(uint64(f.bob.ID), 10)))
	require.Equal(t, []string{agentAlice}, list(c.ListAgentDelegations(ctx, f.agent.ID)))
	require.NoError(t, c.DeleteAgent(ctx, other.ID))
	require.Equal(t, []string{agentAlice}, list(c.ListUserAgentDelegations(ctx, f.alice.ID)))

	// The cleanup of the user deletes their delegations, and only theirs, and the requests they approved. Running it
	// again deletes nothing.
	deleted, err := c.DeleteAgentDelegationsOfUser(ctx, f.bob.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)
	deleted, err = c.DeleteAgentDelegationsOfUser(ctx, f.bob.ID)
	require.NoError(t, err)
	require.Zero(t, deleted)
	for _, id := range []string{agentBob, otherBob} {
		_, err := c.GetAgentDelegation(ctx, id)
		require.ErrorIs(t, err, ErrAgentDelegationNotFound)
	}
	var approvedByBob int64
	require.NoError(t, c.db.WithContext(ctx).Model(&types.AgentDelegationRequest{}).Where("approved_user_id = ?", f.bob.ID).Count(&approvedByBob).Error)
	require.Zero(t, approvedByBob)

	// The cleanup of the agent deletes its delegations and requests, and only its.
	deleted, err = c.DeleteAgentDelegationsOfAgent(ctx, other.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	_, err = c.GetAgentDelegation(ctx, otherAlice)
	require.ErrorIs(t, err, ErrAgentDelegationNotFound)
	var otherRequests int64
	require.NoError(t, c.db.WithContext(ctx).Model(&types.AgentDelegationRequest{}).Where("agent_id = ?", other.ID).Count(&otherRequests).Error)
	require.Zero(t, otherRequests)
	_, err = c.GetAgentDelegation(ctx, agentAlice)
	require.NoError(t, err)
	f.storedRequest(t, pending.ID)
}

func TestDeleteExpiredAgentDelegationRequests(t *testing.T) {
	c := newTestClient(t)
	f := newDelegationTestFixture(t, c)
	ctx := t.Context()

	old, recent, current := f.request(t, delegationTestSecretA), f.request(t, delegationTestSecretA), f.request(t, delegationTestSecretA)
	now := time.Now()
	// One expired longer ago than a code lasts, and one more recently, which may still hold a code issued before it
	// expired.
	require.NoError(t, c.db.WithContext(ctx).Model(&types.AgentDelegationRequest{}).Where("id = ?", old.ID).
		Update("expires_at", now.Add(-AgentDelegationCodeTTL-time.Minute)).Error)
	require.NoError(t, c.db.WithContext(ctx).Model(&types.AgentDelegationRequest{}).Where("id = ?", recent.ID).
		Update("expires_at", now.Add(-time.Minute)).Error)

	deleted, err := c.DeleteExpiredAgentDelegationRequests(ctx, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	_, err = getAgentDelegationRequest(c.db.WithContext(ctx), old.ID, false)
	require.ErrorIs(t, err, ErrAgentDelegationRequestNotFound)
	f.storedRequest(t, recent.ID)
	f.storedRequest(t, current.ID)
}

func TestAgentAllowedReturnHosts(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	_, err := c.CreateAgent(ctx, types.Agent{DisplayName: "bad", AllowedReturnHosts: []string{"https://agent.example.com"}})
	require.ErrorIs(t, err, ErrInvalidAgentMetadata)

	agent, err := c.CreateAgent(ctx, types.Agent{DisplayName: "assistant", AllowedReturnHosts: []string{"Agent.Example.com", "localhost:3000"}})
	require.NoError(t, err)
	require.Equal(t, []string{"agent.example.com", "localhost:3000"}, storedAgent(t, c, agent.ID).AllowedReturnHosts)

	// Saving the same hosts changes nothing, and changing them records both lists.
	_, err = c.UpdateAgent(WithAdminActor(ctx, "1"), types.Agent{ID: agent.ID, DisplayName: "assistant", AllowedReturnHosts: []string{"agent.example.com", "localhost:3000"}})
	require.NoError(t, err)
	require.Empty(t, delegationAuditEvents(t, c, types.AdminAuditActionAgentUpdate))
	_, err = c.UpdateAgent(WithAdminActor(ctx, "1"), types.Agent{ID: agent.ID, DisplayName: "assistant", AllowedReturnHosts: []string{"agent.example.com"}})
	require.NoError(t, err)
	require.Equal(t, []string{"agent.example.com"}, storedAgent(t, c, agent.ID).AllowedReturnHosts)
	events := delegationAuditEvents(t, c, types.AdminAuditActionAgentUpdate)
	require.Len(t, events, 1)
	details := auditDetails(t, events[0])
	require.Equal(t, []any{types.AdminAuditAgentAllowedReturnHosts}, details["changedFields"])
	require.Equal(t, map[string]any{
		"before": []any{"agent.example.com", "localhost:3000"},
		"after":  []any{"agent.example.com"},
	}, details[types.AdminAuditAgentAllowedReturnHosts])

	// An update that leaves them out removes them all.
	_, err = c.UpdateAgent(ctx, types.Agent{ID: agent.ID, DisplayName: "assistant"})
	require.NoError(t, err)
	require.Empty(t, storedAgent(t, c, agent.ID).AllowedReturnHosts)
	_, err = c.UpdateAgent(ctx, types.Agent{ID: agent.ID, DisplayName: "assistant", AllowedReturnHosts: []string{"a.example.com", "A.example.com"}})
	require.ErrorIs(t, err, ErrInvalidAgentMetadata)
}
