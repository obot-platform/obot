package mcp

import (
	"fmt"

	gmcp "github.com/mark3labs/mcp-go/mcp"
)

func (sm *SessionManager) ServerCapabilities(server ServerConfig) (gmcp.ServerCapabilities, error) {
	client, err := sm.local.Client(server.ServerConfig)
	if err != nil {
		return gmcp.ServerCapabilities{}, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return client.Capabilities(), nil
}
