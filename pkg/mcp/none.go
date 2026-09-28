package mcp

import (
	"context"
	"io"

	otypes "github.com/obot-platform/obot/apiclient/types"
)

// noneBackend is the runtime backend for deployments that only proxy remote MCP
// servers (and composites of them). Obot itself is the proxy for those, so no
// container runtime is needed: this backend never talks to Docker or Kubernetes,
// and refuses anything that would have to be hosted.
type noneBackend struct{}

func newNoneBackend() *noneBackend {
	return &noneBackend{}
}

func (n *noneBackend) notSupported(feature string) error {
	return &ErrNotSupportedByBackend{Feature: feature, Backend: RuntimeBackendNone}
}

func (n *noneBackend) ensureServerDeployment(_ context.Context, server ServerConfig) (ServerConfig, error) {
	if server.Runtime == otypes.RuntimeRemote || server.Runtime == otypes.RuntimeVMCP {
		return server, nil
	}
	return ServerConfig{}, n.notSupported("hosted MCP servers")
}

func (n *noneBackend) deployServer(_ context.Context, server ServerConfig) error {
	if server.Runtime == otypes.RuntimeRemote || server.Runtime == otypes.RuntimeVMCP {
		return nil
	}
	return n.notSupported("hosted MCP servers")
}

func (n *noneBackend) streamServerLogs(context.Context, string) (io.ReadCloser, error) {
	return nil, n.notSupported("server logs")
}

func (n *noneBackend) getServerDetails(context.Context, string) (otypes.MCPServerDetails, error) {
	return otypes.MCPServerDetails{}, n.notSupported("server details")
}

func (n *noneBackend) restartServer(context.Context, ServerConfig) error {
	return n.notSupported("server restarts")
}

// shutdownServer succeeds: nothing is ever deployed, so there is nothing to stop.
// Disabled system MCP servers are shut down at startup and must not error.
func (n *noneBackend) shutdownServer(context.Context, string, bool) error {
	return nil
}

func (n *noneBackend) transformObotHostname(url string) string {
	return url
}

// remoteConfig keeps the global validation settings: unlike the Docker backend,
// there are no local containers that remote URL validation must let through.
func (n *noneBackend) remoteConfig(globalConfig RemoteMCPURLValidationConfig) (RemoteMCPURLValidationConfig, []string) {
	return globalConfig, nil
}
