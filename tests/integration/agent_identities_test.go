//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
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

// agentVMCP is a shared vMCP with one NPX component that requires a user-allowed value, and one profile that names
// an agent explicitly.
type agentVMCP struct {
	agent       types.AgentIdentity
	vmcp        types.VMCP
	componentID string
	profileID   string
	mcpKey      string
}

// The integration server runs with authentication off, so requests without a key act as its single owner. Agents
// work in that mode too (an agent key authenticates as its agent, never as the owner), which lets these tests drive
// the admin API as the owner and the agent routes with agent keys against one running server.

// TestAgentIdentityKeysAndLifecycle checks, against a running server, that an agent key reaches only the routes its
// scopes and the agent allowlist allow, and that rotation, revocation, disable, enable and delete take effect on the
// next request. It needs no Docker runtime.
func TestAgentIdentityKeysAndLifecycle(t *testing.T) {
	h := harness.New(t)
	agent := h.CreateAgent(t, h.MCPServerName("agent-keys"))

	t.Run("a key needs a scope", func(t *testing.T) {
		resp := h.RequestAs(t, http.MethodPost, "/api/agents/"+agent.ID+"/api-keys", "", types.AgentAPIKeyCreateRequest{Name: "no-scope"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("creating a key with no scope: got %d, want 400\nbody: %s", resp.StatusCode, resp.Body)
		}
	})

	first := h.CreateAgentAPIKey(t, agent.ID, types.AgentAPIKeyCreateRequest{Name: "first", CanAccessSkills: true})
	if !strings.HasPrefix(first.Key, system.AgentAPIKeyPrefix+"-"+agent.ID+"-") {
		t.Fatalf("agent key %q does not have the agent key prefix and the agent's ID", maskKey(first.Key))
	}

	t.Run("the key reaches only what its scopes and the allowlist allow", func(t *testing.T) {
		requireStatus(t, h, http.MethodGet, "/api/skills", first.Key, http.StatusOK)
		// No LLM scope.
		requireStatus(t, h, http.MethodGet, "/api/llm-proxy/v1/models", first.Key, http.StatusForbidden)
		// No MCP scope.
		requireStatus(t, h, http.MethodPost, "/mcp-connect/"+system.DefaultCatalog, first.Key, http.StatusForbidden)
		// Never on the agent allowlist, whatever the scopes.
		for _, path := range []string{"/api/me", "/api/agents", "/api/api-keys", "/api/vmcps", "/api/vmcp-instances"} {
			requireStatus(t, h, http.MethodGet, path, first.Key, http.StatusForbidden)
		}
	})

	t.Run("an invalid agent key is refused, not treated as the owner", func(t *testing.T) {
		bogus := system.AgentAPIKeyPrefix + "-" + agent.ID + "-999999-not-a-real-secret"
		resp := requireStatus(t, h, http.MethodGet, "/api/skills", bogus, http.StatusUnauthorized)
		if resp.Header.Get("WWW-Authenticate") != "" {
			t.Errorf("a refused agent key got a WWW-Authenticate challenge: %q", resp.Header.Get("WWW-Authenticate"))
		}
		requireStatus(t, h, http.MethodGet, "/api/agents", bogus, http.StatusUnauthorized)
	})

	second := h.CreateAgentAPIKey(t, agent.ID, types.AgentAPIKeyCreateRequest{Name: "second", CanAccessSkills: true})

	t.Run("rotation: revoking one key leaves the other working", func(t *testing.T) {
		h.Delete(t, "/api/agents/"+agent.ID+"/api-keys/"+strconv.FormatUint(uint64(first.ID), 10))
		requireStatus(t, h, http.MethodGet, "/api/skills", first.Key, http.StatusUnauthorized)
		requireStatus(t, h, http.MethodGet, "/api/skills", second.Key, http.StatusOK)
	})

	t.Run("disable and enable take effect on the next request", func(t *testing.T) {
		h.Post(t, "/api/agents/"+agent.ID+"/disable", types.AgentIdentityDisableRequest{Reason: "integration test"}, nil)
		// A disabled principal is authenticated and then denied by admission, as a disabled user is.
		requireStatus(t, h, http.MethodGet, "/api/skills", second.Key, http.StatusForbidden)
		h.Post(t, "/api/agents/"+agent.ID+"/enable", map[string]any{}, nil)
		requireStatus(t, h, http.MethodGet, "/api/skills", second.Key, http.StatusOK)
	})

	t.Run("delete revokes every key and keeps the agent for audit", func(t *testing.T) {
		h.Delete(t, "/api/agents/"+agent.ID)
		requireStatus(t, h, http.MethodGet, "/api/skills", second.Key, http.StatusUnauthorized)

		var list types.AgentIdentityList
		h.Get(t, "/api/agents?includeDeleted=true", &list)
		idx := slices.IndexFunc(list.Items, func(a types.AgentIdentity) bool { return a.ID == agent.ID })
		if idx < 0 || list.Items[idx].Deleted == nil {
			t.Fatalf("deleted agent %s is not listed as deleted: %+v", agent.ID, list.Items)
		}
		h.Get(t, "/api/agents", &list)
		if slices.ContainsFunc(list.Items, func(a types.AgentIdentity) bool { return a.ID == agent.ID }) {
			t.Fatalf("deleted agent %s is listed without includeDeleted", agent.ID)
		}
	})

	t.Run("the admin change history records each change without secrets", func(t *testing.T) {
		var agentEvents, keyEvents types.AdminAuditEventList
		h.Get(t, "/api/admin-audit-events?resourceKind=agent&resourceId="+agent.ID, &agentEvents)
		var actions []string
		for _, event := range agentEvents.Items {
			actions = append(actions, event.Action)
		}
		for _, want := range []string{"agent.create", "agent.disable", "agent.enable", "agent.delete"} {
			if !slices.Contains(actions, want) {
				t.Errorf("admin change history of agent %s has no %s event: %v", agent.ID, want, actions)
			}
		}

		h.Get(t, "/api/admin-audit-events?resourceKind=apiKey&action=agent.key.create,agent.key.revoke&limit=1000", &keyEvents)
		var forAgent int
		for _, event := range keyEvents.Items {
			if event.ParentID == agent.ID {
				forAgent++
			}
		}
		if forAgent < 3 {
			t.Errorf("want key create and revoke events for agent %s, got %d: %+v", agent.ID, forAgent, keyEvents.Items)
		}

		raw, err := json.Marshal([]any{agentEvents, keyEvents})
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{first.Key, second.Key} {
			secret := strings.SplitN(key, "-", 4)[3]
			if strings.Contains(string(raw), secret) {
				t.Fatalf("the admin change history contains an API key secret")
			}
		}
	})
}

func newAgentVMCP(t *testing.T, h *harness.Harness, name string) agentVMCP {
	t.Helper()
	agent := h.CreateAgent(t, h.MCPServerName(name))
	entry := h.CreateMCPCatalogEntry(t, system.DefaultCatalog, types.MCPServerCatalogEntryManifest{
		Name:             h.MCPServerName(name),
		ShortDescription: "agent identity integration test server",
		Description:      "MCP Everything for agent identity integration tests",
		Runtime:          types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package:               everythingPackage,
			Args:                  []string{"stdio"},
			StartupTimeoutSeconds: 120,
		},
		Config: []types.MCPConfig{{
			Name:     "Integration Value",
			Key:      integrationEnvKey,
			Required: true,
			Usage:    types.Env,
		}},
	})
	vmcp := h.CreateSharedVMCP(t, types.VMCPManifest{
		DisplayName: h.MCPServerName(name),
		Components: []types.VMCPComponent{{
			Name:                    "everything",
			MCPCatalogID:            system.DefaultCatalog,
			MCPServerCatalogEntryID: entry.ID,
			Configuration: []types.VMCPConfigurationPolicy{{
				Key:    integrationEnvKey,
				Policy: types.VMCPConfigurationPolicyUserAllowed,
			}},
		}},
		Profiles: []types.VMCPProfile{{
			Name:        "agent",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: agent.ID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		}},
	})
	if len(vmcp.Components) != 1 || len(vmcp.Profiles) != 1 || vmcp.Profiles[0].ID == "" {
		t.Fatalf("vMCP create returned unexpected result: %+v", vmcp)
	}
	key := h.CreateAgentAPIKey(t, agent.ID, types.AgentAPIKeyCreateRequest{Name: "mcp", MCPServerIDs: []string{vmcp.ID}})
	return agentVMCP{
		agent:       agent,
		vmcp:        vmcp,
		componentID: vmcp.Components[0].ID,
		profileID:   vmcp.Profiles[0].ID,
		mcpKey:      key.Key,
	}
}

// configure sets the profile's value for the component's required key.
func (a agentVMCP) configure(t *testing.T, h *harness.Harness, value string) {
	t.Helper()
	h.Put(t, "/api/vmcps/"+a.vmcp.ID+"/profiles/"+a.profileID+"/configuration", types.VMCPProfileConfigurationUpdate{
		Components: map[string]map[string]*string{a.componentID: {integrationEnvKey: &value}},
	}, nil)
}

// TestAgentVMCPSetup checks, against a running server, that an agent reaches a shared vMCP only through its profile
// and an MCP-scoped key, gets a 409 that names the missing configuration instead of a challenge, that readiness agrees
// with the connect path, that profile configuration is write-only, and that deleting the agent leaves the profile's
// configuration and subjects in place. It never starts the component, so it needs no MCP runtime.
func TestAgentVMCPSetup(t *testing.T) {
	h := harness.New(t)
	a := newAgentVMCP(t, h, "agent-setup")
	llmKey := h.CreateAgentAPIKey(t, a.agent.ID, types.AgentAPIKeyCreateRequest{Name: "llm", CanAccessLLMProxy: true})

	t.Run("a key without the MCP scope cannot connect", func(t *testing.T) {
		resp := initialize(t, h, a.vmcp.ID, llmKey.Key)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("connect with a key without MCP scope: got %d, want 403\nbody: %s", resp.StatusCode, resp.Body)
		}
	})

	want := types.AgentReadinessReason{ComponentID: a.componentID, Code: types.AgentReadinessConfigurationRequired, Field: integrationEnvKey, ProfileID: a.profileID}

	t.Run("missing configuration is a 409 without a challenge", func(t *testing.T) {
		resp := initialize(t, h, a.vmcp.ID, a.mcpKey)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("connect before configuration: got %d, want 409\nbody: %s", resp.StatusCode, resp.Body)
		}
		if challenge := resp.Header.Get("WWW-Authenticate"); challenge != "" {
			t.Fatalf("an agent got a WWW-Authenticate challenge: %q", challenge)
		}
		var failure struct {
			Error   string                       `json:"error"`
			Reasons []types.AgentReadinessReason `json:"reasons"`
		}
		if err := json.Unmarshal(resp.Body, &failure); err != nil {
			t.Fatalf("decode 409 body: %v\nbody: %s", err, resp.Body)
		}
		if failure.Error != "agent_configuration_required" || !slices.Contains(failure.Reasons, want) {
			t.Fatalf("409 body: got %+v, want error agent_configuration_required with reason %+v", failure, want)
		}
	})

	t.Run("readiness agrees with the connect path", func(t *testing.T) {
		var readiness types.AgentVMCPReadiness
		h.Get(t, "/api/agents/"+a.agent.ID+"/vmcps/"+a.vmcp.ID+"/readiness", &readiness)
		if readiness.Ready || !slices.Contains(readiness.Reasons, want) {
			t.Fatalf("readiness before configuration: %+v", readiness)
		}
	})

	value := "agent-" + h.RunID

	t.Run("profile configuration is write-only and makes the agent ready", func(t *testing.T) {
		a.configure(t, h, value)

		resp := h.RequestAs(t, http.MethodGet, "/api/vmcps/"+a.vmcp.ID+"/profiles/"+a.profileID+"/status", "", nil)
		if resp.StatusCode != http.StatusOK || strings.Contains(string(resp.Body), value) {
			t.Fatalf("profile status: got %d, and it must never contain the value\nbody: %s", resp.StatusCode, resp.Body)
		}
		var readiness types.AgentVMCPReadiness
		h.Get(t, "/api/agents/"+a.agent.ID+"/vmcps/"+a.vmcp.ID+"/readiness", &readiness)
		if !readiness.Ready {
			t.Fatalf("agent is not ready after configuration: %+v", readiness)
		}
		// Agents can't reach their own setup.
		requireStatus(t, h, http.MethodGet, "/api/vmcps/"+a.vmcp.ID+"/profiles/"+a.profileID+"/status", a.mcpKey, http.StatusForbidden)
	})

	t.Run("delete keeps the profile's configuration and subject", func(t *testing.T) {
		h.Delete(t, "/api/agents/"+a.agent.ID)
		requireStatus(t, h, http.MethodPost, "/mcp-connect/"+a.vmcp.ID, a.mcpKey, http.StatusUnauthorized)

		var status types.VMCPProfileStatus
		h.Get(t, "/api/vmcps/"+a.vmcp.ID+"/profiles/"+a.profileID+"/status", &status)
		idx := slices.IndexFunc(status.Components, func(c types.VMCPProfileComponentStatus) bool { return c.ComponentID == a.componentID })
		if idx < 0 || !slices.Contains(status.Components[idx].Configuration.Set, integrationEnvKey) {
			t.Fatalf("profile configuration did not survive the agent's deletion: %+v", status)
		}
		// The deleted agent's subject stays in the profile and matches nobody, as a deleted user's does.
		var stored types.VMCP
		h.Get(t, "/api/vmcps/"+a.vmcp.ID, &stored)
		if !slices.Contains(stored.Profiles[0].Subjects, types.Subject{Type: types.SubjectTypeAgent, ID: a.agent.ID}) {
			t.Fatalf("the deleted agent's subject was removed from the profile: %+v", stored.Profiles[0].Subjects)
		}
	})
}

// TestAgentVMCPConnection_NPXEverything checks, end to end with a real Docker runtime, that an agent's runtime receives
// its profile's configuration, and that deleting the agent removes its vMCP instance and its runtime.
//
// Docker and access to the npm and GHCR registries are required, from the test and from the MCP containers.
func TestAgentVMCPConnection_NPXEverything(t *testing.T) {
	h := harness.New(t)
	a := newAgentVMCP(t, h, "agent-runtime")
	value := "agent-" + h.RunID
	a.configure(t, h, value)

	var containers []string
	if !t.Run("the agent's runtime gets the profile's configuration", func(t *testing.T) {
		before := mcpDeploymentContainers(t)
		result := callToolAsAgent(t, h.BaseURL, a.vmcp.ID, a.mcpKey, &gomcp.CallToolParams{Name: "get-env"}, 3*time.Minute)
		text := assertTextToolResult(t, result, "")
		var env map[string]string
		if err := json.Unmarshal([]byte(text), &env); err != nil {
			t.Fatalf("decode get-env result: %v", err)
		}
		if env[integrationEnvKey] != value {
			t.Fatalf("%s in the agent's runtime = %q, want the profile's value", integrationEnvKey, env[integrationEnvKey])
		}
		// Tests in this package run one at a time, so the containers started by this connect are the agent's.
		for _, id := range mcpDeploymentContainers(t) {
			if !slices.Contains(before, id) {
				containers = append(containers, id)
			}
		}
		if len(containers) == 0 {
			t.Fatalf("no Docker container was started for the agent's component server")
		}
	}) {
		return
	}

	t.Run("delete removes the agent's connection and runtime", func(t *testing.T) {
		instance := requireAgentInstance(t, h, a.agent.ID, a.vmcp.ID)
		h.Delete(t, "/api/agents/"+a.agent.ID)
		h.WaitForDeleted(t, "/api/vmcp-instances/"+instance.ID, time.Minute)
		waitForDockerContainersRemoved(t, containers, time.Minute)
	})
}

func requireStatus(t *testing.T, h *harness.Harness, method, path, bearer string, want int) harness.Response {
	t.Helper()
	resp := h.RequestAs(t, method, path, bearer, nil)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: got %d, want %d\nbody: %s", method, path, resp.StatusCode, want, resp.Body)
	}
	return resp
}

// initialize sends an MCP initialize request to the vMCP with an agent key and returns the response unread by any
// MCP client, so that tests can check its status, headers and body.
func initialize(t *testing.T, h *harness.Harness, vmcpID, key string) harness.Response {
	t.Helper()
	return h.RequestAs(t, http.MethodPost, "/mcp-connect/"+vmcpID, key, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "agent-integration-test", "version": "0"},
		},
	})
}

// callToolAsAgent calls a tool through the vMCP with an agent key. The agent's first connection deploys its
// component server, so the call is retried until timeout.
func callToolAsAgent(t *testing.T, baseURL, vmcpID, key string, params *gomcp.CallToolParams, timeout time.Duration) *gomcp.CallToolResult {
	t.Helper()
	client := mmmcphttp.NewFactory(mmmcphttp.FactoryOptions{})
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		result, err := client.CallTool(ctx, mmmcpconfig.Server{
			Name:    "agent-integration-test",
			URL:     baseURL + "/mcp-connect/" + vmcpID,
			Headers: map[string]string{"Authorization": "Bearer " + key},
		}, params)
		cancel()
		if err == nil {
			return result
		}
		lastErr = err
		if errors.Is(t.Context().Err(), context.Canceled) {
			break
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("call %s tool as agent within %s: %v", params.Name, timeout, lastErr)
	return nil
}

func requireAgentInstance(t *testing.T, h *harness.Harness, agentID, vmcpID string) types.VMCPInstance {
	t.Helper()
	var list types.VMCPInstanceList
	h.Get(t, "/api/vmcp-instances", &list)
	var found []types.VMCPInstance
	for _, instance := range list.Items {
		if instance.UserID == "agent:"+agentID && instance.VMCPID == vmcpID {
			found = append(found, instance)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one vMCP instance for agent %s on vMCP %s, got %+v", agentID, vmcpID, found)
	}
	return found[0]
}

// mcpDeploymentContainers returns the IDs of every Docker container that runs an MCP deployment.
func mcpDeploymentContainers(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "ps", "--all", "--quiet", "--no-trunc", "--filter", "label=mcp.deployment.id").CombinedOutput()
	if err != nil {
		t.Fatalf("list Docker containers: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func waitForDockerContainersRemoved(t *testing.T, ids []string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var remaining []string
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		output, err := exec.CommandContext(ctx, "docker", "ps", "--all", "--quiet", "--no-trunc").CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("list Docker containers: %v\n%s", err, output)
		}
		remaining = remaining[:0]
		for _, id := range ids {
			if strings.Contains(string(output), id) {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) == 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("Docker containers %v were not removed within %s", remaining, timeout)
}

func maskKey(key string) string {
	if i := strings.LastIndex(key, "-"); i > 0 {
		return key[:i] + "-*****"
	}
	return "*****"
}
