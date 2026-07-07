package server

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// deviceTokenAudience is the audience an enrolled device's self-signed access
// JWT must target. A fixed value (not a URL) keeps devices robust to
// TLS-terminating proxies and lets the authenticator claim only its own tokens.
const deviceTokenAudience = "obot/device"

// deviceAssertionAlgs are the asymmetric signing algorithms accepted for a
// device access JWT. Symmetric / "none" are excluded so a registered public
// key can never be abused as an HMAC secret (alg-confusion).
var deviceAssertionAlgs = []string{"ES256", "ES384", "ES512", "EdDSA"}

// DeviceAuthenticator authenticates an enrolled device by a short-lived JWT the
// device signs itself with its identity key. There is no server-minted token:
// the JWT is verified against the public key registered at enrollment. The
// resulting principal is the device (device:<device_id>), scoped to submitting
// and reading its own device scans, so an unattended device can submit a scan
// attributed to itself.
type DeviceAuthenticator struct {
	client *client.Client
}

// NewDeviceAuthenticator creates a new device access-JWT authenticator.
func NewDeviceAuthenticator(client *client.Client) *DeviceAuthenticator {
	return &DeviceAuthenticator{client: client}
}

// AuthenticateRequest implements authenticator.Request.
func (a *DeviceAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	tok := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	// A device access token is a JWT (three dot-separated segments). Anything
	// else (opaque tokens, API keys) is left to other authenticators.
	if tok == "" || strings.Count(tok, ".") != 2 {
		return nil, false, nil
	}

	var device *gtypes.Device
	parser := jwt.NewParser(
		jwt.WithValidMethods(deviceAssertionAlgs),
		jwt.WithAudience(deviceTokenAudience),
		jwt.WithExpirationRequired(),
	)
	// The keyfunc sees the decoded (not yet verified) claims: it claims only
	// our audience — so this never shadows user/session JWTs and foreign tokens
	// bail before the DB lookup — then locates the device's registered key by
	// sub. iss == sub is checked here rather than via parser options because
	// the device id isn't known until the claims are read. Verifying the
	// signature against the STORED key is the proof of device_id — only the
	// holder of the registered private key can sign for it.
	if _, err := parser.ParseWithClaims(tok, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		claims, ok := t.Claims.(*jwt.RegisteredClaims)
		if !ok || !slices.Contains(claims.Audience, deviceTokenAudience) ||
			claims.Subject == "" || claims.Issuer != claims.Subject {
			return nil, fmt.Errorf("not a device access token")
		}
		var err error
		if device, err = a.client.GetDeviceByDeviceID(req.Context(), claims.Subject); err != nil {
			return nil, err
		}
		return x509.ParsePKIXPublicKey(device.PublicKey)
	}); err != nil || device == nil {
		return nil, false, nil
	}

	principal := gtypes.DevicePrincipalName(device.DeviceID)
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: principal,
			UID:  principal,
			// device-scans is the sole capability — beyond baseline
			// authenticated access, it authorizes only the scan endpoints
			// (see authz staticRules).
			Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
			Extra: map[string][]string{
				"device_id":         {device.DeviceID},
				"mdm_deployment_id": {fmt.Sprintf("%d", device.MDMDeploymentID)},
			},
		},
	}, true, nil
}
