package mcp

import (
	"context"
	"fmt"

	"github.com/gptscript-ai/gptscript/pkg/mcp"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ClientForServer(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) (*mcp.Client, error) {
	return sm.ClientForServerWithOptions(ctx, mcpServer, serverConfig, nmcp.ClientOption{TokenStorage: sm.tokenStorage.ForMCPID(mcpServer.Name)})
}

func (sm *SessionManager) ClientForServerWithOptions(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*mcp.Client, error) {
	config, err := sm.transformServerConfig(ctx, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	client, err := sm.local.Client(config, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return client, nil
}
