package setup

import (
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
)

type Handler struct {
	gatewayClient *client.Client
}

func NewHandler(gc *client.Client) *Handler {
	return &Handler{
		gatewayClient: gc,
	}
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
