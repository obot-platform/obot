package authz

import (
	"errors"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
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

	// agentDelegationCrossOrigin refuses a request to approve or cancel that another site's page made. The session
	// cookie is SameSite=Lax, so a browser does not send it with another site's POST, as for the MCP OAuth consent
	// approval; this also refuses one where the user needs no cookie, such as with authentication off.
	agentDelegationCrossOrigin = http.NewCrossOriginProtection()
)

// authorizeAgentDelegationApproval authorizes the routes of agentDelegationApprovalRoutes: they are reached only by a
// user's browser session (agentDelegationApprover), and their POSTs only from Obot's own pages.
func authorizeAgentDelegationApproval(req *http.Request, u User) bool {
	if _, ok := agentDelegationApprovalRoutes.Match(req); !ok {
		return false
	}
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
