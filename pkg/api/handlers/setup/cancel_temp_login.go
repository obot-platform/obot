package setup

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

type CancelTempLoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CancelTempLogin removes the temporary user from the cache and optionally
// demotes the user in the database.
// Endpoint: POST /api/setup/cancel-temp-login
func (h *Handler) CancelTempLogin(req api.Context) error {
	if err := h.requireBootstrapEnabled(req); err != nil {
		return err
	}

	if err := h.requireBootstrap(req); err != nil {
		return err
	}

	cached := req.GatewayClient.GetTempUserCache()
	if cached == nil {
		return types.NewErrHTTP(http.StatusNotFound, "no temporary user to cancel")
	}

	// Get the user from the database
	user, err := req.GatewayClient.UserByID(req.Context(), fmt.Sprintf("%d", cached.UserID))
	if err != nil {
		// If user doesn't exist, just clear cache
		req.GatewayClient.ClearTempUserCache()
		return req.Write(CancelTempLoginResponse{
			Success: true,
			Message: "Temporary login cancelled",
		})
	}

	// Check if the user has an explicit role from environment variables
	// If they do, don't demote them
	explicitRole := req.GatewayClient.HasExplicitRole(user.Email)
	if !explicitRole.HasRole(types.RoleAdmin) {
		// Demote user to Basic role (don't delete, as they may have logged in legitimately)
		if user.Role != types.RoleBasic {
			user.Role = types.RoleBasic
			if _, err := req.GatewayClient.UpdateUser(req.Context(), true, user, fmt.Sprintf("%d", user.ID)); err != nil {
				return fmt.Errorf("failed to demote user: %w", err)
			}
		}
	}

	// Clear the temporary cache
	req.GatewayClient.ClearTempUserCache()

	return req.Write(CancelTempLoginResponse{
		Success: true,
		Message: fmt.Sprintf("Temporary login for %s cancelled", user.Email),
	})
}
