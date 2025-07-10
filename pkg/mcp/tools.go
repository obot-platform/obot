package mcp

import (
	"context"
	"fmt"

	"github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ListTools(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) ([]mcp.Tool, error) {
	config, err := sm.transformServerConfig(ctx, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	client, err := sm.local.Client(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP prompts: %w", err)
	}

	return resp.Tools, nil
}
