package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/mcp"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ClientForServer(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) (*mcp.Client, error) {
	clientName := "Obot MCP Gateway"
	if strings.HasPrefix(serverConfig.URL, fmt.Sprintf("%s/mcp-connect/", sm.baseURL)) {
		// If the URL points back to us, then this is Obot chat. Ensure the client name reflects that.
		clientName = "Obot MCP Chat"
	}
	return sm.ClientForServerWithOptions(ctx, mcpServer, serverConfig, nmcp.ClientOption{
		ClientName:   clientName,
		TokenStorage: sm.tokenStorage.ForMCPID(mcpServer.Name),
	})
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
