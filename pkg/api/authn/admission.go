package authn

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
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

// AdmissionCheck denies requests made by or for users who may not access Obot: disabled, deleted, or missing
// users. It wraps the complete authenticator chain, so it runs once per request, after authentication and before
// authorization, whichever credential authenticated the request.
type AdmissionCheck struct {
	next authenticator.Request
}

func NewAdmissionCheck(next authenticator.Request) *AdmissionCheck {
	return &AdmissionCheck{
		next: next,
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

	if status, recorded := principal.UserStatus(resp.User); recorded {
		if status != types2.UserStatusActive {
			return nil, false, &gclient.UserAccessDeniedError{
				UserID: admissionUserID(resp.User),
				Status: status,
			}
		}
		return resp, true, nil
	}

	if isNonUserPrincipal(resp.User) {
		return resp, true, nil
	}

	return nil, false, &gclient.UserAccessLookupError{
		UserID: admissionUserID(resp.User),
		Err:    fmt.Errorf("the authenticator of principal %q did not report a user status", resp.User.GetName()),
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

// admissionUserID returns the ID of the user a principal acts for, for error reporting, or 0 if it has none.
func admissionUserID(u user.Info) uint {
	id, err := strconv.ParseUint(principal.ResourceOwnerID(u), 10, 0)
	if err != nil {
		return 0
	}
	return uint(id)
}
