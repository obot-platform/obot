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

// DeviceDeploymentsHandler serves the admin API for device deployments — the
// fleet principals that devices enroll into — and their enrollment keys.
type DeviceDeploymentsHandler struct{}

func NewDeviceDeploymentsHandler() *DeviceDeploymentsHandler {
	return &DeviceDeploymentsHandler{}
}

type deviceDeploymentCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// deviceDeploymentCreateResponse returns a new deployment together with the
// enrollment credential of its first key. The plaintext credential is only
// visible here.
type deviceDeploymentCreateResponse struct {
	Deployment           gtypes.DeviceDeployment `json:"deployment"`
	EnrollmentCredential string                  `json:"enrollmentCredential"`
}

type deviceDeploymentListResponse struct {
	Items []gtypes.DeviceDeployment `json:"items"`
}

type enrollmentKeyListResponse struct {
	Items []gtypes.DeviceEnrollmentKey `json:"items"`
}

type enrollmentKeyCreateRequest struct {
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Create handles POST /api/device-deployments. Creating a deployment mints its
// first enrollment key; the plaintext credential is returned once.
func (*DeviceDeploymentsHandler) Create(req api.Context) error {
	var in deviceDeploymentCreateRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	if in.Name == "" {
		return types.NewErrBadRequest("name is required")
	}
	createdBy, err := callerUserID(req)
	if err != nil {
		return err
	}

	deployment, key, err := req.GatewayClient.CreateDeviceDeployment(req.Context(), createdBy, in.Name, in.Description)
	if err != nil {
		return err
	}
	return req.WriteCreated(deviceDeploymentCreateResponse{
		Deployment:           *deployment,
		EnrollmentCredential: key.EnrollmentCredential,
	})
}

// List handles GET /api/device-deployments.
func (*DeviceDeploymentsHandler) List(req api.Context) error {
	deployments, err := req.GatewayClient.ListDeviceDeployments(req.Context())
	if err != nil {
		return err
	}
	return req.Write(deviceDeploymentListResponse{Items: deployments})
}

// Get handles GET /api/device-deployments/{id}.
func (*DeviceDeploymentsHandler) Get(req api.Context) error {
	id, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	deployment, err := req.GatewayClient.GetDeviceDeployment(req.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("device deployment %d not found", id)
		}
		return err
	}
	return req.Write(deployment)
}

// Delete handles DELETE /api/device-deployments/{id}. Removes the deployment and
// its enrollment keys; devices enrolled into it are preserved.
func (*DeviceDeploymentsHandler) Delete(req api.Context) error {
	id, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	return req.GatewayClient.DeleteDeviceDeployment(req.Context(), id)
}

// ListEnrollmentKeys handles GET /api/device-deployments/{id}/enrollment-keys.
func (*DeviceDeploymentsHandler) ListEnrollmentKeys(req api.Context) error {
	id, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	keys, err := req.GatewayClient.ListDeviceEnrollmentKeys(req.Context(), id)
	if err != nil {
		return err
	}
	return req.Write(enrollmentKeyListResponse{Items: keys})
}

// CreateEnrollmentKey handles POST /api/device-deployments/{id}/enrollment-keys.
// It attaches an additional key without disturbing existing keys or enrolled
// devices. The plaintext credential is returned once.
func (*DeviceDeploymentsHandler) CreateEnrollmentKey(req api.Context) error {
	id, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	createdBy, err := callerUserID(req)
	if err != nil {
		return err
	}
	var in enrollmentKeyCreateRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	key, err := req.GatewayClient.CreateDeviceEnrollmentKey(req.Context(), id, createdBy, in.Name, in.ExpiresAt)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("device deployment %d not found", id)
		}
		return err
	}
	return req.WriteCreated(key)
}

// DeleteEnrollmentKey handles DELETE /api/device-deployments/{id}/enrollment-keys/{key_id}.
// It only stops that key from enrolling new devices; enrolled devices are
// unaffected. The key must belong to the deployment in the path, else NotFound.
func (*DeviceDeploymentsHandler) DeleteEnrollmentKey(req api.Context) error {
	deploymentID, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	keyID, err := parsePathUint(req, "key_id", "enrollment key id")
	if err != nil {
		return err
	}
	if err := req.GatewayClient.DeleteDeviceEnrollmentKey(req.Context(), deploymentID, keyID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("enrollment key %d not found in deployment %d", keyID, deploymentID)
		}
		return err
	}
	return nil
}

// ListDevices handles GET /api/device-deployments/{id}/devices.
func (*DeviceDeploymentsHandler) ListDevices(req api.Context) error {
	id, err := deploymentIDFromPath(req)
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

// callerUserID returns the authenticated user's gateway DB id. Admin/owner
// principals (the only callers authz allows here) carry the numeric id as UID.
func callerUserID(req api.Context) (uint, error) {
	id, err := strconv.ParseUint(req.User.GetUID(), 10, 64)
	if err != nil {
		return 0, types.NewErrBadRequest("caller is not a user")
	}
	return uint(id), nil
}

func deploymentIDFromPath(req api.Context) (uint, error) {
	return parsePathUint(req, "id", "device deployment id")
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
