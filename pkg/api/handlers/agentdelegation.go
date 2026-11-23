package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentdelegation"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"gorm.io/gorm"
)

const (
	// agentApprovalPath is the UI page at which a user approves an agent's request (approvalURL).
	agentApprovalPath = "/agents/approve/"
	// agentApprovalLoginHintParam carries an agent's login hint on the approval page's URL, for the page to pass on to
	// sign-in.
	agentApprovalLoginHintParam = "login_hint"
)

// DelegatedTokenIssuer mints the tokens with which an agent acts on behalf of a user (persistent.TokenService).
type DelegatedTokenIssuer interface {
	NewDelegatedToken(ctx context.Context, agentID, userID uint, delegationID string, apiKeyID uint, ttl time.Duration) (string, time.Time, error)
}

// AgentDelegationHandler serves the approval protocol with which a user lets an agent act on their behalf
// (adr/2026-11-20-agent-on-behalf-of-users.md, "Approval and tokens"):
//
//   - The agent, with its own API key and never any other credential, asks for an approval link
//     (POST /api/agent-delegations), and later gets tokens (POST /api/agent-delegations/token).
//   - The signed-in user, in their browser, reads, approves or cancels the request
//     (/api/agent-delegation-requests/{request_id}).
//   - The user lists and revokes their delegations (/api/me/agent-delegations), and administrators an agent's
//     (/api/agents/{agent_id}/delegations).
//
// The binding of an approval to the browser session that asked for it is the secret: the agent keeps it in the user's
// session with the agent, and gives Obot only its hash. The code an approval issues goes back to whichever browser
// approved, and only the agent with that session's secret can redeem it, so an approval link forwarded to someone else
// is useless to whoever forwarded it.
//
// Who may call each route is the authorizer's (pkg/api/authz): the agent's own key on the agent's routes, a user's
// browser session, from Obot's own pages, on the approval routes, the delegation's own user on a delegation under
// /api/me, and administrators under /api/agents. The handler reads the caller the authorizer admitted. What it checks
// is what depends on the data a route names and must hold when it is written: whether a user may approve a given
// request, and whether a code or delegation named in a body is the calling agent's.
type AgentDelegationHandler struct {
	tokens    DelegatedTokenIssuer
	cleanup   agentdelegation.Cleanup
	serverURL string
}

func NewAgentDelegationHandler(tokens DelegatedTokenIssuer, cleanup agentdelegation.Cleanup, serverURL string) *AgentDelegationHandler {
	if cleanup == nil {
		cleanup = agentdelegation.NoCleanup{}
	}
	return &AgentDelegationHandler{
		tokens:    tokens,
		cleanup:   cleanup,
		serverURL: strings.TrimSuffix(serverURL, "/"),
	}
}

// RegisterRoutes registers the routes with handle. Their authorization is in pkg/api/authz: the agent's routes are
// on the allowlist of agents for an agent's own API key only (agentKeyRules), the approval routes are a user's browser
// session's (authorizeAgentDelegationApproval), the "me" routes are users', each delegation its own user's
// (checkAgentDelegation), and the routes under /api/agents are administrators'.
func (h *AgentDelegationHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("POST /api/agent-delegations", h.CreateRequest)
	handle("POST /api/agent-delegations/token", h.Token)

	handle("GET /api/agent-delegation-requests/{request_id}", h.GetRequest)
	handle("POST /api/agent-delegation-requests/{request_id}/approve", h.Approve)
	handle("POST /api/agent-delegation-requests/{request_id}/cancel", h.Cancel)

	handle("GET /api/me/agent-delegations", h.ListMine)
	handle("DELETE /api/me/agent-delegations/{delegation_id}", h.RevokeMine)

	handle("GET /api/agents/{agent_id}/delegations", h.ListForAgent)
	handle("DELETE /api/agents/{agent_id}/delegations/{delegation_id}", h.RevokeForAgent)
}

// CreateRequest records an agent's request for a user's approval, and returns the link the user opens to approve it.
func (h *AgentDelegationHandler) CreateRequest(req api.Context) error {
	agentID, keyID, err := agentCaller(req)
	if err != nil {
		return err
	}
	var body types.AgentDelegationRequestCreate
	if err := readAgentIdentityRequest(req, &body, false); err != nil {
		return err
	}

	request, err := req.GatewayClient.CreateAgentDelegationRequest(req.Context(), gclient.AgentDelegationRequestInput{
		AgentID:      agentID,
		APIKeyID:     keyID,
		ReturnURL:    body.ReturnURL,
		SecretHash:   body.SecretHash,
		LoginHint:    body.LoginHint,
		DelegationID: body.DelegationID,
	})
	if err != nil {
		return agentDelegationError(err)
	}
	return req.WriteCreated(types.AgentDelegationRequestCreated{
		RequestID:   request.ID,
		ApprovalURL: h.approvalURL(request),
		ExpiresAt:   *types.NewTime(request.ExpiresAt),
	})
}

// approvalURL returns the page at which a user approves request, with its login hint for sign-in.
func (h *AgentDelegationHandler) approvalURL(request *gtypes.AgentDelegationRequest) string {
	approvalURL := h.serverURL + agentApprovalPath + url.PathEscape(request.ID)
	if request.LoginHint != "" {
		approvalURL += "?" + url.Values{agentApprovalLoginHintParam: {request.LoginHint}}.Encode()
	}
	return approvalURL
}

// GetRequest returns a request, the agent that made it and what the agent may do for the signed-in user, once it
// checks that they may approve it.
func (h *AgentDelegationHandler) GetRequest(req api.Context) error {
	approver, err := delegationApprover(req)
	if err != nil {
		return err
	}
	approval, err := req.GatewayClient.GetAgentDelegationApproval(req.Context(), req.PathValue("request_id"), approver)
	if err != nil {
		return agentDelegationApprovalError(err)
	}
	access, err := agentDelegationAccess(req, approval.Agent)
	if err != nil {
		return err
	}

	request := types.AgentDelegationApprovalRequest{
		ID:        approval.Request.ID,
		Agent:     convertDelegationAgent(approval.Agent),
		ExpiresAt: *types.NewTime(approval.Request.ExpiresAt),
		Access:    access,
	}
	if approval.Delegation != nil {
		request.DelegationID = approval.Delegation.ID
	}
	return req.Write(request)
}

// Approve approves a request as the signed-in user, once it checks again that they may, and returns where to send
// their browser: the agent's return URL with the request ID and a one-time code.
func (h *AgentDelegationHandler) Approve(req api.Context) error {
	approver, err := delegationApprover(req)
	if err != nil {
		return err
	}
	approval, code, err := req.GatewayClient.ApproveAgentDelegationRequest(req.Context(), req.PathValue("request_id"), approver)
	if err != nil {
		return agentDelegationApprovalError(err)
	}
	redirectURL, err := agentdelegation.RedirectURL(approval.Request.ReturnURL, map[string]string{
		"requestID": approval.Request.ID,
		"code":      code,
	})
	if err != nil {
		return err
	}
	return req.Write(types.AgentDelegationRedirect{RedirectURL: redirectURL})
}

// Cancel turns a request down as the signed-in user, once it checks that they could approve it, and returns where to
// send their browser: the agent's return URL with the request ID and error=access_denied.
func (h *AgentDelegationHandler) Cancel(req api.Context) error {
	approver, err := delegationApprover(req)
	if err != nil {
		return err
	}
	request, err := req.GatewayClient.CancelAgentDelegationRequest(req.Context(), req.PathValue("request_id"), approver)
	if err != nil {
		return agentDelegationApprovalError(err)
	}
	redirectURL, err := agentdelegation.RedirectURL(request.ReturnURL, map[string]string{
		"requestID": request.ID,
		"error":     "access_denied",
	})
	if err != nil {
		return err
	}
	return req.Write(types.AgentDelegationRedirect{RedirectURL: redirectURL})
}

// Token mints a token with which the calling agent acts on behalf of a user: by redeeming the code of an approved
// request, or from a delegation the agent holds. The calling key is the token's minting key.
func (h *AgentDelegationHandler) Token(req api.Context) error {
	agentID, keyID, err := agentCaller(req)
	if err != nil {
		return err
	}
	var body types.AgentDelegationTokenRequest
	if err := readAgentIdentityRequest(req, &body, false); err != nil {
		return err
	}

	var delegation *gtypes.AgentDelegation
	switch body.GrantType {
	case types.AgentDelegationGrantCode:
		if body.DelegationID != "" {
			return types.NewErrBadRequest("delegationID is not part of the %s grant", types.AgentDelegationGrantCode)
		}
		// A missing secret is a wrong one, and uses the code up: the browser that came back has no secret when its
		// session is not the one that asked, as when an approval link was forwarded to someone else.
		if body.RequestID == "" || body.Code == "" {
			return types.NewErrBadRequest("the %s grant needs requestID, code and secret", types.AgentDelegationGrantCode)
		}
		delegation, err = req.GatewayClient.RedeemAgentDelegationCode(req.Context(), agentID, keyID, body.RequestID, body.Code, body.Secret)
		if err != nil {
			return agentDelegationError(err)
		}
	case types.AgentDelegationGrantDelegation:
		if body.RequestID != "" || body.Code != "" || body.Secret != "" {
			return types.NewErrBadRequest("requestID, code and secret are not part of the %s grant", types.AgentDelegationGrantDelegation)
		}
		if body.DelegationID == "" {
			return types.NewErrBadRequest("the %s grant needs delegationID", types.AgentDelegationGrantDelegation)
		}
		// Another agent's delegation is not found either, so that an agent learns nothing of another's.
		delegation, err = req.GatewayClient.GetAgentDelegationOfAgent(req.Context(), body.DelegationID, agentID)
		if errors.Is(err, gclient.ErrAgentDelegationNotFound) {
			return types.NewErrNotFound("agent delegation not found; it was revoked, or is not this agent's, and the user must approve the agent again")
		} else if err != nil {
			return err
		}
	default:
		return types.NewErrBadRequest("grantType must be %q or %q", types.AgentDelegationGrantCode, types.AgentDelegationGrantDelegation)
	}

	return h.mint(req, agentID, keyID, delegation, body.GrantType == types.AgentDelegationGrantDelegation)
}

// mint issues a token from delegation, of the agent with agentID, with its key keyID, once it checks that the agent
// may still act for the user: the agent and the user are active, and a profile of the agent matches the
// user with their current groups. With touch, it records the delegation's use, which redeeming a code already did.
func (h *AgentDelegationHandler) mint(req api.Context, agentID, keyID uint, delegation *gtypes.AgentDelegation, touch bool) error {
	ctx := req.Context()
	agent, err := req.GatewayClient.ActiveAgent(ctx, agentID)
	if err != nil {
		return agentDelegationError(err)
	}
	user, err := req.GatewayClient.UserByID(ctx, strconv.FormatUint(uint64(delegation.UserID), 10))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrForbidden("the user was deleted, so the agent can no longer act on their behalf")
	} else if err != nil {
		return err
	}
	if user.Status() != types.UserStatusActive {
		return types.NewErrForbidden("the user is %s, so the agent can't act on their behalf", user.Status())
	}
	if ok, err := delegationUserMatches(ctx, req.GatewayClient, agent, delegation.UserID); err != nil {
		return err
	} else if !ok {
		return types.NewErrForbidden("no profile of the agent includes the user any more; an administrator must give the agent a profile that includes them")
	}

	token, expiresAt, err := h.tokens.NewDelegatedToken(ctx, agentID, delegation.UserID, delegation.ID, keyID, 0)
	if err != nil {
		return agentDelegationError(err)
	}
	if touch {
		if err := req.GatewayClient.TouchAgentDelegation(ctx, delegation.ID, time.Now()); err != nil {
			return err
		}
	}
	return req.Write(types.AgentDelegationToken{
		DelegationID: delegation.ID,
		AccessToken:  token,
		TokenType:    "Bearer",
		ExpiresAt:    *types.NewTime(expiresAt),
		User:         convertDelegationUser(user),
	})
}

// delegationUserMatches reports whether a profile of agent matches the user with userID, with their current
// groups.
func delegationUserMatches(ctx context.Context, gateway *gclient.Client, agent *gtypes.Agent, userID uint) (bool, error) {
	info, err := gateway.UserInfoByID(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	caller, ok := subjects.CallerFromUserInfo(info)
	if !ok {
		return false, nil
	}
	return agentprofile.Matches(agent.Profiles, caller), nil
}

// ListMine lists the signed-in user's delegations: the agents that may act on their behalf.
func (h *AgentDelegationHandler) ListMine(req api.Context) error {
	userID, err := delegationOwner(req)
	if err != nil {
		return err
	}
	delegations, err := req.GatewayClient.ListUserAgentDelegations(req.Context(), userID)
	if err != nil {
		return err
	}
	user, err := req.GatewayClient.UserByID(req.Context(), strconv.FormatUint(uint64(userID), 10))
	if err != nil {
		return err
	}

	agents := map[uint]types.AgentDelegationAgent{}
	items := make([]types.AgentDelegation, 0, len(delegations))
	for _, delegation := range delegations {
		agent, ok := agents[delegation.AgentID]
		if !ok {
			stored, err := req.GatewayClient.GetAgent(req.Context(), delegation.AgentID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Deleted since it was listed; its cleanup removes the delegation.
				continue
			} else if err != nil {
				return err
			}
			agent = convertDelegationAgent(stored)
			agents[delegation.AgentID] = agent
		}
		items = append(items, convertAgentDelegation(delegation, agent, convertDelegationUser(user)))
	}
	return req.Write(types.AgentDelegationList{Items: items})
}

// RevokeMine revokes a delegation of the signed-in user, which the authorizer admits only them to
// (checkAgentDelegation).
func (h *AgentDelegationHandler) RevokeMine(req api.Context) error {
	userID, err := delegationOwner(req)
	if err != nil {
		return err
	}
	return h.revoke(req, req.PathValue("delegation_id"), principal.User(userID).String())
}

// ListForAgent lists the delegations of an agent, for administrators: the users it may act for, and when they
// approved it and it last got a token. It never shows what a user set up through the agent.
func (h *AgentDelegationHandler) ListForAgent(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	delegations, err := req.GatewayClient.ListAgentDelegations(req.Context(), agent.ID)
	if err != nil {
		return err
	}

	converted := convertDelegationAgent(agent)
	items := make([]types.AgentDelegation, 0, len(delegations))
	for _, delegation := range delegations {
		user, err := req.GatewayClient.UserByID(req.Context(), strconv.FormatUint(uint64(delegation.UserID), 10))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Deleted since it was listed; its cleanup removes the delegation.
			continue
		} else if err != nil {
			return err
		}
		items = append(items, convertAgentDelegation(delegation, converted, convertDelegationUser(user)))
	}
	return req.Write(types.AgentDelegationList{Items: items})
}

// RevokeForAgent revokes a delegation of an agent, as an administrator. A delegation is named under its own agent,
// so that of another agent is not found.
func (h *AgentDelegationHandler) RevokeForAgent(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	delegation, err := req.GatewayClient.GetAgentDelegationOfAgent(req.Context(), req.PathValue("delegation_id"), agent.ID)
	if errors.Is(err, gclient.ErrAgentDelegationNotFound) {
		return types.NewErrNotFound("agent delegation not found")
	} else if err != nil {
		return err
	}
	// The administrator is recorded as the admin change audit records administrators (adminAuditContext).
	return h.revoke(req, delegation.ID, req.User.GetUID())
}

// revoke deletes the delegation with delegationID, as actor, and then what the user set up through the agent
// (agentdelegation.Cleanup). The delegation goes first, so that a failure to remove the setup never keeps the agent
// acting for the user. Setup that is left is removed when the agent or the user is deleted.
func (h *AgentDelegationHandler) revoke(req api.Context, delegationID, actor string) error {
	revoked, err := req.GatewayClient.RevokeAgentDelegation(req.Context(), delegationID, actor)
	if errors.Is(err, gclient.ErrAgentDelegationNotFound) {
		return types.NewErrNotFound("agent delegation not found")
	} else if err != nil {
		return err
	}
	if err := h.cleanup.CleanupDelegations(req.Context(), agentdelegation.Scope{AgentID: revoked.AgentID, UserID: revoked.UserID}); err != nil {
		return types.NewErrHTTP(http.StatusInternalServerError, "the delegation was revoked, but removing what the user set up through the agent failed: "+err.Error())
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// agentCaller returns the agent that makes the request and the ID of the API key it authenticated with, which are the
// token's minting key and the request's. The authorizer admits only an agent with its own key to the agent's routes
// (authz agentKeyRules), so a caller without them is a fault of the server, not of the caller.
func agentCaller(req api.Context) (agentID, keyID uint, err error) {
	p, _ := principal.FromUserInfo(req.User)
	agentID, isAgent := p.AgentID()
	attribution, hasKey := principal.APIKeyAttributionFromUser(req.User)
	if !isAgent || !hasKey {
		return 0, 0, types.NewErrHTTP(http.StatusInternalServerError, "the caller is not an agent with an API key")
	}
	return agentID, attribution.ID, nil
}

// delegationApprover returns the signed-in user who reads, approves or cancels a request, with their groups. The
// authorizer admits only a user's browser session to the approval routes (authz authorizeAgentDelegationApproval).
func delegationApprover(req api.Context) (subjects.Caller, error) {
	caller, ok := subjects.CallerFromUserInfo(req.User)
	if !ok || caller.Principal.Kind != principal.KindUser {
		return subjects.Caller{}, types.NewErrHTTP(http.StatusInternalServerError, "the caller is not a user")
	}
	return caller, nil
}

// delegationOwner returns the ID of the signed-in user whose delegations are listed or revoked. The authorizer admits
// only users to the "me" routes.
func delegationOwner(req api.Context) (uint, error) {
	p, _ := principal.FromUserInfo(req.User)
	userID, ok := p.UserID()
	if !ok {
		return 0, types.NewErrHTTP(http.StatusInternalServerError, "the caller is not a user")
	}
	return userID, nil
}

// agentDelegationAccess returns what agent may do for a user its profiles match: its own grants (agentGrants), with
// the profiles of each vMCP together.
func agentDelegationAccess(req api.Context, agent *gtypes.Agent) (types.AgentDelegationAccess, error) {
	grants, err := agentGrants(req, agent)
	if err != nil {
		return types.AgentDelegationAccess{}, err
	}

	access := types.AgentDelegationAccess{
		VMCPs:               []types.AgentDelegationVMCPAccess{},
		ModelAccessPolicies: grants.ModelAccessPolicies,
		SkillAccessRules:    grants.SkillAccessRules,
	}
	// agentGrants lists the profiles of each vMCP together, and the vMCPs by ID.
	for _, profile := range grants.VMCPProfiles {
		if len(access.VMCPs) == 0 || access.VMCPs[len(access.VMCPs)-1].VMCPID != profile.VMCPID {
			access.VMCPs = append(access.VMCPs, types.AgentDelegationVMCPAccess{
				VMCPID:          profile.VMCPID,
				VMCPDisplayName: profile.VMCPDisplayName,
				Profiles:        []types.AgentDelegationVMCPProfile{},
			})
		}
		vmcp := &access.VMCPs[len(access.VMCPs)-1]
		vmcp.Profiles = append(vmcp.Profiles, types.AgentDelegationVMCPProfile{
			ID:   profile.ProfileID,
			Name: profile.ProfileName,
		})
	}
	return access, nil
}

// agentDelegationApprovalError maps a refusal of a user's approval onto the response the approval page shows, with
// a message for the user.
func agentDelegationApprovalError(err error) error {
	switch {
	case errors.Is(err, gclient.ErrAgentDelegationRequestNotFound):
		return types.NewErrNotFound("This approval link doesn't exist or has expired. Go back to the agent to get a new one.")
	case errors.Is(err, gclient.ErrAgentDelegationRequestUnusable):
		return types.NewErrForbidden("This approval link has expired or was already used. Go back to the agent to get a new one.")
	case errors.Is(err, gclient.ErrAgentDelegationReturnURLNotAllowed):
		return types.NewErrForbidden("This agent is no longer allowed to send you back to where it asked. Go back to the agent to get a new link.")
	case errors.Is(err, gclient.ErrAgentDelegationNoProfile):
		return types.NewErrForbidden("This agent isn't allowed to act on your behalf. Ask an administrator to give it user access that includes you.")
	case errors.Is(err, gclient.ErrAgentDelegationOtherUser):
		return types.NewErrForbidden("This approval link is for another account. Sign in as the account that approved the agent before, or go back to the agent to get a new link.")
	}
	if _, ok := errors.AsType[*gclient.AgentAccessDeniedError](err); ok {
		return types.NewErrForbidden("This agent is disabled or was deleted, so it can't act on your behalf.")
	}
	return err
}

// agentDelegationError maps an error of the agent's routes onto the status the API answers with. Any other error is
// a 500.
func agentDelegationError(err error) error {
	switch {
	case errors.Is(err, gclient.ErrInvalidAgentDelegationRequest), errors.Is(err, gclient.ErrInvalidAgentDelegationGrant):
		return types.NewErrBadRequest("%s", err.Error())
	case errors.Is(err, gclient.ErrAgentDelegationRequestLimit):
		return types.NewErrHTTP(http.StatusTooManyRequests, err.Error())
	}
	if denied, ok := errors.AsType[*gclient.AgentAccessDeniedError](err); ok {
		return denied.HTTPError()
	}
	if denied, ok := errors.AsType[*gclient.UserAccessDeniedError](err); ok {
		return types.NewErrForbidden("the user is %s, so the agent can't act on their behalf", denied.Status)
	}
	return err
}

func convertAgentDelegation(delegation gtypes.AgentDelegation, agent types.AgentDelegationAgent, user types.AgentDelegationUser) types.AgentDelegation {
	return types.AgentDelegation{
		ID:              delegation.ID,
		Agent:           agent,
		User:            user,
		FirstApprovedAt: *types.NewTime(delegation.FirstApprovedAt),
		LastApprovedAt:  *types.NewTime(delegation.LastApprovedAt),
		LastUsedAt:      optionalAPITime(delegation.LastUsedAt),
	}
}

func convertDelegationAgent(agent *gtypes.Agent) types.AgentDelegationAgent {
	return types.AgentDelegationAgent{
		ID:          strconv.FormatUint(uint64(agent.ID), 10),
		DisplayName: agent.DisplayName,
		Description: agent.Description,
		Status:      agent.Status(),
	}
}

func convertDelegationUser(user *gtypes.User) types.AgentDelegationUser {
	return types.AgentDelegationUser{
		ID:          strconv.FormatUint(uint64(user.ID), 10),
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Status:      user.Status(),
	}
}
