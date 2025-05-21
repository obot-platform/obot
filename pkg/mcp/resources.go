package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func (sm *SessionManager) ListResources(ctx context.Context, server ServerConfig) ([]mcp.Resource, error) {
	client, err := sm.local.Client(server.ServerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP resources: %w", err)
	}

	return resp.Resources, nil
}

func (sm *SessionManager) ReadResource(ctx context.Context, server ServerConfig, uri string) ([]mcp.ResourceContents, error) {
	client, err := sm.local.Client(server.ServerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: struct {
			URI       string         `json:"uri"`
			Arguments map[string]any `json:"arguments,omitempty"`
		}{URI: uri},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP resource: %w", err)
	}

	return resp.Contents, nil
}
