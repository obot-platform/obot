//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	mmmcphttp "github.com/obot-platform/mmmcp/component/http"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/tests/integration/harness"
)

const (
	everythingPackage       = "@modelcontextprotocol/server-everything@2026.8.18"
	integrationEnvKey       = "INTEGRATION_VALUE"
	integrationEnvValue     = "configured"
	updatedIntegrationValue = "reconfigured"
)

// TestMCPServerLifecycle_NPXEverything exercises a single-user NPX MCP server
// through the public Obot API. The Docker backend runs the pinned MCP Everything
// package through mmmcp and proves create, configure, launch, invoke, restart,
// reconfigure, and delete behavior across the full stack.
//
// Docker and access to the npm and GHCR registries are required.
func TestMCPServerLifecycle_NPXEverything(t *testing.T) {
	h := harness.New(t)

	var (
		created              types.MCPServer
		initialContainerID   string
		restartedContainerID string
	)

	if !t.Run("create_and_configure", func(t *testing.T) {
		entry := h.CreateMCPCatalogEntry(t, system.DefaultCatalog, types.MCPServerCatalogEntryManifest{
			Name:             h.MCPServerName("lifecycle"),
			ShortDescription: "integration test server",
			Description:      "MCP Everything integration test server",
			Runtime:          types.RuntimeNPX,
			ServerUserType:   types.ServerUserTypeSingleUser,
			NPXConfig: &types.NPXRuntimeConfig{
				Package:               everythingPackage,
				Args:                  []string{"stdio"},
				StartupTimeoutSeconds: 120,
			},
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
				Name:     "Integration Value",
				Key:      integrationEnvKey,
				Required: true,
			}}},
		})
		if entry.ID == "" {
			t.Fatalf("catalog entry create returned empty ID: %+v", entry)
		}

		created = h.CreateMCPServerFromCatalogEntry(t, entry.ID)
		if created.ID == "" {
			t.Fatalf("server create returned empty ID: %+v", created)
		}
		t.Logf("created MCP server id=%s from catalog entry id=%s", created.ID, entry.ID)

		h.ConfigureMCPServer(t, created.ID, map[string]string{integrationEnvKey: integrationEnvValue})
		configured := h.GetMCPServer(t, created.ID)
		if !configured.Configured {
			t.Fatalf("expected server to report Configured=true after configure, got %+v", configured)
		}
	}) {
		return
	}

	if !t.Run("launch", func(t *testing.T) {
		h.LaunchMCPServer(t, created.ID)
		details := h.WaitForMCPServerAvailable(t, created.ID, 2*time.Minute)
		if details.DeploymentName == "" || details.ReadyReplicas != 1 {
			t.Fatalf("expected one ready deployment, got %+v", details)
		}
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
		assertEchoToolCall(t, h.BaseURL, created.ID, "before restart")
		assertEnvironmentValue(t, h.BaseURL, created.ID, integrationEnvValue)
	}) {
		return
	}

	if !t.Run("restart", func(t *testing.T) {
		h.RestartMCPServer(t, created.ID)
		restartedContainerID = waitForDockerDeploymentReplaced(t, created.ID, initialContainerID, 2*time.Minute)
		h.WaitForMCPServerAvailable(t, created.ID, 2*time.Minute)
		assertEchoToolCall(t, h.BaseURL, created.ID, "after restart")
		assertEnvironmentValue(t, h.BaseURL, created.ID, integrationEnvValue)
	}) {
		return
	}

	if !t.Run("reconfigure", func(t *testing.T) {
		h.ConfigureMCPServer(t, created.ID, map[string]string{integrationEnvKey: updatedIntegrationValue})
		waitForDockerDeploymentRemoved(t, created.ID, 2*time.Minute)
		configured := h.GetMCPServer(t, created.ID)
		if !configured.Configured {
			t.Fatalf("expected server to remain configured after updating %s, got %+v", integrationEnvKey, configured)
		}
		h.LaunchMCPServer(t, created.ID)
		waitForDockerDeploymentReplaced(t, created.ID, restartedContainerID, 2*time.Minute)
		h.WaitForMCPServerAvailable(t, created.ID, 2*time.Minute)
		assertEchoToolCall(t, h.BaseURL, created.ID, "after reconfigure")
		assertEnvironmentValue(t, h.BaseURL, created.ID, updatedIntegrationValue)
	}) {
		return
	}

	t.Run("delete", func(t *testing.T) {
		h.Delete(t, "/api/mcp-servers/"+created.ID)
		h.WaitForMCPServerDeleted(t, created.ID, 30*time.Second)
		waitForDockerDeploymentRemoved(t, created.ID, 30*time.Second)
	})
}

func assertEchoToolCall(t *testing.T, baseURL, id, message string) {
	t.Helper()
	result := callTool(t, baseURL, id, &gomcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"message": message},
	})
	assertTextToolResult(t, result, "Echo: "+message)
}

func assertEnvironmentValue(t *testing.T, baseURL, id, expected string) {
	t.Helper()
	result := callTool(t, baseURL, id, &gomcp.CallToolParams{Name: "get-env"})
	text := assertTextToolResult(t, result, "")
	var env map[string]string
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("decode get-env result: %v\nbody: %s", err, text)
	}
	if actual := env[integrationEnvKey]; actual != expected {
		t.Fatalf("%s = %q, want %q", integrationEnvKey, actual, expected)
	}
}

func callTool(t *testing.T, baseURL, id string, params *gomcp.CallToolParams) *gomcp.CallToolResult {
	t.Helper()
	client := mmmcphttp.NewFactory(mmmcphttp.FactoryOptions{})
	result, err := client.CallTool(t.Context(), mmmcpconfig.Server{
		Name: "integration-test",
		URL:  baseURL + "/mcp-connect/" + id,
	}, params)
	if err != nil {
		t.Fatalf("call %s tool: %v", params.Name, err)
	}
	return result
}

func assertTextToolResult(t *testing.T, result *gomcp.CallToolResult, expected string) string {
	t.Helper()
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("unexpected tool result: %+v", result)
	}
	text, ok := result.Content[0].(*gomcp.TextContent)
	if !ok || expected != "" && text.Text != expected {
		t.Fatalf("unexpected tool result: %+v", result)
	}
	return text.Text
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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "ps", "--all", "--quiet", "--filter", "label=mcp.deployment.id="+id).CombinedOutput()
	if err != nil {
		t.Fatalf("list Docker containers for MCP deployment %s: %v\n%s", id, err, output)
	}
	return strings.Fields(string(output))
}
