package mcp

import (
	"errors"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestNoneBackendAllowsRemoteAndVMCP(t *testing.T) {
	for _, runtime := range []types.Runtime{types.RuntimeRemote, types.RuntimeVMCP} {
		t.Run(string(runtime), func(t *testing.T) {
			backend := newNoneBackend()
			server := ServerConfig{
				Runtime: runtime,
				URL:     "https://mcp.example.com/mcp",
			}
			if err := backend.deployServer(t.Context(), server); err != nil {
				t.Fatalf("deployServer: %v", err)
			}
			got, err := backend.ensureServerDeployment(t.Context(), server)
			if err != nil {
				t.Fatalf("ensureServerDeployment: %v", err)
			}
			if got.URL != server.URL {
				t.Fatalf("ensureServerDeployment changed the URL: got %q, want %q", got.URL, server.URL)
			}
		})
	}
}

func TestNoneBackendRefusesHostedServers(t *testing.T) {
	for _, runtime := range []types.Runtime{types.RuntimeUVX, types.RuntimeNPX, types.RuntimeContainerized} {
		t.Run(string(runtime), func(t *testing.T) {
			backend := newNoneBackend()
			server := ServerConfig{
				Runtime: runtime,
			}
			if err := backend.deployServer(t.Context(), server); !isNotSupportedByNone(err) {
				t.Fatalf("deployServer: got %v, want ErrNotSupportedByBackend", err)
			}
			if _, err := backend.ensureServerDeployment(t.Context(), server); !isNotSupportedByNone(err) {
				t.Fatalf("ensureServerDeployment: got %v, want ErrNotSupportedByBackend", err)
			}
		})
	}
}

func TestNoneBackendShutdownIsNoop(t *testing.T) {
	// Disabled system MCP servers are shut down at startup; with nothing ever
	// deployed this must succeed rather than retry forever.
	if err := newNoneBackend().shutdownServer(t.Context(), "any-server", true); err != nil {
		t.Fatalf("shutdownServer: %v", err)
	}
}

func TestNoneBackendUnsupportedOperations(t *testing.T) {
	backend := newNoneBackend()
	if _, err := backend.streamServerLogs(t.Context(), "id"); !isNotSupportedByNone(err) {
		t.Fatalf("streamServerLogs: got %v, want ErrNotSupportedByBackend", err)
	}
	if _, err := backend.getServerDetails(t.Context(), "id"); !isNotSupportedByNone(err) {
		t.Fatalf("getServerDetails: got %v, want ErrNotSupportedByBackend", err)
	}
	if err := backend.restartServer(t.Context(), ServerConfig{}); !isNotSupportedByNone(err) {
		t.Fatalf("restartServer: got %v, want ErrNotSupportedByBackend", err)
	}
}

func TestNoneBackendKeepsRemoteURLValidation(t *testing.T) {
	global := RemoteMCPURLValidationConfig{}
	got, extra := newNoneBackend().remoteConfig(global)
	if got != global {
		t.Fatalf("remoteConfig changed the validation settings: got %+v, want %+v", got, global)
	}
	if len(extra) != 0 {
		t.Fatalf("remoteConfig allowed extra hosts: %v", extra)
	}
}

func isNotSupportedByNone(err error) bool {
	var nse *ErrNotSupportedByBackend
	return errors.As(err, &nse) && nse.Backend == RuntimeBackendNone
}
