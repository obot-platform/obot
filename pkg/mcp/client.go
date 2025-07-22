package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/mcp"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ClientForMCPServerWithOptions(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*mcp.Client, error) {
	mcpServerName := mcpServer.Spec.Manifest.Name
	if mcpServerName == "" {
		mcpServerName = mcpServer.Name
	}

	return sm.clientForServerWithOptions(ctx, mcpServerName, serverConfig, opts...)
}

func (sm *SessionManager) ClientForMCPServer(ctx context.Context, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) (*mcp.Client, error) {
	mcpServerName := mcpServer.Spec.Manifest.Name
	if mcpServerName == "" {
		mcpServerName = mcpServer.Name
	}

	return sm.ClientForServer(ctx, userID, mcpServerName, mcpServer.Name, serverConfig)
}

func (sm *SessionManager) ClientForServer(ctx context.Context, userID, mcpServerName, mcpServerID string, serverConfig ServerConfig) (*mcp.Client, error) {
	clientName := "Obot MCP Gateway"
	if strings.HasPrefix(serverConfig.URL, fmt.Sprintf("%s/mcp-connect/", sm.baseURL)) {
		// If the URL points back to us, then this is Obot chat. Ensure the client name reflects that.
		clientName = "Obot MCP Chat"
	}

	return sm.clientForServerWithOptions(ctx, mcpServerName, serverConfig, nmcp.ClientOption{
		ClientName:   clientName,
		TokenStorage: sm.tokenStorage.ForUserAndMCP(userID, mcpServerID),
	})
}

func (sm *SessionManager) clientForServerWithOptions(ctx context.Context, mcpServerName string, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*mcp.Client, error) {
	config, err := sm.transformServerConfig(ctx, mcpServerName, serverConfig)
	if err != nil {
		return nil, err
	}

	client, err := sm.local.Client(config, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return client, nil
}
