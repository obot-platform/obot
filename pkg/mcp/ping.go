package mcp

import (
	"context"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
)

func (sm *SessionManager) PingServer(ctx context.Context, serverConfig ServerConfig) (*nmcp.PingResult, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	return client.Ping(ctx)
}
