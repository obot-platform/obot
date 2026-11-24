package authz

import (
	"errors"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	// agentDelegationApprovalRoutes are the routes on which a signed-in user reads, approves or cancels an agent's
	// request to act on their behalf (authorizeAgentDelegationApproval). Whether the user may approve a given request
	// is the handler's to answer: it depends on the request, the agent and the user's groups at the moment of approval,
	// and the approval page shows why it may not.
	agentDelegationApprovalRoutes = newPathMatcher(
		"GET  /api/agent-delegation-requests/{request_id}",
		"POST /api/agent-delegation-requests/{request_id}/approve",
		"POST /api/agent-delegation-requests/{request_id}/cancel",
	)

	// agentDelegationRequestSetupRoutes are the routes on which a signed-in user sets up the vMCPs of a request they are
	// approving, through the agent that made it (authorizeAgentDelegationRequestSetup). The vMCP's path value is not
	// vmcp_id, as on the delegation's own setup routes.
	agentDelegationRequestSetupRoutes = newPathMatcher(
		"GET    /api/agent-delegation-requests/{request_id}/setup",
		"PUT    /api/agent-delegation-requests/{request_id}/setup/vmcps/{setup_vmcp_id}/configuration",
		"POST   /api/agent-delegation-requests/{request_id}/setup/vmcps/{setup_vmcp_id}/components/{component_id}/oauth",
		"DELETE /api/agent-delegation-requests/{request_id}/setup/vmcps/{setup_vmcp_id}/components/{component_id}/oauth",
	)

	// agentDelegationCrossOrigin refuses a request to approve or cancel that another site's page made. The session
	// cookie is SameSite=Lax, so a browser does not send it with another site's POST, as for the MCP OAuth consent
	// approval; this also refuses one where the user needs no cookie, such as with authentication off.
	agentDelegationCrossOrigin = http.NewCrossOriginProtection()
)

// authorizeAgentDelegationApproval authorizes the routes of agentDelegationApprovalRoutes: they are reached only by a
// user's browser session (agentDelegationApprover), and their POSTs only from Obot's own pages.
func authorizeAgentDelegationApproval(req *http.Request, u User) bool {
	_, ok := agentDelegationApprovalRoutes.Match(req)
	return ok && agentDelegationSession(req, u)
}

// authorizeAgentDelegationRequestSetup authorizes the routes of agentDelegationRequestSetupRoutes as
// authorizeAgentDelegationApproval does the approval routes, a user's browser session from Obot's own pages, and only
// for a request that the user may approve now: it is pending and unexpired, its agent is active, a profile of the
// agent matches the user, and a request to approve a delegation again is for the user's own
// (client.GetAgentDelegationApproval). What the user sets up there is the pair's of the request's agent and the user,
// so another user's request, or one that expired or was used, is refused before anything is read or written.
func (a *Authorizer) authorizeAgentDelegationRequestSetup(req *http.Request, u User) bool {
	vars, ok := agentDelegationRequestSetupRoutes.Match(req)
	if !ok || !agentDelegationSession(req, u) {
		return false
	}
	approver, ok := subjects.CallerFromUserInfo(u.Info)
	if !ok || approver.Principal.Kind != principal.KindUser {
		return false
	}
	_, err := a.gatewayClient.GetAgentDelegationApproval(req.Context(), vars("request_id"), approver)
	return err == nil
}

// agentDelegationSession reports whether req is from a user's browser session that may approve an agent's request
// (agentDelegationApprover), and, if it is unsafe, from Obot's own pages (agentDelegationCrossOrigin).
func agentDelegationSession(req *http.Request, u User) bool {
	return slices.Contains(u.GetGroups(), types.GroupAPI) &&
		agentDelegationApprover(u.Info) &&
		agentDelegationCrossOrigin.Check(req) == nil
}

// agentDelegationApprover reports whether a caller may approve an agent's request in their browser: a signed-in user,
// and not the bootstrap login or an API key. Approval is consent, shown to a person in Obot's own page.
//
// The bootstrap login is recognized by its auth provider, as api.Context.UserIsBootstrap does, and never by the
// username, which an identity provider supplies and which may be "bootstrap" for a real user.
func agentDelegationApprover(u user.Info) bool {
	if !slices.Contains(u.GetGroups(), types.GroupAuthenticated) {
		return false
	}
	extra := u.GetExtra()
	if slices.Contains(extra["auth_provider_name"], system.BootstrapName) || slices.Contains(extra["auth_provider_namespace"], system.BootstrapName) {
		return false
	}
	if _, ok := principal.APIKeyAttributionFromUser(u); ok {
		return false
	}
	// A caller that is not a well-formed principal is the zero Principal, which is no user.
	p, _ := principal.FromUserInfo(u)
	userID, ok := p.UserID()
	return ok && userID != 0
}

// checkAgentDelegation lets a user reach a delegation ({delegation_id}) only if it is theirs: a user's approval for an
// agent to act on their behalf is the user's to manage. Administrators manage an agent's delegations through the
// agent's own routes.
func (a *Authorizer) checkAgentDelegation(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.AgentDelegationID == "" {
		return true, nil
	}
	p, _ := principal.FromUserInfo(u.Info)
	userID, ok := p.UserID()
	if !ok {
		return false, nil
	}
	delegation, err := a.gatewayClient.GetAgentDelegation(req.Context(), resources.AgentDelegationID)
	if errors.Is(err, client.ErrAgentDelegationNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return delegation.UserID == userID, nil
}
