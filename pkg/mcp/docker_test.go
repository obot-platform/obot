package mcp

import (
	"errors"
	"testing"

	"github.com/moby/moby/api/types/container"
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

func TestDockerBackendNetworkConfigUsesDetectedContainerNetwork(t *testing.T) {
	localCalled := false

	containerEnv, network, host, err := dockerBackendNetworkConfig(
		func() (string, string, error) {
			return "obot_default", "172.18.0.4", nil
		},
		func() (string, error) {
			localCalled = true
			return "192.168.1.4", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containerEnv {
		t.Fatalf("expected containerEnv")
	}
	if network != "obot_default" {
		t.Fatalf("expected detected network, got %q", network)
	}
	if host != "172.18.0.4" {
		t.Fatalf("expected detected host, got %q", host)
	}
	if localCalled {
		t.Fatalf("did not expect local IP detection to be called")
	}
}

func TestDockerBackendNetworkConfigFallsBackToLocalIP(t *testing.T) {
	tests := map[string]func() (string, string, error){
		"container detection errors": func() (string, string, error) {
			return "", "", errors.New("inspect failed")
		},
		"container detection has no IP": func() (string, string, error) {
			return "obot_default", "", nil
		},
	}

	for name, detectContainer := range tests {
		t.Run(name, func(t *testing.T) {
			containerEnv, network, host, err := dockerBackendNetworkConfig(
				detectContainer,
				func() (string, error) {
					return "192.168.1.4", nil
				},
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if containerEnv {
				t.Fatalf("did not expect containerEnv")
			}
			if network != "bridge" {
				t.Fatalf("expected default network, got %q", network)
			}
			if host != "192.168.1.4" {
				t.Fatalf("expected local host, got %q", host)
			}
		})
	}
}

func TestDockerBackendNetworkConfigReturnsLocalIPError(t *testing.T) {
	routeErr := errors.New("route failed")

	_, _, _, err := dockerBackendNetworkConfig(
		func() (string, string, error) {
			return "", "", errors.New("inspect failed")
		},
		func() (string, error) {
			return "", routeErr
		},
	)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, routeErr) {
		t.Fatalf("expected wrapped route error, got %v", err)
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
