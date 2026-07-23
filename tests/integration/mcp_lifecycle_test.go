//go:build integration

package integration

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/tests/integration/harness"
)

const (
	echoPrefix        = "configured: "
	updatedEchoPrefix = "reconfigured: "
)

// TestMCPServerLifecycle_Containerized exercises the full lifecycle of a
// single-user containerized MCP server against a running obot instance:
//
//  1. Create  — POST /api/mcp-servers
//  2. Configure — POST /api/mcp-servers/{id}/configure (flips Configured=true)
//  3. Launch   — POST /api/mcp-servers/{id}/launch
//  4. Wait for the deployment to become available
//  5. List tools — GET /api/mcp-servers/{id}/tools (proves the gateway can
//     reach the container and speak MCP)
//  6. Invoke the echo tool through the public MCP gateway
//  7. Restart and verify the Docker container is replaced and remains usable
//  8. Reconfigure and verify the new value reaches a replacement deployment
//  9. Delete  — DELETE /api/mcp-servers/{id}, then verify the API object and
//     Docker deployment are gone
//
// This is the project's first integration test. It is intentionally a single
// end-to-end happy-path flow rather than a battery of edge cases — its job is
// to prove the harness works and to serve as a template for future tests.
//
// Prerequisites:
//   - Docker is reachable. The test suite starts obot with the Docker MCP
//     runtime backend and builds the MCP fixture image locally.
func TestMCPServerLifecycle_Containerized(t *testing.T) {
	h := harness.New(t)

	var (
		created              types.MCPServer
		initialContainerID   string
		restartedContainerID string
	)

	if !t.Run("create_and_configure", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Name:        h.MCPServerName("lifecycle"),
			Description: "integration test server",
			Runtime:     types.RuntimeContainerized,
			ContainerizedConfig: &types.ContainerizedRuntimeConfig{
				Image: integrationMCPImage,
				Port:  3001,
				Path:  "/mcp",
			},
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
				Name:     "Echo Prefix",
				Key:      "ECHO_PREFIX",
				Required: true,
			}}},
		}

		created = h.CreateMCPServer(t, types.MCPServer{MCPServerManifest: manifest})
		if created.ID == "" {
			t.Fatalf("create returned empty ID: %+v", created)
		}
		t.Logf("created MCP server id=%s", created.ID)

		h.ConfigureMCPServer(t, created.ID, map[string]string{"ECHO_PREFIX": echoPrefix})
		configured := h.GetMCPServer(t, created.ID)
		if !configured.Configured {
			t.Fatalf("expected server to report Configured=true after configure, got %+v", configured)
		}
	}) {
		return
	}

	if !t.Run("launch", func(t *testing.T) {
		h.LaunchMCPServer(t, created.ID)
		details := h.WaitForMCPServerAvailable(t, created.ID, 30*time.Second)
		if details.DeploymentName == "" || details.ReadyReplicas != 1 {
			t.Fatalf("expected one ready deployment, got %+v", details)
		}
		t.Logf("details: deployment=%s ready=%d/%d available=%v events=%d",
			details.DeploymentName, details.ReadyReplicas, details.Replicas, details.IsAvailable, len(details.Events))
		initialContainerID = requireSingleDockerDeployment(t, created.ID)
	}) {
		return
	}

	if !t.Run("invoke_tool", func(t *testing.T) {
		var tools []types.MCPServerTool
		h.Get(t, "/api/mcp-servers/"+created.ID+"/tools", &tools)
		if len(tools) == 0 {
			t.Fatalf("expected at least one tool on a running MCP server, got none")
		}
		t.Logf("listed %d tools from server", len(tools))
		assertEchoToolCall(t, h.BaseURL, created.ID, echoPrefix, "before restart")
		assertMCPServerStartupLog(t, h, created.ID)
	}) {
		return
	}

	if !t.Run("restart", func(t *testing.T) {
		h.RestartMCPServer(t, created.ID)
		restartedContainerID = waitForDockerDeploymentReplaced(t, created.ID, initialContainerID, 30*time.Second)
		if restartedContainerID == initialContainerID {
			t.Fatalf("restart kept the original Docker container %s", initialContainerID)
		}
		h.WaitForMCPServerAvailable(t, created.ID, 30*time.Second)
		assertEchoToolCall(t, h.BaseURL, created.ID, echoPrefix, "after restart")
		assertMCPServerStartupLog(t, h, created.ID)
	}) {
		return
	}

	if !t.Run("reconfigure", func(t *testing.T) {
		h.ConfigureMCPServer(t, created.ID, map[string]string{"ECHO_PREFIX": updatedEchoPrefix})
		waitForDockerDeploymentRemoved(t, created.ID, 30*time.Second)
		configured := h.GetMCPServer(t, created.ID)
		if !configured.Configured {
			t.Fatalf("expected server to remain configured after updating ECHO_PREFIX, got %+v", configured)
		}
		h.LaunchMCPServer(t, created.ID)
		waitForDockerDeploymentReplaced(t, created.ID, restartedContainerID, 30*time.Second)
		h.WaitForMCPServerAvailable(t, created.ID, 30*time.Second)
		assertEchoToolCall(t, h.BaseURL, created.ID, updatedEchoPrefix, "after reconfigure")
	}) {
		return
	}

	t.Run("delete", func(t *testing.T) {
		h.Delete(t, "/api/mcp-servers/"+created.ID)
		h.WaitForMCPServerDeleted(t, created.ID, 30*time.Second)
		waitForDockerDeploymentRemoved(t, created.ID, 30*time.Second)
	})
}

func assertEchoToolCall(t *testing.T, baseURL, id, prefix, message string) {
	t.Helper()
	client, err := nmcp.NewClient(t.Context(), "integration-test", nmcp.Server{
		BaseURL: baseURL + "/mcp-connect/" + id,
	})
	if err != nil {
		t.Fatalf("create MCP client: %v", err)
	}
	defer client.Close(false)

	result, err := client.Call(t.Context(), "echo", map[string]any{"message": message})
	if err != nil {
		t.Fatalf("call echo tool: %v", err)
	}
	expected := prefix + message
	if result.IsError || len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text != expected {
		t.Fatalf("unexpected echo tool result: %+v", result)
	}
}

func assertMCPServerStartupLog(t *testing.T, h *harness.Harness, id string) {
	t.Helper()
	const marker = "integration MCP server listening on ports 3001 and 8080"
	logs := h.ReadStreamUntil(t, "/api/mcp-servers/"+id+"/logs", []byte(marker), 5*time.Second, 4096)
	if !bytes.Contains(logs, []byte(marker)) {
		t.Fatalf("MCP server logs did not contain %q: %s", marker, logs)
	}
}

func requireSingleDockerDeployment(t *testing.T, id string) string {
	t.Helper()
	containers := dockerContainersForDeployment(t, id)
	if len(containers) != 1 {
		t.Fatalf("expected one Docker container for MCP deployment %s, got %v", id, containers)
	}
	return containers[0]
}

func waitForDockerDeploymentReplaced(t *testing.T, id, previousID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var containers []string
	for time.Now().Before(deadline) {
		containers = dockerContainersForDeployment(t, id)
		if len(containers) == 1 && containers[0] != previousID {
			return containers[0]
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("Docker deployment %s was not replaced within %s (previous=%s, current=%v)", id, timeout, previousID, containers)
	return ""
}

func waitForDockerDeploymentRemoved(t *testing.T, id string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var remaining []string
	for time.Now().Before(deadline) {
		remaining = dockerContainersForDeployment(t, id)
		if len(remaining) == 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("Docker containers for MCP deployment %s were not removed within %s: %v", id, timeout, remaining)
}

func dockerContainersForDeployment(t *testing.T, id string) []string {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), "docker", "ps", "--all", "--quiet", "--filter", "label=mcp.deployment.id="+id).CombinedOutput()
	if err != nil {
		t.Fatalf("list Docker containers for MCP deployment %s: %v\n%s", id, err, output)
	}
	return strings.Fields(string(output))
}
