package handlers

import (
	"errors"
	"net/http"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// MDMDevicesHandler serves the admin API for enrolled devices across all MDM
// configurations. These are the devices that count toward the license's device limit,
// unlike the scan-derived device views.
type MDMDevicesHandler struct{}

func NewMDMDevicesHandler() *MDMDevicesHandler {
	return &MDMDevicesHandler{}
}

// List handles GET /api/mdm/devices.
func (*MDMDevicesHandler) List(req api.Context) error {
	devices, err := req.GatewayClient.ListAllDevices(req.Context())
	if err != nil {
		return err
	}
	items := make([]types.Device, 0, len(devices))
	for _, device := range devices {
		items = append(items, gtypes.ConvertDevice(device))
	}
	return req.Write(types.DeviceList{Items: items})
}

// Delete handles DELETE /api/mdm/devices/{device_id}. It frees the device's seat and
// revokes its access. Its audit logs are kept, and so are its scans unless ?deleteScans=true.
func (*MDMDevicesHandler) Delete(req api.Context) error {
	var (
		deviceID    = req.PathValue("device_id")
		deleteScans = req.URL.Query().Get("deleteScans") == "true"
	)
	if err := req.GatewayClient.DeleteDevice(req.Context(), deviceID, deleteScans); errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("device %q not found", deviceID)
	} else if err != nil {
		return err
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}
