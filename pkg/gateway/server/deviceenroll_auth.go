package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const deviceEnrollAuthPrefix = "ode1-"

// DeviceEnrollmentAuthenticator authenticates device-enrollment credentials
// (ode1-<deployment_id>-<key_id>-<secret>).
//
// The resulting principal IS the MDMDeployment the credential belongs to:
// its Name/UID is the deployment's namespaced identity (mdm-deployment:<id>)
// and its capability group is DeviceEnroll, authorizing the enrollment endpoint.
// The deployment id is stashed in Extra for the enroll handler to record on the
// device.
//
// This mirrors APIKeyAuthenticator, but yields a non-user principal — there is
// no gateway user behind an enrollment credential — so it must be placed after
// the UserDecorator in the authenticator union.
type DeviceEnrollmentAuthenticator struct {
	client *client.Client
}

// NewDeviceEnrollmentAuthenticator creates a new device-enrollment authenticator.
func NewDeviceEnrollmentAuthenticator(client *client.Client) *DeviceEnrollmentAuthenticator {
	return &DeviceEnrollmentAuthenticator{client: client}
}

// AuthenticateRequest implements authenticator.Request.
func (a *DeviceEnrollmentAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	// Only handle Bearer device-enrollment credentials; let other
	// authenticators try the rest.
	credential, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	if !ok || !strings.HasPrefix(credential, deviceEnrollAuthPrefix) {
		return nil, false, nil
	}

	key, err := a.client.ValidateDeviceEnrollmentCredential(req.Context(), credential)
	if err != nil {
		// Invalid credential: fall through (ends at anonymous, then denied by authz).
		return nil, false, nil
	}

	principal := gtypes.MDMDeploymentPrincipalName(key.MDMDeploymentID)
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   principal,
			UID:    principal,
			Groups: []string{types.GroupDeviceEnroll, types.GroupAuthenticated},
			Extra: map[string][]string{
				"mdm_deployment_id": {fmt.Sprintf("%d", key.MDMDeploymentID)},
			},
		},
	}, true, nil
}
