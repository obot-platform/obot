package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestAnalyzeContainerState(t *testing.T) {
	tests := []struct {
		name        string
		state       *container.State
		wantRetry   bool
		wantErr     bool
		wantMessage string
	}{
		{
			name:      "missing state retries",
			wantRetry: true,
		},
		{
			name:      "created container retries",
			state:     &container.State{Status: container.StateCreated},
			wantRetry: true,
		},
		{
			name:      "restarting container retries",
			state:     &container.State{Status: container.StateRestarting, Restarting: true},
			wantRetry: true,
		},
		{
			name:        "restarting failed container returns health failure",
			state:       &container.State{Status: container.StateRestarting, Restarting: true, ExitCode: 1},
			wantErr:     true,
			wantMessage: "container is restarting with exit code 1",
		},
		{
			name:  "running container is ready",
			state: &container.State{Status: container.StateRunning, Running: true},
		},
		{
			name:        "paused container fails",
			state:       &container.State{Status: container.StatePaused, Running: true, Paused: true},
			wantErr:     true,
			wantMessage: "container is paused",
		},
		{
			name:        "exited container fails even with zero exit code",
			state:       &container.State{Status: container.StateExited},
			wantErr:     true,
			wantMessage: "container is exited with exit code 0",
		},
		{
			name:        "failed container includes Docker error",
			state:       &container.State{Status: container.StateExited, ExitCode: 2, Error: "exec format error"},
			wantErr:     true,
			wantMessage: "container is exited with exit code 2: exec format error",
		},
		{
			name:        "OOM-killed container fails",
			state:       &container.State{Status: container.StateExited, ExitCode: 137, OOMKilled: true},
			wantErr:     true,
			wantMessage: "container was OOM-killed",
		},
		{
			name:        "dead container fails",
			state:       &container.State{Status: container.StateDead, Dead: true},
			wantErr:     true,
			wantMessage: "container is dead",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRetry, err := analyzeContainerState(tt.state)
			if shouldRetry != tt.wantRetry {
				t.Fatalf("analyzeContainerState() retry = %v, want %v", shouldRetry, tt.wantRetry)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("analyzeContainerState() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrHealthCheckFailed) {
				t.Fatalf("analyzeContainerState() error = %v, want %v", err, ErrHealthCheckFailed)
			}
			if tt.wantMessage != "" && !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("analyzeContainerState() error = %q, want it to contain %q", err, tt.wantMessage)
			}
		})
	}
}

func TestMonitorContainerReadinessDetectsCrashLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	err := monitorContainerReadiness(ctx, time.Millisecond,
		func(context.Context) (container.InspectResponse, error) {
			return container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{
				RestartCount: 4,
				State: &container.State{
					Status:   container.StateRunning,
					Running:  true,
					ExitCode: 1,
				},
			}}, nil
		},
		func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if !errors.Is(err, ErrHealthCheckFailed) {
		t.Fatalf("monitorContainerReadiness() error = %v, want %v", err, ErrHealthCheckFailed)
	}
	if !strings.Contains(err.Error(), "4 restarts") {
		t.Fatalf("monitorContainerReadiness() error = %q, want restart count", err)
	}
}

func TestMonitorContainerReadinessReturnsSuccessfulHealthCheck(t *testing.T) {
	err := monitorContainerReadiness(t.Context(), time.Hour,
		func(context.Context) (container.InspectResponse, error) {
			t.Fatal("container inspection should not run before the health check succeeds")
			return container.InspectResponse{}, nil
		},
		func(context.Context) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("monitorContainerReadiness() error = %v", err)
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
