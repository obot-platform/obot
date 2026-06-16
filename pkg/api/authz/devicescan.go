package authz

import (
	"net/http"
	"strconv"
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

	// If the user submitted the scan, then authorization is granted.
	return scan.SubmittedBy == u.GetUID(), nil
}
