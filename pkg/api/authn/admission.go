package authn

import (
	"context"
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
	// ErrRestricted refuses a principal while the environment is restricted.
	ErrRestricted = types2.NewErrHTTP(http.StatusPaymentRequired, "Only admins can use this environment right now, because it's over its plan or its subscription couldn't be confirmed. An admin can bring usage back within the plan, or an owner can check the subscription.")

	// nonUserGroups mark principals that act for Obot itself, never for a user, so they have no lifecycle status.
	// Only their own authenticators assign these groups. Devices have none either, see isDevice.
	nonUserGroups = []string{
		authz.UnauthenticatedGroup,
		authz.MetricsGroup,
		types2.GroupTunnel,
		types2.GroupTunnelBridge,
		types2.GroupTunnelPeer,
		types2.GroupSCIM,
	}
)

// Restriction reports whether the environment is restricted.
type Restriction interface {
	Restricted(context.Context) (bool, error)
}

// AdmissionCheck denies requests made by or for users who may not access Obot: disabled, deleted, or missing
// users, and while the environment is restricted, users who aren't admins or owners, and devices. It wraps the
// complete authenticator chain, so it runs once per request, after authentication and before authorization,
// whichever credential authenticated the request.
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

	if status, recorded := principal.UserStatus(resp.User); recorded {
		if status != types2.UserStatusActive {
			return nil, false, &gclient.UserAccessDeniedError{
				UserID: admissionUserID(resp.User),
				Status: status,
			}
		}
	} else if !isNonUserPrincipal(resp.User) {
		return nil, false, &gclient.UserAccessLookupError{
			UserID: admissionUserID(resp.User),
			Err:    fmt.Errorf("the authenticator of principal %q did not report a user status", resp.User.GetName()),
		}
	}

	if a.restriction != nil && restrictable(resp.User) {
		restricted, err := a.restriction.Restricted(req.Context())
		if err != nil {
			// Like a user status that can't be read: refused as unavailable, without the details.
			return nil, false, &gclient.UserAccessLookupError{
				UserID: admissionUserID(resp.User),
				Err:    fmt.Errorf("failed to check whether the environment is restricted: %w", err),
			}
		}
		if restricted {
			return nil, false, ErrRestricted
		}
	}
	return resp, true, nil
}

// restrictable reports whether a restricted environment refuses the principal: users who aren't admins or owners, and
// devices, which have no user. Admins and owners keep access so they can bring usage back within the limits, and
// Obot's own principals and SCIM always get through.
func restrictable(u user.Info) bool {
	if _, isUser := principal.UserStatus(u); isUser {
		return !principal.IsAdminOrOwner(u)
	}
	return isDevice(u)
}

// isNonUserPrincipal reports whether a principal acts for Obot itself or for a device rather than for a user.
func isNonUserPrincipal(u user.Info) bool {
	groups := u.GetGroups()
	for _, group := range nonUserGroups {
		if slices.Contains(groups, group) {
			return true
		}
	}
	return isDevice(u)
}

// isDevice reports whether a principal acts for a device. A device's principal carries device-scans, which a user's API
// key can carry too, so it is recognized by the device ID its authenticator records.
func isDevice(u user.Info) bool {
	groups := u.GetGroups()
	return slices.Contains(groups, types2.GroupDeviceEnroll) ||
		slices.Contains(groups, types2.GroupDeviceScans) && len(u.GetExtra()["device_id"]) > 0
}

// admissionUserID returns the ID of the user a principal acts for, for error reporting, or 0 if it has none.
func admissionUserID(u user.Info) uint {
	id, err := strconv.ParseUint(u.GetUID(), 10, 0)
	if err != nil {
		return 0
	}
	return uint(id)
}
