package mcp

import (
	"testing"

	"github.com/moby/moby/api/types/container"
)

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
