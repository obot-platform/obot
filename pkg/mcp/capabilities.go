package mcp

import (
	"context"
	"fmt"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ServerCapabilities(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) (nmcp.ServerCapabilities, error) {
	config, err := sm.transformServerConfig(ctx, mcpServer, serverConfig)
	if err != nil {
		return nmcp.ServerCapabilities{}, err
	}

	client, err := sm.local.Client(config)
	if err != nil {
		return nmcp.ServerCapabilities{}, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return client.Capabilities(), nil
}
