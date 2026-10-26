package handlers

import (
	"errors"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/billing"
	"github.com/obot-platform/obot/pkg/mcp"
)

type MCPCapacityHandler struct {
	mcpSessionManager *mcp.SessionManager
	billing           *billing.Subscription
}

func NewMCPCapacityHandler(mcpSessionManager *mcp.SessionManager, subscription *billing.Subscription) *MCPCapacityHandler {
	return &MCPCapacityHandler{
		mcpSessionManager: mcpSessionManager,
		billing:           subscription,
	}
}

// GetCapacity returns capacity information for the MCP namespace.
// This endpoint is admin/owner-only.
func (h *MCPCapacityHandler) GetCapacity(req api.Context) error {
	info, err := h.mcpSessionManager.GetCapacityInfo(req.Context())
	if err != nil {
		// If backend doesn't support capacity info (e.g., Docker), return bad request.
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrBadRequest("%s", nse.Error())
		}
		return err
	}
	// With billing, the plan sets the limits, and billing counts what hosted MCP servers request.
	if h.billing != nil {
		subscription, err := h.billing.Read(req.Context())
		if err != nil {
			return err
		}
		info = mcp.PlanCapacity(info, billing.EntitlementsOf(subscription.Status), &subscription.Status.Usage)
	}

	return req.Write(info)
}
