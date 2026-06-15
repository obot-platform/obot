package setup

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/bootstrap"
)

type Handler struct {
	serverURL        string
	bootstrapEnabler *bootstrap.Bootstrap
}

func NewHandler(serverURL string, bootstrapEnabler *bootstrap.Bootstrap) *Handler {
	return &Handler{
		serverURL:        serverURL,
		bootstrapEnabler: bootstrapEnabler,
	}
}

// requireBootstrap checks if the request is from the bootstrap user.
// Returns an error if not authenticated as bootstrap.
func (h *Handler) requireBootstrap(req api.Context) error {
	// Check if user is bootstrap user
	if req.User.GetName() != "bootstrap" {
		log.Infof("Denied setup endpoint for non-bootstrap user")
		return types.NewErrHTTP(http.StatusForbidden,
			"this endpoint requires bootstrap authentication")
	}
	return nil
}

// requireBootstrapEnabled checks if bootstrap mode is enabled.
// Returns 404 if bootstrap is disabled.
func (h *Handler) requireBootstrapEnabled(req api.Context) error {
	if h.bootstrapEnabler == nil {
		return fmt.Errorf("bootstrap enabler is not set")
	}

	enabled, err := h.bootstrapEnabler.SetupEnabled(req.Context())
	if err != nil {
		return err
	}
	if !enabled {
		log.Infof("Rejected setup endpoint because bootstrap mode is disabled")
		return types.NewErrHTTP(http.StatusNotFound, "not found")
	}

	return nil
}
