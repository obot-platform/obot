package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func (sm *SessionManager) ListPrompts(ctx context.Context, server ServerConfig) ([]mcp.Prompt, error) {
	client, err := sm.local.Client(server.ServerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP prompts: %w", err)
	}

	return resp.Prompts, nil
}

func (sm *SessionManager) GetPrompt(ctx context.Context, server ServerConfig, name string, args map[string]string) ([]mcp.PromptMessage, string, error) {
	client, err := sm.local.Client(server.ServerConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.GetPrompt(ctx, mcp.GetPromptRequest{
		Params: struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments,omitempty"`
		}{Name: name, Arguments: args},
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get MCP prompt: %w", err)
	}

	return resp.Messages, resp.Description, nil
}
