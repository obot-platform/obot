package mcp

import (
	"context"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ServerCapabilities(ctx context.Context, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) (nmcp.ServerCapabilities, error) {
	client, err := sm.ClientForMCPServer(ctx, userID, mcpServer, serverConfig)
	if err != nil {
		return nmcp.ServerCapabilities{}, err
	}

	return client.Capabilities(), nil
}
