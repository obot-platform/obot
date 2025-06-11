package mcp

import (
	"context"
	"fmt"

	"github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ListPrompts(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) ([]mcp.Prompt, error) {
	config, err := sm.transformServerConfig(ctx, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	client, err := sm.local.Client(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.ListPrompts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP prompts: %w", err)
	}

	return resp.Prompts, nil
}

func (sm *SessionManager) GetPrompt(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig, name string, args map[string]string) ([]mcp.PromptMessage, string, error) {
	config, err := sm.transformServerConfig(ctx, mcpServer, serverConfig)
	if err != nil {
		return nil, "", err
	}

	client, err := sm.local.Client(config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.GetPrompt(ctx, name, args)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get MCP prompt: %w", err)
	}

	return resp.Messages, resp.Description, nil
}
