package mcp

import (
	"context"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (sm *SessionManager) ServerCapabilities(ctx context.Context, serverConfig ServerConfig) (*gomcp.ServerCapabilities, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	return client.InitializeResult().Capabilities, nil
}
