package mcp

import (
	"context"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
)

type localBackend struct{}

func newLocalBackend() backend {
	return &localBackend{}
}

func (*localBackend) ensureServerDeployment(_ context.Context, server ServerConfig, _, _, _ string) (ServerConfig, error) {
	if server.Runtime == types.RuntimeContainerized {
		// The containerized runtime is not supported when running servers locally.
		return ServerConfig{}, &ErrNotSupportedByBackend{Feature: "containerized runtime", Backend: "local"}
	}

	return server, nil
}

func (*localBackend) transformConfig(_ context.Context, _ string, server ServerConfig) (*ServerConfig, error) {
	// No transformation needed for local backend.
	return &server, nil
}

func (*localBackend) streamServerLogs(context.Context, string) (io.ReadCloser, error) {
	return nil, &ErrNotSupportedByBackend{Feature: "streaming logs", Backend: "local"}
}

func (*localBackend) getServerDetails(context.Context, string) (types.MCPServerDetails, error) {
	return types.MCPServerDetails{}, &ErrNotSupportedByBackend{Feature: "server details", Backend: "local"}
}

func (*localBackend) restartServer(context.Context, string) error {
	return &ErrNotSupportedByBackend{Feature: "restarting server", Backend: "local"}
}

func (*localBackend) shutdownServer(context.Context, string) error {
	// Nothing to do for the local backend.
	return nil
}
