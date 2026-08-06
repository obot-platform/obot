package mcp

import (
	"context"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (sm *SessionManager) PingServer(ctx context.Context, serverConfig ServerConfig) error {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return err
	}

	return client.Ping(ctx, &gomcp.PingParams{})
}
