package handlers

import (
	"errors"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
)

type MCPCapacityHandler struct {
	capacity *mcp.ComputeCapacity
}

func NewMCPCapacityHandler(capacity *mcp.ComputeCapacity) *MCPCapacityHandler {
	return &MCPCapacityHandler{
		capacity: capacity,
	}
}

// GetCapacity returns capacity information for the MCP namespace.
// This endpoint is admin/owner-only.
func (h *MCPCapacityHandler) GetCapacity(req api.Context) error {
	info, err := h.capacity.GetCapacityInfo(req.Context())
	if err != nil {
		// If backend doesn't support capacity info (e.g., Docker), return bad request.
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrBadRequest("%s", nse.Error())
		}
		return err
	}

	return req.Write(info)
}
