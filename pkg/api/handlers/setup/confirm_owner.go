package setup

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

type ConfirmOwnerResponse struct {
	Success bool   `json:"success"`
	UserID  uint   `json:"userId"`
	Email   string `json:"email"`
	Message string `json:"message"`
}

// ConfirmOwner confirms the temporary user as a permanent Owner.
// The user is already in the database (created during OAuth), so we just
// ensure they have the Owner role and clear the cache.
// Endpoint: POST /api/setup/confirm-owner
func (h *Handler) ConfirmOwner(req api.Context) error {
	if err := h.requireBootstrap(req); err != nil {
		return err
	}

	cached := h.gatewayClient.GetTempUserCache()
	if cached == nil {
		return types.NewErrHTTP(http.StatusNotFound, "no temporary user to confirm")
	}

	// Get the user from the database
	user, err := h.gatewayClient.UserByID(req.Context(), fmt.Sprintf("%d", cached.UserID))
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if the user has an explicit role from environment variables
	explicitRole := h.gatewayClient.HasExplicitRole(user.Email)

	// Ensure user has Owner role
	// Note: If the user is a hardcoded Admin or Owner from environment variables,
	// we must respect that configuration and not override it.
	if !user.Role.HasRole(types.RoleOwner) {
		// Don't promote hardcoded Admins - that would override explicit configuration
		if explicitRole.HasRole(types.RoleAdmin) {
			return types.NewErrHTTP(http.StatusBadRequest,
				fmt.Sprintf("cannot promote user %s to Owner: user is configured as Admin via environment variables", user.Email))
		}

		// Update user role to Owner
		user.Role = user.Role.SwitchBaseRole(types.RoleOwner)

		// Update in database
		if _, err := h.gatewayClient.UpdateUser(req.Context(), true, user, fmt.Sprintf("%d", user.ID)); err != nil {
			return fmt.Errorf("failed to update user role: %w", err)
		}
	}

	// Clear the temporary cache
	h.gatewayClient.ClearTempUserCache()

	return req.Write(ConfirmOwnerResponse{
		Success: true,
		UserID:  user.ID,
		Email:   user.Email,
		Message: fmt.Sprintf("User %s confirmed as Owner", user.Email),
	})
}
