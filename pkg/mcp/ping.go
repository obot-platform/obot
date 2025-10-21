package mcp

import (
	"context"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
)

func (sm *SessionManager) PingServer(ctx context.Context, serverConfig ServerConfig) (*nmcp.PingResult, error) {
	client, err := sm.clientForMCPServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	return client.Ping(ctx)
}
