package handlers

import (
	"errors"
	"strconv"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// MDMConfigurationsHandler serves the admin API for MDM configurations and
// their enrollment keys.
type MDMConfigurationsHandler struct {
	serverURL string
}

func NewMDMConfigurationsHandler(serverURL string) *MDMConfigurationsHandler {
	return &MDMConfigurationsHandler{serverURL: serverURL}
}

// Create handles POST /api/mdm/configurations. The optional asset selection and
// first enrollment key are committed with the configuration.
func (h *MDMConfigurationsHandler) Create(req api.Context) error {
	var in types.MDMConfiguration
	if err := req.Read(&in); err != nil {
		return err
	}
	configuration, err := h.mdmConfigurationForSave(req, in, nil)
	if err != nil {
		return err
	}

	created, key, err := req.GatewayClient.CreateMDMConfiguration(req.Context(), req.UserID(), configuration)
	if err != nil {
		return err
	}
	return req.WriteCreated(types.MDMConfigurationCreateResponse{
		MDMConfiguration:     h.mdmConfigurationFromGateway(req, *created, true),
		EnrollmentCredential: key.EnrollmentCredential,
	})
}

// List handles GET /api/mdm/configurations.
func (h *MDMConfigurationsHandler) List(req api.Context) error {
	configurations, err := req.GatewayClient.ListMDMConfigurations(req.Context())
	if err != nil {
		return err
	}
	items := make([]types.MDMConfiguration, 0, len(configurations))
	for _, configuration := range configurations {
		items = append(items, h.mdmConfigurationFromGateway(req, configuration, false))
	}
	return req.Write(types.MDMConfigurationList{Items: items})
}

// Get handles GET /api/mdm/configurations/{id}.
func (h *MDMConfigurationsHandler) Get(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	configuration, err := req.GatewayClient.GetMDMConfiguration(req.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("MDM configuration %d not found", id)
	}
	if err != nil {
		return err
	}
	return req.Write(h.mdmConfigurationFromGateway(req, *configuration, true))
}

// Update handles PUT /api/mdm/configurations/{id}. Supplying a blank asset
// digest, platform, and OS clears the optional asset selection.
func (h *MDMConfigurationsHandler) Update(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	current, err := h.getConfiguration(req, id)
	if err != nil {
		return err
	}
	var in types.MDMConfiguration
	if err := req.Read(&in); err != nil {
		return err
	}
	configuration, err := h.mdmConfigurationForSave(req, in, current)
	if err != nil {
		return err
	}
	configuration.ID = id
	if err := req.GatewayClient.UpdateMDMConfiguration(req.Context(), configuration); errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("MDM configuration %d not found", id)
	} else if err != nil {
		return err
	}
	updated, err := req.GatewayClient.GetMDMConfiguration(req.Context(), id)
	if err != nil {
		return err
	}
	return req.Write(h.mdmConfigurationFromGateway(req, *updated, true))
}

// Delete removes the configuration and its keys. Enrolled devices remain.
func (*MDMConfigurationsHandler) Delete(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	return req.GatewayClient.DeleteMDMConfiguration(req.Context(), id)
}

func (*MDMConfigurationsHandler) ListEnrollmentKeys(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	keys, err := req.GatewayClient.ListDeviceEnrollmentKeys(req.Context(), id)
	if err != nil {
		return err
	}
	items := make([]types.MDMEnrollmentKey, 0, len(keys))
	for _, key := range keys {
		items = append(items, mdmEnrollmentKeyFromGateway(key))
	}
	return req.Write(types.MDMEnrollmentKeyList{Items: items})
}

func (*MDMConfigurationsHandler) CreateEnrollmentKey(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	var in types.MDMEnrollmentKeyCreateRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	var expiresAt *time.Time
	if in.ExpiresAt != nil {
		value := in.ExpiresAt.GetTime()
		expiresAt = &value
	}
	key, err := req.GatewayClient.CreateDeviceEnrollmentKey(req.Context(), id, req.UserID(), in.Name, expiresAt)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("MDM configuration %d not found", id)
	}
	if err != nil {
		return err
	}
	return req.WriteCreated(types.MDMEnrollmentKeyCreateResponse{
		MDMEnrollmentKey:     mdmEnrollmentKeyFromGateway(key.DeviceEnrollmentKey),
		EnrollmentCredential: key.EnrollmentCredential,
	})
}

func (*MDMConfigurationsHandler) DeleteEnrollmentKey(req api.Context) error {
	configurationID, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	keyID, err := parsePathUint(req, "key_id", "enrollment key id")
	if err != nil {
		return err
	}
	return req.GatewayClient.DeleteDeviceEnrollmentKey(req.Context(), configurationID, keyID)
}

func (*MDMConfigurationsHandler) ListDevices(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	devices, err := req.GatewayClient.ListDevices(req.Context(), id)
	if err != nil {
		return err
	}
	items := make([]types.Device, 0, len(devices))
	for _, device := range devices {
		items = append(items, gtypes.ConvertDevice(device))
	}
	return req.Write(types.DeviceList{Items: items})
}

func mdmEnrollmentKeyFromGateway(key gtypes.DeviceEnrollmentKey) types.MDMEnrollmentKey {
	return types.MDMEnrollmentKey{
		ID:         key.ID,
		Name:       key.Name,
		CreatedAt:  *types.NewTime(key.CreatedAt),
		LastUsedAt: optionalAPITime(key.LastUsedAt),
		ExpiresAt:  optionalAPITime(key.ExpiresAt),
	}
}

func optionalAPITime(value *time.Time) *types.Time {
	if value == nil {
		return nil
	}
	return types.NewTime(*value)
}

func configurationIDFromPath(req api.Context) (uint, error) {
	return parsePathUint(req, "id", "MDM configuration id")
}

func parsePathUint(req api.Context, param, label string) (uint, error) {
	raw := req.PathValue(param)
	if raw == "" {
		return 0, types.NewErrBadRequest("missing %s", label)
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, types.NewErrBadRequest("invalid %s: %v", label, err)
	}
	return uint(id), nil
}
