package authn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	// ErrRestricted refuses a principal while the environment is restricted.
	ErrRestricted = types2.NewErrHTTP(http.StatusPaymentRequired, "Only admins can use this environment right now, because it's over its plan or its subscription couldn't be confirmed. An admin can bring usage back within the plan, or an owner can check the subscription.")

	// errAgentOAuthGrantCaller refuses a caller whose UID is the owner of a vMCP's agent OAuth grants. No authenticator
	// builds one, and none must: the owner stands for a vMCP's grants, never for anyone who can act.
	errAgentOAuthGrantCaller = errors.New("the owner of a vMCP's agent OAuth grants cannot authenticate")

	// nonUserGroups mark principals that act for Obot itself or for a device, never for a user, so they have no
	// lifecycle status. Only their own authenticators assign these groups.
	nonUserGroups = []string{
		authz.UnauthenticatedGroup,
		authz.MetricsGroup,
		types2.GroupTunnel,
		types2.GroupTunnelBridge,
		types2.GroupTunnelPeer,
		types2.GroupDeviceEnroll,
		types2.GroupSCIM,
	}
)

// Restriction reports whether the environment is restricted.
type Restriction interface {
	Restricted(context.Context) (bool, error)
}

// AdmissionCheck denies requests made by or for users or agents who may not access Obot: disabled, deleted, or
// missing ones, and while the environment is restricted, any who aren't admins or owners. It wraps the complete
// authenticator chain, so it runs once per request, after authentication and before authorization, whichever
// credential authenticated the request. Agents' authenticators record their lifecycle status as users' do, so an
// agent is admitted by the same rule.
type AdmissionCheck struct {
	next        authenticator.Request
	restriction Restriction
}

// NewAdmissionCheck returns the check. A nil restriction never restricts.
func NewAdmissionCheck(next authenticator.Request, restriction Restriction) *AdmissionCheck {
	return &AdmissionCheck{
		next:        next,
		restriction: restriction,
	}
}

func (a *AdmissionCheck) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	resp, ok, err := a.next.AuthenticateRequest(req)
	if err != nil || !ok {
		return resp, ok, err
	}
	if resp == nil || resp.User == nil {
		return nil, false, errors.New("authenticator returned no principal")
	}
	// The owner of a vMCP's agent OAuth grants is refused whatever status or groups its authenticator recorded, so that
	// no authenticator, now or later, can let a caller read or use the grants as their own.
	if principal.IsAgentOAuthGrantOwner(resp.User.GetUID()) {
		return nil, false, errAgentOAuthGrantCaller
	}

	status, recorded := principal.UserStatus(resp.User)
	if !recorded {
		// Principals that act for Obot itself or for a device have no status, and the restriction doesn't apply to them.
		// An agent is never a non-user principal: whatever groups it has, it is admitted only with a recorded status.
		if !principal.ClaimsAgent(resp.User) && isNonUserPrincipal(resp.User) {
			return resp, true, nil
		}
		return nil, false, accessLookupError(resp.User, fmt.Errorf("the authenticator of principal %q did not report a lifecycle status", resp.User.GetName()))
	}
	if status != types2.UserStatusActive {
		return nil, false, accessDeniedError(resp.User, status)
	}

	// Check the restriction. Admins and owners keep access so they can bring usage back within the limits.
	if a.restriction == nil || principal.IsAdminOrOwner(resp.User) {
		return resp, true, nil
	}
	restricted, err := a.restriction.Restricted(req.Context())
	if err != nil {
		// Like a user status that can't be read: refused as unavailable, without the details.
		return nil, false, accessLookupError(resp.User, fmt.Errorf("failed to check whether the environment is restricted: %w", err))
	}
	if restricted {
		return nil, false, ErrRestricted
	}
	return resp, true, nil
}

// accessDeniedError returns the error that denies u for its lifecycle status. It names the agent a principal acts for,
// so that a denial of an agent is never reported as one of the user with the same numeric ID, or of user 0.
func accessDeniedError(u user.Info, status types2.UserStatus) error {
	// A principal that acts for no one is the zero Principal, which is reported as user 0.
	p, _ := principal.FromUserInfo(u)
	if agentID, ok := p.AgentID(); ok {
		return &gclient.AgentAccessDeniedError{
			AgentID: agentID,
			Status:  status,
		}
	}
	userID, _ := p.UserID()
	return &gclient.UserAccessDeniedError{
		UserID: userID,
		Status: status,
	}
}

// accessLookupError returns the error that refuses u because its lifecycle status is unknown.
func accessLookupError(u user.Info, err error) error {
	p, _ := principal.FromUserInfo(u)
	if agentID, ok := p.AgentID(); ok {
		return &gclient.AgentAccessLookupError{
			AgentID: agentID,
			Err:     err,
		}
	}
	userID, _ := p.UserID()
	return &gclient.UserAccessLookupError{
		UserID: userID,
		Err:    err,
	}
}

// isNonUserPrincipal reports whether a principal acts for Obot itself or for a device rather than for a user.
func isNonUserPrincipal(u user.Info) bool {
	groups := u.GetGroups()
	for _, group := range nonUserGroups {
		if slices.Contains(groups, group) {
			return true
		}
	}

	// A device's principal carries device-scans, which a user's API key can carry too, so it is recognized by the
	// device ID its authenticator records.
	return slices.Contains(groups, types2.GroupDeviceScans) && len(u.GetExtra()["device_id"]) > 0
}
