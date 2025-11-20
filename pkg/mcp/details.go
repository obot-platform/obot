package mcp

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
)

// GetServerDetails will get the details of a specific MCP server based on its configuration, if the backend supports it.
// If the server is remote, it will return an error as remote servers do not support this operation.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) GetServerDetails(ctx context.Context, serverConfig ServerConfig) (types.MCPServerDetails, error) {
	if serverConfig.Runtime == types.RuntimeRemote {
		return types.MCPServerDetails{}, fmt.Errorf("getting server details is not supported for remote servers")
	}

	if err := sm.deployServer(ctx, serverConfig); err != nil {
		return types.MCPServerDetails{}, err
	}

	return sm.backend.getServerDetails(ctx, serverConfig.MCPServerName)
}

// StreamServerLogs will stream the logs of a specific MCP server based on its configuration, if the backend supports it.
// If the server is remote, it will return an error as remote servers do not support this operation.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) StreamServerLogs(ctx context.Context, serverConfig ServerConfig) (io.ReadCloser, error) {
	if serverConfig.Runtime == types.RuntimeRemote {
		return nil, fmt.Errorf("streaming logs is not supported for remote servers")
	}

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
