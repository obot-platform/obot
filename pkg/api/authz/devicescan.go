package authz

import (
	"errors"
	"net/http"
	"strconv"

	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (a *Authorizer) checkDeviceScan(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.DeviceScanID == "" || u.IsAdmin || u.IsAuditor {
		return true, nil
	}

	id, err := strconv.ParseUint(resources.DeviceScanID, 10, 64)
	if err != nil {
		return false, err
	}

	scan, err := a.gatewayClient.GetDeviceScan(req.Context(), uint(id))
	if err != nil {
		return false, err
	}

	// An enrolled device owns only the scans it uploaded as itself: device
	// uploads have an empty SubmittedBy, and requiring that keeps a human scan
	// whose user-controlled manifest DeviceID matches from granting access.
	if deviceID := gtypes.PrincipalDeviceID(u); deviceID != "" {
		if scan.SubmittedBy != "" || scan.DeviceID != deviceID {
			return false, nil
		}

		device, err := a.gatewayClient.GetDeviceByDeviceID(req.Context(), deviceID)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			// The device was deleted after it authenticated, so it owns no scans.
			return false, nil
		case err != nil:
			return false, err
		}

		// A deleted device's kept scans stay under its DeviceID, and anyone with
		// an enrollment token can enroll that ID again with a new key. Only scans
		// received during the current enrollment belong to this device.
		// Re-enrolling an existing device row keeps EnrolledAt, but after the
		// device is deleted any re-enroll, with the same key or not, starts a new
		// enrollment and hides the earlier scans, because the old owner can no
		// longer be told apart from a new one.
		// shortcut: EnrolledAt and scan CreatedAt come from possibly different
		// replicas' clocks, so skew can hide a device's first scans (this mostly
		// fails closed; only scans uploaded within the skew of a
		// delete-then-re-enroll could stay visible); stamp scans with the
		// enrolling devices.id if devices report missing first scans.
		return !scan.CreatedAt.Before(device.EnrolledAt), nil
	}

	// If the user submitted the scan, then authorization is granted.
	return scan.SubmittedBy == u.GetUID(), nil
}
