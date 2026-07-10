package handlers

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/x509"
	"strconv"
	"strings"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

// DeviceEnrollHandler serves device enrollment.
type DeviceEnrollHandler struct{}

func NewDeviceEnrollHandler() *DeviceEnrollHandler {
	return nil
}

// Enroll handles POST /api/mdm/enroll. The caller is authenticated by an
// enrollment credential (the DeviceEnroll principal carries the deployment id
// in Extra). It registers the device's identity key (trust-on-first-use; a
// different key for an existing device is rejected).
func (*DeviceEnrollHandler) Enroll(req api.Context) error {
	deploymentID, ok := uintFromExtra(req.User.GetExtra(), "mdm_deployment_id")
	if !ok {
		return types.NewErrBadRequest("enrollment requires a device enrollment credential")
	}
	var in types.DeviceEnrollRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	// Validation only — the id is stored untrimmed because it must match the
	// iss/sub the device signs into its access JWTs byte-for-byte.
	if strings.TrimSpace(in.DeviceID) == "" {
		return types.NewErrBadRequest("deviceID is required")
	}

	pub, err := x509.ParsePKIXPublicKey(in.PublicKey)
	if err != nil {
		return types.NewErrBadRequest("publicKey is not a valid PKIX public key: %v", err)
	}
	// Register only keys the device authenticator can verify access JWTs
	// against (deviceAssertionAlgs: ES256/384/512, EdDSA). Anything else —
	// e.g. RSA — would enroll a device that can never authenticate.
	switch key := pub.(type) {
	case ed25519.PublicKey:
	case *ecdsa.PublicKey:
		switch key.Curve {
		case elliptic.P256(), elliptic.P384(), elliptic.P521():
		default:
			return types.NewErrBadRequest("unsupported ECDSA curve %s: publicKey must be an ECDSA P-256/P-384/P-521 or Ed25519 key", key.Curve.Params().Name)
		}
	default:
		return types.NewErrBadRequest("unsupported public key type %T: publicKey must be an ECDSA P-256/P-384/P-521 or Ed25519 key", pub)
	}

	device, err := req.GatewayClient.EnrollDevice(req.Context(), gateway.DeviceEnrollment{
		DeviceID:        in.DeviceID,
		MDMDeploymentID: deploymentID,
		PublicKey:       in.PublicKey,
		Hostname:        in.Hostname,
		OS:              in.OS,
		OSVersion:       in.OSVersion,
	})
	if err != nil {
		return types.NewErrBadRequest("%v", err)
	}

	return req.WriteCreated(types.DeviceEnrollResponse{Device: gtypes.ConvertDevice(*device)})
}

func uintFromExtra(extra map[string][]string, key string) (uint, bool) {
	vals := extra[key]
	if len(vals) == 0 {
		return 0, false
	}
	id, err := strconv.ParseUint(vals[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}
