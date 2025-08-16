package mcp

import (
	"context"
	"fmt"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
)

// GetServerDetails will get the details of a specific MCP server based on its configuration, if the backend supports it.
// If the server is remote, it will return an error as remote servers do not support this operation.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) GetServerDetails(ctx context.Context, mcpServerDisplayName, mcpServerName string, serverConfig ServerConfig) (types.MCPServerDetails, error) {
	if serverConfig.Runtime == types.RuntimeRemote {
		return types.MCPServerDetails{}, fmt.Errorf("getting server details is not supported for remote servers")
	}

	id := deploymentID(serverConfig)

	_, err := sm.ensureDeployment(ctx, id, serverConfig, mcpServerDisplayName, mcpServerName)
	if err != nil {
		return types.MCPServerDetails{}, err
	}

	return sm.backend.getServerDetails(ctx, id)
}

// StreamServerLogs will stream the logs of a specific MCP server based on its configuration, if the backend supports it.
// If the server is remote, it will return an error as remote servers do not support this operation.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) StreamServerLogs(ctx context.Context, mcpServerDisplayName, mcpServerName string, serverConfig ServerConfig) (io.ReadCloser, error) {
	if serverConfig.Runtime == types.RuntimeRemote {
		return nil, fmt.Errorf("streaming logs is not supported for remote servers")
	}

	id := deploymentID(serverConfig)

	_, err := sm.ensureDeployment(ctx, id, serverConfig, mcpServerDisplayName, mcpServerName)
	if err != nil {
		return nil, err
	}

	return sm.backend.streamServerLogs(ctx, id)
}
