package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (sm *SessionManager) ListPrompts(ctx context.Context, serverConfig ServerConfig) ([]*gomcp.Prompt, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListPrompts(ctx, &gomcp.ListPromptsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP prompts: %w", err)
	}

	return resp.Prompts, nil
}

func (sm *SessionManager) GetPrompt(ctx context.Context, serverConfig ServerConfig, name string, args map[string]string) ([]*gomcp.PromptMessage, string, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, "", err
	}

	resp, err := client.GetPrompt(ctx, &gomcp.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get MCP prompt: %w", err)
	}

	return resp.Messages, resp.Description, nil
}
