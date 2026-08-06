package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (sm *SessionManager) ListResources(ctx context.Context, serverConfig ServerConfig) ([]*gomcp.Resource, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP resources: %w", err)
	}

	return resp.Resources, nil
}

func (sm *SessionManager) ReadResource(ctx context.Context, serverConfig ServerConfig, uri string) ([]*gomcp.ResourceContents, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ReadResource(ctx, &gomcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP resource: %w", err)
	}

	return resp.Contents, nil
}
