package setup

import (
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

// requireBootstrap checks if the request is from the bootstrap user.
// Returns an error if not authenticated as bootstrap.
func (h *Handler) requireBootstrap(req api.Context) error {
	// Check if user is bootstrap user
	if req.User.GetName() != "bootstrap" {
		return types.NewErrHTTP(http.StatusForbidden,
			"this endpoint requires bootstrap authentication")
	}
	return nil
}

// requireBootstrapEnabled checks if bootstrap mode is enabled.
// Returns 404 if bootstrap is disabled.
func (h *Handler) requireBootstrapEnabled(req api.Context) error {
	// Query all Owner users
	adminUsers, err := req.GatewayClient.Users(req.Context(), gwtypes.UserQuery{
		Role: types.RoleOwner,
	})
	if err != nil {
		return err
	}

	// Check if any non-bootstrap Owner with email exists
	for _, u := range adminUsers {
		if u.Username != "bootstrap" && u.Email != "" {
			// Bootstrap is disabled - return 404
			return types.NewErrHTTP(http.StatusNotFound, "not found")
		}
	}

	// Bootstrap is enabled
	return nil
}
