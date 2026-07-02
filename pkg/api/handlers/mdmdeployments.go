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

// MDMDeploymentsHandler serves the admin API for MDM deployments — the
// fleet principals that devices enroll into — and their enrollment keys.
type MDMDeploymentsHandler struct{}

func NewMDMDeploymentsHandler() *MDMDeploymentsHandler {
	return &MDMDeploymentsHandler{}
}

type mdmDeploymentCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// mdmDeploymentCreateResponse returns a new deployment together with the
// enrollment credential of its first key. The plaintext credential is only
// visible here.
type mdmDeploymentCreateResponse struct {
	Deployment           gtypes.MDMDeployment `json:"deployment"`
	EnrollmentCredential string               `json:"enrollmentCredential"`
}

type mdmDeploymentListResponse struct {
	Items []gtypes.MDMDeployment `json:"items"`
}

type enrollmentKeyListResponse struct {
	Items []gtypes.DeviceEnrollmentKey `json:"items"`
}

type enrollmentKeyCreateRequest struct {
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Create handles POST /api/mdm/deployments. Creating a deployment mints its
// first enrollment key; the plaintext credential is returned once.
func (*MDMDeploymentsHandler) Create(req api.Context) error {
	var in mdmDeploymentCreateRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return types.NewErrBadRequest("name is required")
	}

	deployment, key, err := req.GatewayClient.CreateMDMDeployment(req.Context(), req.UserID(), in.Name, in.Description)
	if err != nil {
		return err
	}
	return req.WriteCreated(mdmDeploymentCreateResponse{
		Deployment:           *deployment,
		EnrollmentCredential: key.EnrollmentCredential,
	})
}

// List handles GET /api/mdm/deployments.
func (*MDMDeploymentsHandler) List(req api.Context) error {
	deployments, err := req.GatewayClient.ListMDMDeployments(req.Context())
	if err != nil {
		return err
	}
	return req.Write(mdmDeploymentListResponse{Items: deployments})
}

// Get handles GET /api/mdm/deployments/{id}.
func (*MDMDeploymentsHandler) Get(req api.Context) error {
	id, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	deployment, err := req.GatewayClient.GetMDMDeployment(req.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("MDM deployment %d not found", id)
		}
		return err
	}
	return req.Write(deployment)
}

// Delete handles DELETE /api/mdm/deployments/{id}. Removes the deployment and
// its enrollment keys; devices enrolled into it are preserved.
func (*MDMDeploymentsHandler) Delete(req api.Context) error {
	id, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	return req.GatewayClient.DeleteMDMDeployment(req.Context(), id)
}

// ListEnrollmentKeys handles GET /api/mdm/deployments/{id}/enrollment-keys.
func (*MDMDeploymentsHandler) ListEnrollmentKeys(req api.Context) error {
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

// CreateEnrollmentKey handles POST /api/mdm/deployments/{id}/enrollment-keys.
// It attaches an additional key without disturbing existing keys or enrolled
// devices. The plaintext credential is returned once.
func (*MDMDeploymentsHandler) CreateEnrollmentKey(req api.Context) error {
	id, err := deploymentIDFromPath(req)
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
			return types.NewErrNotFound("MDM deployment %d not found", id)
		}
		return err
	}
	return req.WriteCreated(key)
}

// DeleteEnrollmentKey handles DELETE /api/mdm/deployments/{id}/enrollment-keys/{key_id}.
// It only stops that key from enrolling new devices; enrolled devices are
// unaffected. Deletion is scoped to the deployment in the path and idempotent.
func (*MDMDeploymentsHandler) DeleteEnrollmentKey(req api.Context) error {
	deploymentID, err := deploymentIDFromPath(req)
	if err != nil {
		return err
	}
	keyID, err := parsePathUint(req, "key_id", "enrollment key id")
	if err != nil {
		return err
	}
	return req.GatewayClient.DeleteDeviceEnrollmentKey(req.Context(), deploymentID, keyID)
}

// ListDevices handles GET /api/mdm/deployments/{id}/devices.
func (*MDMDeploymentsHandler) ListDevices(req api.Context) error {
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

func deploymentIDFromPath(req api.Context) (uint, error) {
	return parsePathUint(req, "id", "MDM deployment id")
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
