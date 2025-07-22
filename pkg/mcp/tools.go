package mcp

import (
	"context"
	"fmt"

	"github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ListTools(ctx context.Context, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) ([]mcp.Tool, error) {
	client, err := sm.ClientForMCPServer(ctx, userID, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP prompts: %w", err)
	}

	return resp.Tools, nil
}
