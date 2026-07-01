package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	otypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestDockerTransformObotHostnameAlwaysRewritesHost(t *testing.T) {
	d := &dockerBackend{hostBaseURLWithPort: "http://172.17.0.1:8080"}

	tests := map[string]string{
		"http://localhost:8080/oauth/token":                 "http://172.17.0.1:8080/oauth/token",
		"http://obot.example.com/oauth/token":               "http://172.17.0.1:8080/oauth/token",
		"https://obot.example.com/oauth/token?audience=mcp": "http://172.17.0.1:8080/oauth/token?audience=mcp",
		"http://obot.example.com":                           "http://172.17.0.1:8080",
		"":                                                  "",
		"not-a-url":                                         "not-a-url",
	}

	for input, expected := range tests {
		if result := d.transformObotHostname(input); result != expected {
			t.Fatalf("transformObotHostname(%q) = %q, want %q", input, result, expected)
		}
	}
}

func TestContainerFilesStablePathsAcrossDataChanges(t *testing.T) {
	filesA := []File{{
		EnvKey: "TLS_CERT",
		Data:   "value-a",
	}, {
		EnvKey: "TLS_KEY",
		Data:   "value-b",
	}}

	filesB := []File{{
		EnvKey: "TLS_CERT",
		Data:   "new-value-a",
	}, {
		EnvKey: "TLS_KEY",
		Data:   "new-value-b",
	}}

	_, envA := containerFiles(filesA, "server")
	_, envB := containerFiles(filesB, "server")

	if envA["TLS_CERT"] != envB["TLS_CERT"] {
		t.Fatalf("expected stable path for TLS_CERT, got %q and %q", envA["TLS_CERT"], envB["TLS_CERT"])
	}

	if envA["TLS_KEY"] != envB["TLS_KEY"] {
		t.Fatalf("expected stable path for TLS_KEY, got %q and %q", envA["TLS_KEY"], envB["TLS_KEY"])
	}
}

func TestFileEnvKeysHashIgnoresData(t *testing.T) {
	filesA := []File{{
		EnvKey: "TLS_CERT",
		Data:   "a",
	}, {
		EnvKey: "TLS_KEY",
		Data:   "b",
	}}

	filesB := []File{{
		EnvKey: "TLS_CERT",
		Data:   "new-a",
	}, {
		EnvKey: "TLS_KEY",
		Data:   "new-b",
	}}

	if fileEnvKeysHash(filesA) != fileEnvKeysHash(filesB) {
		t.Fatalf("expected file env key hash to ignore file data")
	}
}

func TestFileEnvKeysHashChangesWithKeySet(t *testing.T) {
	filesA := []File{{
		EnvKey: "TLS_CERT",
		Data:   "a",
	}}

	filesB := []File{{
		EnvKey: "TLS_CERT",
		Data:   "a",
	}, {
		EnvKey: "TLS_KEY",
		Data:   "b",
	}}

	if fileEnvKeysHash(filesA) == fileEnvKeysHash(filesB) {
		t.Fatalf("expected different file env key hash when key set changes")
	}
}

func TestApplyServerConfigToContainerConfigOverridesImageAndLabels(t *testing.T) {
	config := &container.Config{
		Image:  "ghcr.io/obot-platform/nanobot:v0.0.59",
		Labels: nil,
	}

	server := ServerConfig{
		MCPServerName:  "mcp-server-abc",
		ContainerImage: "ghcr.io/obot-platform/nanobot:v0.0.65",
		Runtime:        "containerized",
		Files: []File{{
			EnvKey:  "NANOBOT_ENV_FILE",
			Data:    "value",
			Dynamic: true,
		}},
	}

	applyServerConfigToContainerConfig(config, server)

	if config.Image != server.ContainerImage {
		t.Fatalf("expected image %q, got %q", server.ContainerImage, config.Image)
	}

	if got, ok := config.Labels["mcp.config.hash"]; !ok || got != serverID(server) {
		t.Fatalf("expected mcp.config.hash %q, got %q", serverID(server), got)
	}

	if got, ok := config.Labels["mcp.file.env.keys.hash"]; !ok || got != fileEnvKeysHash(server.Files) {
		t.Fatalf("expected mcp.file.env.keys.hash %q, got %q", fileEnvKeysHash(server.Files), got)
	}
}

func TestApplyServerConfigToContainerConfigNoImageNoChanges(t *testing.T) {
	config := &container.Config{
		Image: "ghcr.io/obot-platform/nanobot:v0.0.65",
		Labels: map[string]string{
			"existing": "label",
		},
	}

	originalImage := config.Image
	originalExistingLabel := config.Labels["existing"]

	server := ServerConfig{
		MCPServerName: "mcp-server-abc",
	}

	applyServerConfigToContainerConfig(config, server)

	if config.Image != originalImage {
		t.Fatalf("expected image to remain %q, got %q", originalImage, config.Image)
	}

	if config.Labels["existing"] != originalExistingLabel {
		t.Fatalf("expected existing label to remain %q, got %q", originalExistingLabel, config.Labels["existing"])
	}

	if _, ok := config.Labels["mcp.config.hash"]; ok {
		t.Fatalf("did not expect mcp.config.hash label to be set")
	}

	if _, ok := config.Labels["mcp.file.env.keys.hash"]; ok {
		t.Fatalf("did not expect mcp.file.env.keys.hash label to be set")
	}
}

func TestCreateAndStartContainerUsesInspectFallbackForCreatedNameConflict(t *testing.T) {
	const (
		containerName = "sms1obot-mcp-server"
		containerID   = "17f163b3e3d6685f518c2b4cdbbd2545cc9228b57bb120555675bcf6fdf81d3c"
		imageName     = "ghcr.io/obot-platform/obot-mcp-server:v0.1.1"
	)

	var createCalls atomic.Int32
	var listCalls atomic.Int32
	var inspectCalls atomic.Int32
	var startCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.52")
		switch {
		case r.Method == http.MethodPost && path == "/images/create":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "{}")
		case r.Method == http.MethodPost && path == "/containers/create":
			createCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": "container name \"" + containerName + "\" is already in use by " + containerID,
			})
		case r.Method == http.MethodGet && path == "/containers/json":
			listCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "[]")
		case r.Method == http.MethodGet && path == "/containers/"+containerName+"/json":
			inspectCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    containerID,
					Name:  "/" + containerName,
					Image: imageName,
					State: &container.State{Status: container.StateCreated},
				},
				Config: &container.Config{
					Image: imageName,
					Labels: map[string]string{
						"mcp.config.hash":        "config-hash",
						"mcp.file.env.keys.hash": "",
					},
				},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{"bridge": {}},
				},
			})
		case r.Method == http.MethodPost && path == "/containers/"+containerID+"/start":
			startCalls.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected docker API request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	cli, err := client.NewClientWithOpts(client.WithHost("tcp://"+strings.TrimPrefix(server.URL, "http://")), client.WithHTTPClient(server.Client()))
	require.NoError(t, err)

	d := &dockerBackend{
		client:  cli,
		network: "bridge",
	}

	id, port, err := d.createAndStartContainer(t.Context(), ServerConfig{
		MCPServerName:        containerName,
		MCPServerDisplayName: "SMS MCP Server",
		Runtime:              otypes.RuntimeContainerized,
		ContainerImage:       imageName,
		ContainerPort:        8080,
	}, containerName, "config-hash", "", nil)
	require.NoError(t, err)
	require.Equal(t, containerID, id)
	require.Equal(t, 8080, port)
	require.Equal(t, int32(1), createCalls.Load())
	require.Equal(t, int32(1), listCalls.Load())
	require.Equal(t, int32(1), inspectCalls.Load())
	require.Equal(t, int32(1), startCalls.Load())
}

func TestInspectResponseToSummaryPreservesDeploymentFields(t *testing.T) {
	const containerName = "system-mcp-server"

	summary := inspectResponseToSummary(containerName, container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    "container-id",
			Name:  "/" + containerName,
			Image: "old-image",
			State: &container.State{Status: container.StateCreated},
		},
		Config: &container.Config{
			Image: "desired-image",
			Labels: map[string]string{
				"mcp.config.hash":        "config-hash",
				"mcp.file.env.keys.hash": "file-env-hash",
			},
		},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{"bridge": {IPAddress: "172.17.0.2"}},
		},
	})

	require.Equal(t, "container-id", summary.ID)
	require.Equal(t, []string{"/" + containerName}, summary.Names)
	require.Equal(t, "desired-image", summary.Image)
	require.Equal(t, container.StateCreated, summary.State)
	require.Equal(t, "config-hash", summary.Labels["mcp.config.hash"])
	require.Equal(t, "file-env-hash", summary.Labels["mcp.file.env.keys.hash"])
	require.Equal(t, "172.17.0.2", summary.NetworkSettings.Networks["bridge"].IPAddress)
}
