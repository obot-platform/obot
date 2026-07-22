//go:build integration

package integration

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/tests/integration/harness"
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
//  8. Delete  — DELETE /api/mcp-servers/{id}, then verify the API object and
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	manifest := types.MCPServerManifest{
		Name:        h.MCPServerName("lifecycle"),
		Description: "integration test server",
		Runtime:     types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: integrationMCPImage,
			Port:  3001,
			Path:  "/mcp",
		},
	}

	created := h.CreateMCPServer(ctx, types.MCPServer{MCPServerManifest: manifest})
	if created.ID == "" {
		t.Fatalf("create returned empty ID: %+v", created)
	}
	t.Logf("created MCP server id=%s", created.ID)

	h.ConfigureMCPServer(ctx, created.ID, nil)
	configured := h.GetMCPServer(ctx, created.ID)
	if !configured.Configured {
		t.Fatalf("expected server to report Configured=true after configure, got %+v", configured)
	}

	h.LaunchMCPServer(ctx, created.ID)

	details := h.WaitForMCPServerAvailable(ctx, created.ID, 30*time.Second)
	if details.DeploymentName == "" || details.ReadyReplicas != 1 {
		t.Fatalf("expected one ready deployment, got %+v", details)
	}
	t.Logf("details: deployment=%s ready=%d/%d available=%v events=%d",
		details.DeploymentName, details.ReadyReplicas, details.Replicas, details.IsAvailable, len(details.Events))
	initialContainerID := requireSingleDockerDeployment(ctx, t, created.ID)

	var tools []types.MCPServerTool
	h.Get(ctx, "/api/mcp-servers/"+created.ID+"/tools", &tools)
	if len(tools) == 0 {
		t.Fatalf("expected at least one tool on a running MCP server, got none")
	}
	t.Logf("listed %d tools from server", len(tools))
	assertEchoToolCall(ctx, t, h.BaseURL, created.ID, "before restart")
	assertMCPServerStartupLog(ctx, t, h, created.ID)

	h.RestartMCPServer(ctx, created.ID)
	restartedContainerID := waitForDockerDeploymentReplaced(ctx, t, created.ID, initialContainerID, 30*time.Second)
	if restartedContainerID == initialContainerID {
		t.Fatalf("restart kept the original Docker container %s", initialContainerID)
	}
	h.WaitForMCPServerAvailable(ctx, created.ID, 30*time.Second)
	assertEchoToolCall(ctx, t, h.BaseURL, created.ID, "after restart")
	assertMCPServerStartupLog(ctx, t, h, created.ID)

	h.Delete(ctx, "/api/mcp-servers/"+created.ID)
	h.WaitForMCPServerDeleted(ctx, created.ID, 30*time.Second)
	waitForDockerDeploymentRemoved(ctx, t, created.ID, 30*time.Second)
}

func assertEchoToolCall(ctx context.Context, t *testing.T, baseURL, id, message string) {
	t.Helper()
	client, err := nmcp.NewClient(ctx, "integration-test", nmcp.Server{
		BaseURL: baseURL + "/mcp-connect/" + id,
	})
	if err != nil {
		t.Fatalf("create MCP client: %v", err)
	}
	defer client.Close(false)

	result, err := client.Call(ctx, "echo", map[string]any{"message": message})
	if err != nil {
		t.Fatalf("call echo tool: %v", err)
	}
	if result.IsError || len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text != message {
		t.Fatalf("unexpected echo tool result: %+v", result)
	}
}

func assertMCPServerStartupLog(ctx context.Context, t *testing.T, h *harness.Harness, id string) {
	t.Helper()
	const marker = "integration MCP server listening on ports 3001 and 8080"
	logs := h.ReadStreamUntil(ctx, "/api/mcp-servers/"+id+"/logs", []byte(marker), 5*time.Second, 4096)
	if !bytes.Contains(logs, []byte(marker)) {
		t.Fatalf("MCP server logs did not contain %q: %s", marker, logs)
	}
}

func requireSingleDockerDeployment(ctx context.Context, t *testing.T, id string) string {
	t.Helper()
	containers := dockerContainersForDeployment(ctx, t, id)
	if len(containers) != 1 {
		t.Fatalf("expected one Docker container for MCP deployment %s, got %v", id, containers)
	}
	return containers[0]
}

func waitForDockerDeploymentReplaced(ctx context.Context, t *testing.T, id, previousID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var containers []string
	for time.Now().Before(deadline) {
		containers = dockerContainersForDeployment(ctx, t, id)
		if len(containers) == 1 && containers[0] != previousID {
			return containers[0]
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("Docker deployment %s was not replaced within %s (previous=%s, current=%v)", id, timeout, previousID, containers)
	return ""
}

func waitForDockerDeploymentRemoved(ctx context.Context, t *testing.T, id string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var remaining []string
	for time.Now().Before(deadline) {
		remaining = dockerContainersForDeployment(ctx, t, id)
		if len(remaining) == 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("Docker containers for MCP deployment %s were not removed within %s: %v", id, timeout, remaining)
}

func dockerContainersForDeployment(ctx context.Context, t *testing.T, id string) []string {
	t.Helper()
	output, err := exec.CommandContext(ctx, "docker", "ps", "--all", "--quiet", "--filter", "label=mcp.deployment.id="+id).CombinedOutput()
	if err != nil {
		t.Fatalf("list Docker containers for MCP deployment %s: %v\n%s", id, err, output)
	}
	return strings.Fields(string(output))
}
