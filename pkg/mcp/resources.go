package mcp

import (
	"context"
	"fmt"

	"github.com/nanobot-ai/nanobot/pkg/mcp"
)

func (sm *SessionManager) ListResources(ctx context.Context, serverConfig ServerConfig) ([]mcp.Resource, error) {
	client, err := sm.clientForMCPServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP resources: %w", err)
	}

	return resp.Resources, nil
}

func (sm *SessionManager) ReadResource(ctx context.Context, serverConfig ServerConfig, uri string) ([]mcp.ResourceContent, error) {
	client, err := sm.clientForMCPServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ReadResource(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP resource: %w", err)
	}

	return resp.Contents, nil
}
