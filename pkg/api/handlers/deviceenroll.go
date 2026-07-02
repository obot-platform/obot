package handlers

import (
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
	return &DeviceEnrollHandler{}
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

	if _, err := x509.ParsePKIXPublicKey(in.PublicKey); err != nil {
		return types.NewErrBadRequest("publicKey is not a valid PKIX public key: %v", err)
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
