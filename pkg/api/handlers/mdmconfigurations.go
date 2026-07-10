package handlers

import (
	"errors"
	"strconv"
	"strings"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// MDMConfigurationsHandler serves the admin API for MDM configurations — the
// fleet principals that devices enroll into — and their enrollment keys.
type MDMConfigurationsHandler struct{}

func NewMDMConfigurationsHandler() *MDMConfigurationsHandler {
	return &MDMConfigurationsHandler{}
}

type mdmConfigurationCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// mdmConfigurationCreateResponse returns a new configuration together with the
// enrollment credential of its first key. The plaintext credential is only
// visible here.
type mdmConfigurationCreateResponse struct {
	Configuration        gtypes.MDMConfiguration `json:"configuration"`
	EnrollmentCredential string                  `json:"enrollmentCredential"`
}

type mdmConfigurationListResponse struct {
	Items []gtypes.MDMConfiguration `json:"items"`
}

type enrollmentKeyListResponse struct {
	Items []gtypes.DeviceEnrollmentKey `json:"items"`
}

type enrollmentKeyCreateRequest struct {
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Create handles POST /api/mdm/configurations. Creating a configuration mints its
// first enrollment key; the plaintext credential is returned once.
func (*MDMConfigurationsHandler) Create(req api.Context) error {
	var in mdmConfigurationCreateRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return types.NewErrBadRequest("name is required")
	}

	configuration, key, err := req.GatewayClient.CreateMDMConfiguration(req.Context(), req.UserID(), in.Name, in.Description)
	if err != nil {
		return err
	}
	return req.WriteCreated(mdmConfigurationCreateResponse{
		Configuration:        *configuration,
		EnrollmentCredential: key.EnrollmentCredential,
	})
}

// List handles GET /api/mdm/configurations.
func (*MDMConfigurationsHandler) List(req api.Context) error {
	configurations, err := req.GatewayClient.ListMDMConfigurations(req.Context())
	if err != nil {
		return err
	}
	return req.Write(mdmConfigurationListResponse{Items: configurations})
}

// Get handles GET /api/mdm/configurations/{id}.
func (*MDMConfigurationsHandler) Get(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	configuration, err := req.GatewayClient.GetMDMConfiguration(req.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("MDM configuration %d not found", id)
		}
		return err
	}
	return req.Write(configuration)
}

// Delete handles DELETE /api/mdm/configurations/{id}. Removes the configuration and
// its enrollment keys; devices enrolled into it are preserved.
func (*MDMConfigurationsHandler) Delete(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	return req.GatewayClient.DeleteMDMConfiguration(req.Context(), id)
}

// ListEnrollmentKeys handles GET /api/mdm/configurations/{id}/enrollment-keys.
func (*MDMConfigurationsHandler) ListEnrollmentKeys(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	keys, err := req.GatewayClient.ListDeviceEnrollmentKeys(req.Context(), id)
	if err != nil {
		return err
	}
	return req.Write(enrollmentKeyListResponse{Items: keys})
}

// CreateEnrollmentKey handles POST /api/mdm/configurations/{id}/enrollment-keys.
// It attaches an additional key without disturbing existing keys or enrolled
// devices. The plaintext credential is returned once.
func (*MDMConfigurationsHandler) CreateEnrollmentKey(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	var in enrollmentKeyCreateRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	key, err := req.GatewayClient.CreateDeviceEnrollmentKey(req.Context(), id, req.UserID(), in.Name, in.ExpiresAt)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("MDM configuration %d not found", id)
		}
		return err
	}
	return req.WriteCreated(key)
}

// DeleteEnrollmentKey handles DELETE /api/mdm/configurations/{id}/enrollment-keys/{key_id}.
// It only stops that key from enrolling new devices; enrolled devices are
// unaffected. Deletion is scoped to the configuration in the path and idempotent.
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

// ListDevices handles GET /api/mdm/configurations/{id}/devices.
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
	for _, d := range devices {
		items = append(items, gtypes.ConvertDevice(d))
	}
	return req.Write(types.DeviceList{Items: items})
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
