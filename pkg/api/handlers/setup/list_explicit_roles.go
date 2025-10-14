package setup

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

type ExplicitRoleEmailsResponse struct {
	Owners []string `json:"owners"`
	Admins []string `json:"admins"`
}

// ListExplicitRoleEmails returns all emails with explicit Owner or Admin roles.
// This is informational only - the bootstrap user can choose to log in as any user,
// not just those on these lists.
// Endpoint: GET /api/setup/explicit-role-emails
func (h *Handler) ListExplicitRoleEmails(req api.Context) error {
	if err := h.requireBootstrapEnabled(req); err != nil {
		return err
	}

	if err := h.requireBootstrap(req); err != nil {
		return err
	}

	emailRoles := h.gatewayClient.GetExplicitRoleEmails()

	var owners, admins []string
	for email, role := range emailRoles {
		if role.HasRole(types.RoleOwner) {
			owners = append(owners, email)
		} else if role.HasRole(types.RoleAdmin) {
			admins = append(admins, email)
		}
	}

	return req.Write(ExplicitRoleEmailsResponse{
		Owners: owners,
		Admins: admins,
	})
}
