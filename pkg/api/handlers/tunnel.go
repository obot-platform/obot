package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

type tunnelConnector interface {
	ConnectionsContext(context.Context) ([]types.TunnelConnection, error)
	ServeConnect(http.ResponseWriter, *http.Request, string)
	ServePeer(http.ResponseWriter, *http.Request)
}

type TunnelHandler struct {
	connector tunnelConnector
}

func NewTunnelHandler(tunnels tunnelConnector) *TunnelHandler {
	return &TunnelHandler{
		connector: tunnels,
	}
}

// List returns the tunnels currently connected to this Obot installation.
func (h *TunnelHandler) List(req api.Context) error {
	connections, err := h.connector.ConnectionsContext(req.Context())
	if err != nil {
		return fmt.Errorf("failed to list tunnel connections: %w", err)
	}
	return req.Write(types.TunnelConnectionList{Items: connections})
}

// Connect upgrades the request to the tunnel identified by its authenticated
// tunnel principal.
func (h *TunnelHandler) Connect(req api.Context) error {
	if h.connector == nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "tunnel manager is not configured")
	}
	if req.User == nil {
		return types.NewErrHTTP(http.StatusUnauthorized, "tunnel principal is required")
	}
	h.connector.ServeConnect(req.ResponseWriter, req.Request, req.User.GetUID())
	return nil
}

// Peer upgrades an authenticated connection from another Obot replica.
func (h *TunnelHandler) Peer(req api.Context) error {
	if h.connector == nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "tunnel manager is not configured")
	}
	h.connector.ServePeer(req.ResponseWriter, req.Request)
	return nil
}
