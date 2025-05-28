package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ListResources(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) ([]mcp.Resource, error) {
	config, err := sm.transformServerConfig(ctx, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	client, err := sm.local.Client(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	resp, err := client.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP resources: %w", err)
	}

	return resp.Resources, nil
}

func (sm *SessionManager) ReadResource(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig, uri string) ([]mcp.ResourceContents, error) {
	config, err := sm.transformServerConfig(ctx, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	client, err := sm.local.Client(config)
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
