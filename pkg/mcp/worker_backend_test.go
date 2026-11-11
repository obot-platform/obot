package mcp

import (
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestFixedOpenAPIWorkerBackendCapabilityBoundaries(t *testing.T) {
	worker := httptest.NewServer(nil)
	defer worker.Close()
	t.Setenv("MCPOpenAPIWorkerURL", worker.URL)
	t.Setenv("MCPOpenAPIWorkerObotInternalURL", "http://obot:8080")
	t.Setenv("MCPOpenAPIWorkerAllowedHosts", " business.example.test:443 ,,out.example.test ")

	backend, err := newFixedOpenAPIWorkerBackend(t.Context(), true, nil, nil)
	require.NoError(t, err)
	defer backend.client.Close()

	openAPI := ServerConfig{Runtime: types.RuntimeOpenAPI}
	ready, err := backend.ensureServerDeployment(t.Context(), openAPI)
	require.NoError(t, err)
	require.Equal(t, openAPI, ready)

	remote := ServerConfig{Runtime: types.RuntimeRemote, URL: "https://remote.example.test/mcp"}
	ready, err = backend.ensureServerDeployment(t.Context(), remote)
	require.NoError(t, err)
	require.Equal(t, remote, ready)

	dynamic := ServerConfig{Runtime: types.RuntimeNPX}
	_, err = backend.ensureServerDeployment(t.Context(), dynamic)
	var unsupported *ErrNotSupportedByBackend
	require.ErrorAs(t, err, &unsupported)
	require.Equal(t, RuntimeBackendOpenAPIWorker, unsupported.Backend)

	remoteConfig, allowedHosts := backend.remoteConfig(RemoteMCPURLValidationConfig{})
	require.False(t, remoteConfig.AllowLocalhostMCP)
	require.False(t, remoteConfig.AllowPrivateIPMCP)
	require.False(t, remoteConfig.AllowLinkLocalMCP)
	require.Equal(t, []string{"obot:8080", "business.example.test:443", "out.example.test"}, allowedHosts)
	require.Equal(
		t,
		"http://obot:8080/mcp-connect-composite/vmcp1",
		backend.transformObotHostname("https://obot.example.test/mcp-connect-composite/vmcp1"),
	)

	require.Error(t, backend.deployServer(t.Context(), dynamic))
	require.Error(t, backend.restartServer(t.Context(), dynamic))
	_, err = backend.streamServerLogs(t.Context(), "server")
	require.ErrorAs(t, err, &unsupported)
	_, err = backend.getServerDetails(t.Context(), "server")
	require.ErrorAs(t, err, &unsupported)
	require.NoError(t, backend.shutdownServer(t.Context(), "server", true))
}

func TestNewSessionManagerOpenAPIWorkerDoesNotInitializeDynamicBackends(t *testing.T) {
	worker := httptest.NewServer(nil)
	defer worker.Close()
	t.Setenv("MCPOpenAPIWorkerURL", worker.URL)
	t.Setenv("DOCKER_HOST", "unix:///definitely-unavailable/docker.sock")
	t.Setenv("KUBECONFIG", "/definitely/unavailable/kubeconfig")

	manager, err := NewSessionManager(
		t.Context(), false, true, nil, nil, "https://obot.example.test", 8080,
		Options{MCPRuntimeBackend: RuntimeBackendOpenAPIWorker}, nil, nil, nil, nil, nil, nil, "", nil,
	)
	require.NoError(t, err)
	require.Equal(t, RuntimeBackendOpenAPIWorker, manager.MCPRuntimeBackend())
	require.NotNil(t, manager.OpenAPIWorkerExecutor())
	manager.CloseOpenAPIWorkerClient()

	_, err = NewSessionManager(
		t.Context(), false, true, nil, nil, "https://obot.example.test", 8080,
		Options{MCPRuntimeBackend: RuntimeBackendKubernetes}, nil, nil, nil, nil, nil, nil, "", nil,
	)
	require.Error(t, err)

	_, err = NewSessionManager(
		t.Context(), false, true, nil, nil, "https://obot.example.test", 8080,
		Options{MCPRuntimeBackend: "invalid"}, nil, nil, nil, nil, nil, nil, "", nil,
	)
	require.Error(t, err)
}
