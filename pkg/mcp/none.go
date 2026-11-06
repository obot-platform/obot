package mcp

import (
	"context"
	"fmt"
	"io"
	neturl "net/url"
	"strings"

	otypes "github.com/obot-platform/obot/apiclient/types"
)

// noneBackend is the runtime backend for deployments that only proxy remote MCP
// servers (and composites of them). Obot itself is the proxy for those, so no
// container runtime is needed: this backend never talks to Docker or Kubernetes,
// and refuses anything that would have to be hosted.
type noneBackend struct {
	// localBaseURL is where this Obot process listens; used when Obot calls
	// itself (e.g. MCP hook/filter servers via /mcp-connect), so those calls do
	// not depend on the public hostname resolving from inside the deployment.
	localBaseURL string
}

func newNoneBackend(httpListenPort int) *noneBackend {
	return &noneBackend{localBaseURL: fmt.Sprintf("http://localhost:%d", httpListenPort)}
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

// transformObotHostname points URLs that target Obot itself at the local
// listener, like the Docker backend does with its host address: the public
// hostname (OBOT_SERVER_HOSTNAME) may not resolve, or may be firewalled, from
// where Obot runs (e.g. an internal-only network behind an egress proxy).
func (n *noneBackend) transformObotHostname(rawURL string) string {
	if n.localBaseURL == "" || rawURL == "" {
		return rawURL
	}
	parsed, err := neturl.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURL
	}
	base, err := neturl.Parse(n.localBaseURL)
	if err != nil {
		return rawURL
	}
	parsed.Scheme = base.Scheme
	parsed.Host = base.Host
	parsed.User = nil
	return parsed.String()
}

// remoteConfig keeps the global validation settings and lets through only this
// Obot's own listener, like the Kubernetes backend: transformObotHostname points
// Obot's calls to itself (filters, composite loopbacks, system MCP servers) at
// localhost:<port>, which the default DisallowLocalhostMCP would otherwise block.
// No local containers exist, so nothing else is allowed.
func (n *noneBackend) remoteConfig(globalConfig RemoteMCPURLValidationConfig) (RemoteMCPURLValidationConfig, []string) {
	if n.localBaseURL == "" {
		return globalConfig, nil
	}
	return globalConfig, []string{strings.TrimPrefix(n.localBaseURL, "http://")}
}
