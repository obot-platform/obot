package mcp

import (
	"context"
	"io"

	otypes "github.com/obot-platform/obot/apiclient/types"
)

// cloudFoundryBackend runs Obot as a Cloud Foundry application without a
// container-orchestration runtime for MCP servers.
//
// Cloud Foundry has no equivalent of the docker or Kubernetes runtimes that the
// other backends drive, so this backend supports exactly the runtimes that need
// no orchestration: RuntimeRemote and RuntimeComposite, which are served by
// Obot's own gateway process. Every runtime that requires Obot to start a
// workload (uvx, npx, containerized) reports ErrNotSupportedByBackend, which the
// API handlers translate into a 404 rather than a 500.
//
// Deploying MCP servers as Cloud Foundry applications through the Cloud
// Controller v3 API is deliberately out of scope here; streamServerLogs,
// restartServer, and getServerDetails are the seams that work would grow into.
type cloudFoundryBackend struct {
	authEnabled    bool
	httpListenPort int
}

func newCloudFoundryBackend(authEnabled bool, httpListenPort int, _ Options) backend {
	return &cloudFoundryBackend{
		authEnabled:    authEnabled,
		httpListenPort: httpListenPort,
	}
}

// runtimeIsGatewayServed reports whether a runtime is served by Obot's own
// process and therefore needs no deployment of any kind.
func runtimeIsGatewayServed(runtime otypes.Runtime) bool {
	return runtime == otypes.RuntimeRemote || runtime == otypes.RuntimeComposite
}

func (c *cloudFoundryBackend) ensureServerDeployment(_ context.Context, server ServerConfig) (ServerConfig, error) {
	for i, component := range server.Components {
		component.URL = c.transformObotHostname(component.URL)
		server.Components[i] = component
	}

	for i, webhook := range server.Webhooks {
		webhook.URL = c.transformObotHostname(webhook.URL)
		server.Webhooks[i] = webhook
	}

	if runtimeIsGatewayServed(server.Runtime) {
		return server, nil
	}

	return ServerConfig{}, &ErrNotSupportedByBackend{
		Feature: "deploying " + string(server.Runtime) + " MCP servers",
		Backend: RuntimeBackendCloudFoundry,
	}
}

func (c *cloudFoundryBackend) deployServer(_ context.Context, server ServerConfig) error {
	if runtimeIsGatewayServed(server.Runtime) {
		return nil
	}

	return &ErrNotSupportedByBackend{
		Feature: "deploying " + string(server.Runtime) + " MCP servers",
		Backend: RuntimeBackendCloudFoundry,
	}
}

func (c *cloudFoundryBackend) getServerDetails(_ context.Context, _ string) (otypes.MCPServerDetails, error) {
	return otypes.MCPServerDetails{}, &ErrNotSupportedByBackend{
		Feature: "server details",
		Backend: RuntimeBackendCloudFoundry,
	}
}

func (c *cloudFoundryBackend) streamServerLogs(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, &ErrNotSupportedByBackend{
		Feature: "server logs",
		Backend: RuntimeBackendCloudFoundry,
	}
}

func (c *cloudFoundryBackend) restartServer(_ context.Context, _ ServerConfig) error {
	return &ErrNotSupportedByBackend{
		Feature: "restarting servers",
		Backend: RuntimeBackendCloudFoundry,
	}
}

// shutdownServer must succeed for every runtime, including the ones this backend
// cannot deploy. It is called from delete and finalizer paths as well as the
// idle-disable path, and returning an error there wedges the finalizer instead
// of degrading. Nothing is ever deployed, so there is nothing to shut down.
func (c *cloudFoundryBackend) shutdownServer(_ context.Context, _ string, _ bool) error {
	return nil
}

// transformObotHostname is the identity function. Obot is reachable at its
// external route, and no MCP workload runs anywhere that needs a rewritten
// hostname. Routing over the Cloud Foundry internal domain would change this.
func (c *cloudFoundryBackend) transformObotHostname(url string) string {
	return url
}

// remoteConfig returns the global validation config untouched, which keeps
// localhost, private-IP, and link-local blocking exactly as the operator
// configured it. The docker backend has to relax private-IP blocking because it
// talks to MCP containers over a bridge network; this backend talks to nothing
// internal, so it grants no exceptions and returns an empty allowlist.
func (c *cloudFoundryBackend) remoteConfig(globalConfig RemoteMCPURLValidationConfig) (RemoteMCPURLValidationConfig, []string) {
	return globalConfig, nil
}
