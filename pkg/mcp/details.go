package mcp

import (
	"context"
	"errors"
	"io"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
)

// ErrServerNotRunning is returned when an MCP server is not running
var ErrServerNotRunning = errors.New("mcp server is not running")

// GetServerDetails will get the details of a specific MCP server based on its configuration, if the backend supports it.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) GetServerDetails(ctx context.Context, serverConfig ServerConfig) (types.MCPServerDetails, error) {
	// Try to get details first - only deploy if server doesn't exist
	// This prevents unnecessary redeployments that would update K8s settings and clear the NeedsK8sUpdate flag
	details, err := sm.backend.getServerDetails(ctx, serverConfig.MCPServerName)
	if err == nil {
		return details, nil
	}

	// Only deploy if server is not running - for any other error, return it
	if !errors.Is(err, ErrServerNotRunning) {
		return types.MCPServerDetails{}, err
	}

	// Server not running - deploy it
	if err := sm.deployServer(ctx, serverConfig); err != nil {
		return types.MCPServerDetails{}, err
	}

	return sm.backend.getServerDetails(ctx, serverConfig.MCPServerName)
}

// StreamServerLogs will stream the logs of a specific MCP server based on its configuration, if the backend supports it.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) StreamServerLogs(ctx context.Context, serverConfig ServerConfig) (io.ReadCloser, error) {
	// Check if server exists first - only deploy if it doesn't
	// This prevents unnecessary redeployments that would update K8s settings and clear the NeedsK8sUpdate flag
	_, err := sm.backend.getServerDetails(ctx, serverConfig.MCPServerName)
	if err == nil {
		return sm.backend.streamServerLogs(ctx, serverConfig.MCPServerName)
	}

	// Only deploy if server is not running - for any other error, return it
	if !errors.Is(err, ErrServerNotRunning) {
		return nil, err
	}

	// Server not running - deploy it
	if err := sm.deployServer(ctx, serverConfig); err != nil {
		return nil, err
	}

	return sm.backend.streamServerLogs(ctx, serverConfig.MCPServerName)
}

func (sm *SessionManager) deployServer(ctx context.Context, server ServerConfig) error {
	var webhooks []Webhook
	if !server.ComponentMCPServer {
		// Don't get webhooks for servers that are components of composite servers.
		// The webhooks would be called at the composite level.
		var err error
		webhooks, err = sm.webhookHelper.GetWebhooksForMCPServer(ctx, sm.gptClient, server)
		if err != nil {
			return err
		}

		slices.SortFunc(webhooks, func(a, b Webhook) int {
			if a.Name < b.Name {
				return -1
			} else if a.Name > b.Name {
				return 1
			}
			return 0
		})
	}

	return sm.backend.deployServer(ctx, server, webhooks)
}
