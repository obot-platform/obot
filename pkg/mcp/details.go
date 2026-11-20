package mcp

import (
	"context"
	"errors"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
)

var (
	// ErrServerNotRunning is returned when an MCP server is not running
	ErrServerNotRunning = errors.New("mcp server is not running")
)

// GetRunningServerDetails inspects an existing deployment using its server name.
func (sm *SessionManager) GetRunningServerDetails(ctx context.Context, serverName string) (types.MCPServerDetails, error) {
	return sm.backend.getServerDetails(ctx, serverName)
}

// StreamRunningServerLogs opens logs for an existing deployment using its server name.
func (sm *SessionManager) StreamRunningServerLogs(ctx context.Context, serverName string) (io.ReadCloser, error) {
	if _, err := sm.GetRunningServerDetails(ctx, serverName); err != nil {
		return nil, err
	}
	return sm.backend.streamServerLogs(ctx, serverName)
}

// GetOrStartServerDetails deploys a stopped server before returning its details.
func (sm *SessionManager) GetOrStartServerDetails(ctx context.Context, config ServerConfig) (types.MCPServerDetails, error) {
	details, err := sm.GetRunningServerDetails(ctx, config.MCPServerName)
	if !errors.Is(err, ErrServerNotRunning) {
		return details, err
	}
	if err := sm.deployServer(ctx, config); err != nil {
		return types.MCPServerDetails{}, err
	}
	return sm.GetRunningServerDetails(ctx, config.MCPServerName)
}

// StartServerIfNeededAndStreamLogs deploys a stopped server before streaming its logs.
func (sm *SessionManager) StartServerIfNeededAndStreamLogs(ctx context.Context, config ServerConfig) (io.ReadCloser, error) {
	_, err := sm.GetRunningServerDetails(ctx, config.MCPServerName)
	if errors.Is(err, ErrServerNotRunning) {
		err = sm.deployServer(ctx, config)
	}
	if err != nil {
		return nil, err
	}
	return sm.backend.streamServerLogs(ctx, config.MCPServerName)
}
