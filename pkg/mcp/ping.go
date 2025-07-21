package mcp

import (
	"context"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) PingServer(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) (*nmcp.PingResult, error) {
	client, err := sm.ClientForServer(ctx, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	return client.Ping(ctx)
}
