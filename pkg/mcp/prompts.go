package mcp

import (
	"context"
	"fmt"

	"github.com/nanobot-ai/nanobot/pkg/mcp"
)

func (sm *SessionManager) ListPrompts(ctx context.Context, serverConfig ServerConfig) ([]mcp.Prompt, error) {
	client, err := sm.clientForMCPServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListPrompts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP prompts: %w", err)
	}

	return resp.Prompts, nil
}

func (sm *SessionManager) GetPrompt(ctx context.Context, serverConfig ServerConfig, name string, args map[string]string) ([]mcp.PromptMessage, string, error) {
	client, err := sm.clientForMCPServer(ctx, serverConfig)
	if err != nil {
		return nil, "", err
	}

	resp, err := client.GetPrompt(ctx, name, args)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get MCP prompt: %w", err)
	}

	return resp.Messages, resp.Description, nil
}
