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

	// Read sub + aud WITHOUT verifying, only to decide if this token is ours and
	// to locate the device's registered key. Claim only our audience so this
	// never shadows user/session JWTs.
	sub, aud, err := unverifiedSubAud(tok)
	if err != nil || sub == "" || !hasAudience(aud, deviceTokenAudience) {
		return nil, false, nil
	}

	device, err := a.client.GetDeviceByDeviceID(req.Context(), sub)
	if err != nil {
		return nil, false, nil
	}

	pubKey, err := x509.ParsePKIXPublicKey(device.PublicKey)
	if err != nil {
		return nil, false, nil
	}

	// Verify the signature against the STORED key: this is the proof of
	// device_id — only the holder of the registered private key can sign for it.
	if err := verifyDeviceJWT(tok, device.DeviceID, pubKey); err != nil {
		return nil, false, nil
	}

	principal := gtypes.DevicePrincipalName(device.DeviceID)
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: principal,
			UID:  principal,
			// device-scans is the only capability — authorizes the scan
			// endpoints and nothing else (see authz staticRules).
			Groups: []string{types.GroupDeviceScans},
			Extra: map[string][]string{
				"device_id":            {device.DeviceID},
				"device_deployment_id": {fmt.Sprintf("%d", device.DeviceDeploymentID)},
			},
		},
	}, true, nil
}

// unverifiedSubAud reads the sub and aud of a JWT WITHOUT verifying its
// signature, used only to route the request to this authenticator and locate
// the device's registered key.
func unverifiedSubAud(tok string) (sub string, aud jwt.ClaimStrings, err error) {
	var claims jwt.RegisteredClaims
	if _, _, err = jwt.NewParser().ParseUnverified(tok, &claims); err != nil {
		return "", nil, err
	}
	return claims.Subject, claims.Audience, nil
}

func hasAudience(aud jwt.ClaimStrings, want string) bool {
	return slices.Contains(aud, want)
}

// verifyDeviceJWT verifies a device access JWT against the device's registered
// public key: signature (asymmetric alg only), audience, expiry, and that
// iss == sub == the device id.
func verifyDeviceJWT(tok, deviceID string, pubKey any) error {
	parser := jwt.NewParser(
		jwt.WithValidMethods(deviceAssertionAlgs),
		jwt.WithAudience(deviceTokenAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(deviceID),
		jwt.WithSubject(deviceID),
	)
	t, err := parser.ParseWithClaims(tok, &jwt.RegisteredClaims{}, func(*jwt.Token) (any, error) {
		return pubKey, nil
	})
	if err != nil {
		return err
	}
	if !t.Valid {
		return fmt.Errorf("device assertion is invalid")
	}
	return nil
}
