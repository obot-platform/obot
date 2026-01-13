package handlers

import (
	"errors"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
)

type MCPCapacityHandler struct {
	mcpSessionManager *mcp.SessionManager
}

func NewMCPCapacityHandler(mcpSessionManager *mcp.SessionManager) *MCPCapacityHandler {
	return &MCPCapacityHandler{
		mcpSessionManager: mcpSessionManager,
	}
}

// GetCapacity returns capacity information for the MCP namespace.
// This endpoint is admin/owner-only.
func (h *MCPCapacityHandler) GetCapacity(req api.Context) error {
	if !req.UserIsAdmin() && !req.UserIsOwner() {
		return types.NewErrForbidden("admin access required")
	}

	info, err := h.mcpSessionManager.GetCapacityInfo(req.Context())
	if err != nil {
		// If backend doesn't support capacity info (e.g., Docker), return empty info
		var notSupported *mcp.ErrNotSupportedByBackend
		if errors.As(err, &notSupported) {
			return req.Write(types.MCPCapacityInfo{
				Error: notSupported.Error(),
			})
		}
		return err
	}

	return req.Write(info)
}
