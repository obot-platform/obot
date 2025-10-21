package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
)

// GetServerDetails will get the details of a specific MCP server based on its configuration, if the backend supports it.
// If the server is remote, it will return an error as remote servers do not support this operation.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) GetServerDetails(ctx context.Context, serverConfig ServerConfig) (types.MCPServerDetails, error) {
	if serverConfig.Runtime == types.RuntimeRemote {
		return types.MCPServerDetails{}, fmt.Errorf("getting server details is not supported for remote servers")
	}

	_, err := sm.ensureDeployment(ctx, serverConfig, true)
	if err != nil {
		return types.MCPServerDetails{}, err
	}

	return sm.backend.getServerDetails(ctx, serverConfig.MCPServerName)
}

// shouldAttemptLogStreaming checks if the error is one that should still have logs
// These are errors where the server might be running but not healthy
func shouldAttemptLogStreaming(err error) bool {
	for unwrappedErr := err; unwrappedErr != nil; unwrappedErr = errors.Unwrap(unwrappedErr) {
		switch unwrappedErr {
		case ErrHealthCheckFailed, ErrHealthCheckTimeout, ErrPodConfigurationFailed:
			return true
		}
	}
	return false
}

// StreamServerLogs will stream the logs of a specific MCP server based on its configuration, if the backend supports it.
// If the server is remote, it will return an error as remote servers do not support this operation.
// If the backend does not support the operation, it will return an [ErrNotSupportedByBackend] error.
func (sm *SessionManager) StreamServerLogs(ctx context.Context, serverConfig ServerConfig) (io.ReadCloser, error) {
	if serverConfig.Runtime == types.RuntimeRemote {
		return nil, fmt.Errorf("streaming logs is not supported for remote servers")
	}

	_, err := sm.ensureDeployment(ctx, serverConfig, true)
	if err != nil {
		// If error of deployment is not one that should still have logs, return the error
		if !shouldAttemptLogStreaming(err) {
			return nil, err
		}
	}

	return sm.backend.streamServerLogs(ctx, serverConfig.MCPServerName)
}
