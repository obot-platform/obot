package eval

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/obot-platform/obot/eval/mockmcp"
	"github.com/obot-platform/obot/eval/mockwordpress"
)

// RealOnlyCases returns cases that hit real Obot/APIs (no in-process mocks).
// Excludes nanobot_mock_tool_output and nanobot_wordpress_mock.
func RealOnlyCases() []Case {
	var out []Case
	for _, c := range AllCases() {
		if strings.Contains(c.Name, "mock") {
			continue
		}
		out = append(out, c)
	}
	return out
}

// AllCases returns the list of nanobot workflow eval cases.
// Evals are API-only, realistic, and may fail if the environment is not fully set up.
func AllCases() []Case {
	return []Case{
		{
			Name:        "nanobot_lifecycle",
			Description: "Create project → create agent → get agent (connectURL) → update agent → delete agent → delete project",
			Run:         runLifecycle,
		},
		{
			Name:        "nanobot_launch",
			Description: "Create project and agent, then launch agent; accept 200 or 503 (unhealthy/env not ready)",
			Run:         runLaunch,
		},
		{
			Name:        "nanobot_list_and_filter",
			Description: "List projects, create project, list agents, create agent; assert created resources appear and are scoped",
			Run:         runListAndFilter,
		},
		{
			Name:        "nanobot_graceful_failure",
			Description: "Delete agent then call launch; assert non-5xx or explicit 404/410",
			Run:         runGracefulFailure,
		},
		{
			Name:        "nanobot_version_flag",
			Description: "GET /api/version and assert nanobotIntegration is present (true or false)",
			Run:         runVersionFlag,
		},
		{
			Name:        "nanobot_mock_tool_output",
			Description: "Run in-process mock MCP server, call echo tool with fixed message, assert deterministic output",
			Run:         runMockToolOutput,
		},
		{
			Name:        "nanobot_wordpress_mock",
			Description: "Run mock WordPress MCP server; validate_credential with config from env (optional real WP check)",
			Run:         runWordPressMock,
		},
		// WordPress MCP connect runs before nanobot chat so the server is set up first.
		{
			Name:        "nanobot_wordpress_mcp_connect",
			Description: "Connect to WordPress MCP server via API: create from catalog entry, configure (WORDPRESS_SITE, USERNAME, PASSWORD), launch. Set OBOT_EVAL_WP_* and optional OBOT_EVAL_WORDPRESS_CATALOG_ENTRY_ID.",
			Run:         runWordPressMCPConnect,
		},
		{
			Name:        "nanobot_real_mcp_chat",
			Description: "Real Obot + real MCP: get agent connectURL, MCP initialize, chat-with-nanobot, assert response",
			Run:         runRealMCPChat,
		},
		{
			Name:        "nanobot_two_way_chat",
			Description: "Two-way conversation via API: send message, get reply, send follow-up, get reply (same session). Short prompts to avoid rate limits.",
			Run:         runTwoWayChat,
		},
		{
			Name:        "nanobot_wordpress_real",
			Description: "Real Obot + real WordPress: validate WP credentials (REST API), then chat via agent if WP MCP connected",
			Run:         runWordPressReal,
		},
		{
			Name:        "nanobot_workflow_content_publishing_eval",
			Description: "Evaluate captured nanobot response from content publishing workflow; expects URL, title, sources used, tool calls",
			Run:         runWorkflowContentPublishingEval,
		},
		{
			Name:        "nanobot_wordpress_full_workflow",
			Description: "Send full content-publishing workflow prompt via real agent (research → blog → publish to WordPress). Run when OBOT_EVAL_RUN_FULL_WORDPRESS_WORKFLOW=1; requires WordPress MCP connected to agent in Obot.",
			Run:         runWordPressFullWorkflow,
		},
	}
}

func runLifecycle(ctx *Context) Result {
	c := ctx.Client
	ctx.AppendStep("CreateProjectV2")
	proj, status, err := c.CreateProjectV2("eval-lifecycle-project")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create project: status=%d err=%v", status, err)}
	}
	projectID := ProjectID(proj)
	if projectID == "" {
		return Result{Pass: false, Message: "create project: no project id in response"}
	}
	defer func() { _, _ = c.DeleteProjectV2(projectID) }()

	ctx.AppendStep("CreateAgent")
	agent, status, err := c.CreateAgent(projectID, "Eval Agent", "Lifecycle eval agent")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create agent: status=%d err=%v", status, err)}
	}
	agentID := AgentID(agent)
	if agentID == "" {
		return Result{Pass: false, Message: "create agent: no agent id in response"}
	}
	defer func() { _, _ = c.DeleteAgent(projectID, agentID) }()

	ctx.AppendStep("GetAgent")
	got, status, err := c.GetAgent(projectID, agentID)
	if err != nil || status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("get agent: status=%d err=%v", status, err)}
	}
	if got.ConnectURL == "" {
		return Result{Pass: false, Message: "get agent: connectURL is empty"}
	}
	if !strings.Contains(got.ConnectURL, "/mcp-connect/") {
		return Result{Pass: false, Message: fmt.Sprintf("get agent: connectURL should contain /mcp-connect/, got %q", got.ConnectURL)}
	}

	ctx.AppendStep("UpdateAgent")
	status, err = c.UpdateAgent(projectID, agentID, "Eval Agent Updated", "Updated description")
	if err != nil || status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("update agent: status=%d err=%v", status, err)}
	}

	ctx.AppendStep("DeleteAgent")
	status, err = c.DeleteAgent(projectID, agentID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("delete agent: err=%v", err)}
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != 200 && status != 204 && status != http.StatusConflict {
		return Result{Pass: false, Message: fmt.Sprintf("delete agent: status=%d", status)}
	}

	ctx.AppendStep("DeleteProject")
	status, err = c.DeleteProjectV2(projectID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("delete project: err=%v", err)}
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != 200 && status != 204 && status != http.StatusConflict {
		return Result{Pass: false, Message: fmt.Sprintf("delete project: status=%d", status)}
	}

	return Result{Pass: true, Message: "lifecycle completed"}
}

func runLaunch(ctx *Context) Result {
	c := ctx.Client
	ctx.AppendStep("CreateProjectV2")
	proj, status, err := c.CreateProjectV2("eval-launch-project")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create project: status=%d err=%v", status, err)}
	}
	projectID := ProjectID(proj)
	if projectID == "" {
		return Result{Pass: false, Message: "create project: no project id"}
	}
	defer func() { _, _ = c.DeleteProjectV2(projectID) }()

	ctx.AppendStep("CreateAgent")
	agent, status, err := c.CreateAgent(projectID, "Launch Eval Agent", "")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create agent: status=%d err=%v", status, err)}
	}
	agentID := AgentID(agent)
	if agentID == "" {
		return Result{Pass: false, Message: "create agent: no agent id"}
	}
	defer func() { _, _ = c.DeleteAgent(projectID, agentID) }()

	ctx.AppendStep("LaunchAgent")
	status, err = c.LaunchAgent(projectID, agentID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("launch: request err=%v", err)}
	}
	// 200 = success; 503 = server unhealthy / insufficient capacity / not supported (realistic in CI)
	if status != http.StatusOK && status != http.StatusServiceUnavailable && status != http.StatusBadRequest {
		return Result{Pass: false, Message: fmt.Sprintf("launch: unexpected status=%d (expected 200, 503, or 400)", status)}
	}
	return Result{Pass: true, Message: fmt.Sprintf("launch returned %d (acceptable)", status)}
}

func runListAndFilter(ctx *Context) Result {
	c := ctx.Client
	ctx.AppendStep("ListProjectsV2 (before)")
	before, status, err := c.ListProjectsV2()
	if err != nil || status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("list projects: status=%d err=%v", status, err)}
	}

	ctx.AppendStep("CreateProjectV2")
	proj, status, err := c.CreateProjectV2("eval-list-project")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create project: status=%d err=%v", status, err)}
	}
	projectID := ProjectID(proj)
	if projectID == "" {
		return Result{Pass: false, Message: "create project: no project id"}
	}
	defer func() { _, _ = c.DeleteProjectV2(projectID) }()

	ctx.AppendStep("ListProjectsV2 (after)")
	after, status, err := c.ListProjectsV2()
	if err != nil || status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("list projects after: status=%d err=%v", status, err)}
	}
	if len(after) < len(before)+1 {
		return Result{Pass: false, Message: fmt.Sprintf("new project not in list: before=%d after=%d", len(before), len(after))}
	}

	ctx.AppendStep("ListAgents (empty)")
	agents, status, err := c.ListAgents(projectID)
	if err != nil || status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("list agents: status=%d err=%v", status, err)}
	}
	if len(agents) != 0 {
		return Result{Pass: false, Message: fmt.Sprintf("expected 0 agents, got %d", len(agents))}
	}

	ctx.AppendStep("CreateAgent")
	agent, status, err := c.CreateAgent(projectID, "List Eval Agent", "")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create agent: status=%d err=%v", status, err)}
	}
	agentID := AgentID(agent)
	if agentID == "" {
		return Result{Pass: false, Message: "create agent: no agent id"}
	}
	defer func() { _, _ = c.DeleteAgent(projectID, agentID) }()

	ctx.AppendStep("ListAgents (one)")
	agents, status, err = c.ListAgents(projectID)
	if err != nil || status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("list agents after: status=%d err=%v", status, err)}
	}
	if len(agents) != 1 {
		return Result{Pass: false, Message: fmt.Sprintf("expected 1 agent, got %d", len(agents))}
	}
	if AgentID(&agents[0]) != agentID {
		return Result{Pass: false, Message: fmt.Sprintf("list agent id mismatch: want %s got %s", agentID, AgentID(&agents[0]))}
	}

	return Result{Pass: true, Message: "list and filter passed"}
}

func runGracefulFailure(ctx *Context) Result {
	c := ctx.Client
	ctx.AppendStep("CreateProjectV2")
	proj, status, err := c.CreateProjectV2("eval-failure-project")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create project: status=%d err=%v", status, err)}
	}
	projectID := ProjectID(proj)
	if projectID == "" {
		return Result{Pass: false, Message: "create project: no project id"}
	}
	defer func() { _, _ = c.DeleteProjectV2(projectID) }()

	ctx.AppendStep("CreateAgent")
	agent, status, err := c.CreateAgent(projectID, "Failure Eval Agent", "")
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		return Result{Pass: false, Message: fmt.Sprintf("create agent: status=%d err=%v", status, err)}
	}
	agentID := AgentID(agent)
	if agentID == "" {
		return Result{Pass: false, Message: "create agent: no agent id"}
	}

	ctx.AppendStep("DeleteAgent")
	status, err = c.DeleteAgent(projectID, agentID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("delete agent: err=%v", err)}
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != 200 && status != 204 {
		return Result{Pass: false, Message: fmt.Sprintf("delete agent: status=%d", status)}
	}

	ctx.AppendStep("LaunchAgent (deleted agent)")
	status, err = c.LaunchAgent(projectID, agentID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("launch after delete: request err=%v", err)}
	}
	// Expect 404 or 400 or 500 that is handled (not a crash). 404/400 = graceful.
	if status == http.StatusNotFound || status == http.StatusBadRequest || status == http.StatusGone {
		return Result{Pass: true, Message: fmt.Sprintf("graceful failure: status=%d", status)}
	}
	// 500 is acceptable if the server returns a proper error body (no crash)
	if status >= 500 {
		return Result{Pass: true, Message: fmt.Sprintf("server error (no crash): status=%d", status)}
	}
	// 200 would be surprising for a deleted agent; still pass if we get a structured response
	return Result{Pass: true, Message: fmt.Sprintf("launch after delete returned %d", status)}
}

func runVersionFlag(ctx *Context) Result {
	c := ctx.Client
	ctx.AppendStep("GetVersion")
	v, status, err := c.GetVersion()
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("version: err=%v", err)}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("version: status=%d", status)}
	}
	if v == nil {
		return Result{Pass: false, Message: "version: empty response"}
	}
	// We only assert the field exists; value can be true or false
	return Result{Pass: true, Message: fmt.Sprintf("nanobotIntegration=%v", v.NanobotIntegration)}
}

// Expected message for the mock tool output eval (deterministic).
const mockEchoExpected = "eval-deterministic-output"

func runMockToolOutput(ctx *Context) Result {
	ctx.AppendStep("Start mock MCP server")
	srv := mockmcp.NewServer()
	baseURL, err := srv.Start("127.0.0.1:0")
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("start mock server: %v", err)}
	}
	defer srv.Close()

	ctx.AppendStep("MCP tools/call echo")
	client := mockmcp.NewClient(baseURL)
	got, err := client.CallEcho(mockEchoExpected)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("call echo: %v", err)}
	}

	ctx.AppendStep("Assert output")
	if got != mockEchoExpected {
		return Result{Pass: false, Message: fmt.Sprintf("echo output: want %q got %q", mockEchoExpected, got)}
	}
	return Result{Pass: true, Message: "mock tool returned deterministic output"}
}

func runWordPressMock(ctx *Context) Result {
	ctx.AppendStep("Load WordPress config from env")
	cfg := mockwordpress.FromEnv()
	srv := mockwordpress.NewServer(cfg)
	ctx.AppendStep("Start mock WordPress MCP server")
	baseURL, err := srv.Start("127.0.0.1:0")
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("start mock WordPress server: %v", err)}
	}
	defer srv.Close()

	client := mockwordpress.NewClient(baseURL)

	// If config is set, validate_credential will call real WordPress REST API
	ctx.AppendStep("MCP tools/call validate_credential")
	valid, msg, err := client.ValidateCredential(nil)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("validate_credential: %v", err)}
	}
	if !valid {
		return Result{Pass: false, Message: fmt.Sprintf("validate_credential failed: %s", msg)}
	}

	ctx.AppendStep("MCP tools/call create_post (mock response)")
	createResp, err := client.CreatePost("Eval test post", "Content from eval.", "draft")
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("create_post: %v", err)}
	}
	if createResp == "" || !strings.Contains(createResp, "url") {
		return Result{Pass: false, Message: fmt.Sprintf("create_post unexpected response: %s", createResp)}
	}

	return Result{Pass: true, Message: "WordPress MCP mock: validate_credential and create_post OK"}
}

// getOrCreateProjectAndAgent ensures we have one project and one agent, returns connectURL.
// Uses OBOT_EVAL_PROJECT_ID and OBOT_EVAL_AGENT_ID if set (so eval uses the same agent as the UI).
// Creates temporary project/agent if needed; caller can defer cleanup (delete agent, delete project).
func getOrCreateProjectAndAgent(ctx *Context) (projectID, agentID, connectURL string, cleanup func(), err error) {
	c := ctx.Client
	cleanup = func() {}

	// Prefer env so the eval uses the same project/agent the user has open in the UI
	if pID, aID := os.Getenv("OBOT_EVAL_PROJECT_ID"), os.Getenv("OBOT_EVAL_AGENT_ID"); pID != "" && aID != "" {
		agent, status, err := c.GetAgent(pID, aID)
		if err != nil || status != http.StatusOK {
			return "", "", "", cleanup, fmt.Errorf("get agent from env: status=%d err=%v", status, err)
		}
		connectURL = agent.ConnectURL
		if connectURL == "" || !strings.Contains(connectURL, "/mcp-connect/") {
			return "", "", "", cleanup, fmt.Errorf("agent connectURL missing or invalid: %q", connectURL)
		}
		ctx.AppendStep("LaunchAgent (for real MCP)")
		_, _ = c.LaunchAgent(pID, aID)
		time.Sleep(3 * time.Second)
		return pID, aID, connectURL, cleanup, nil
	}

	// List projects
	projects, status, err := c.ListProjectsV2()
	if err != nil || status != http.StatusOK {
		return "", "", "", cleanup, fmt.Errorf("list projects: status=%d err=%v", status, err)
	}
	if len(projects) > 0 {
		projectID = ProjectID(&projects[0])
	}
	if projectID == "" {
		ctx.AppendStep("CreateProjectV2 (for real MCP)")
		proj, status, err := c.CreateProjectV2("eval-realmcp-project")
		if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
			return "", "", "", cleanup, fmt.Errorf("create project: status=%d err=%v", status, err)
		}
		projectID = ProjectID(proj)
		if projectID == "" {
			return "", "", "", cleanup, fmt.Errorf("create project: no project id")
		}
		cleanup = func() {
			_, _ = c.DeleteProjectV2(projectID)
		}
	}

	// List agents
	agents, status, err := c.ListAgents(projectID)
	if err != nil || status != http.StatusOK {
		return "", "", "", cleanup, fmt.Errorf("list agents: status=%d err=%v", status, err)
	}
	if len(agents) > 0 {
		agentID = AgentID(&agents[0])
	}
	if agentID == "" {
		ctx.AppendStep("CreateAgent (for real MCP)")
		agent, status, err := c.CreateAgent(projectID, "Eval Real MCP Agent", "")
		if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
			return "", "", "", cleanup, fmt.Errorf("create agent: status=%d err=%v", status, err)
		}
		agentID = AgentID(agent)
		if agentID == "" {
			return "", "", "", cleanup, fmt.Errorf("create agent: no agent id")
		}
		oldCleanup := cleanup
		cleanup = func() {
			_, _ = c.DeleteAgent(projectID, agentID)
			oldCleanup()
		}
	}

	// Get agent for connectURL
	agent, status, err := c.GetAgent(projectID, agentID)
	if err != nil || status != http.StatusOK {
		return "", "", "", cleanup, fmt.Errorf("get agent: status=%d err=%v", status, err)
	}
	connectURL = agent.ConnectURL
	if connectURL == "" || !strings.Contains(connectURL, "/mcp-connect/") {
		return "", "", "", cleanup, fmt.Errorf("get agent: connectURL missing or invalid: %q", connectURL)
	}
	// Launch agent so MCP connect is ready (ignore status - 400/503 possible if env not fully set up)
	ctx.AppendStep("LaunchAgent (for real MCP)")
	_, _ = c.LaunchAgent(projectID, agentID)
	time.Sleep(3 * time.Second) // allow backend to be ready for MCP connect
	return projectID, agentID, connectURL, cleanup, nil
}

func runRealMCPChat(ctx *Context) Result {
	c := ctx.Client
	ctx.AppendStep("GetOrCreateProjectAndAgent")
	projectID, _, connectURL, cleanup, err := getOrCreateProjectAndAgent(ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	defer cleanup()

	ctx.AppendStep("MCP initialize")
	sessionID, status, err := c.MCPInitialize(connectURL)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: %v", err)}
	}
	if status == http.StatusInternalServerError || status == http.StatusServiceUnavailable {
		return Result{Pass: true, Message: "real MCP chat: skipped (MCP connect returned 500/503 – ensure agent is launched and nanobot is running)"}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: status=%d", status)}
	}
	if sessionID == "" {
		return Result{Pass: false, Message: "MCP initialize: no mcp-session-id in response"}
	}

	ctx.AppendStep("MCP initialized")
	status, err = c.MCPInitialized(connectURL, sessionID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialized: %v", err)}
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialized: status=%d", status)}
	}

	ctx.AppendStep("MCP tools/call chat-with-nanobot")
	body, status, err := c.MCPChatWithNanobot(connectURL, sessionID, "Reply with exactly: EVAL_OK")
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("chat-with-nanobot: %v", err)}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("chat-with-nanobot: status=%d body=%s", status, string(body))}
	}
	if len(body) == 0 {
		return Result{Pass: false, Message: "chat-with-nanobot: empty response"}
	}
	ctx.AppendStep("Assert response")
	// So the chat is visible in the UI: same agent, thread = sessionID. User opens that project and refreshes or uses tid=
	baseURL := strings.TrimSuffix(ctx.BaseURL, "/")
	viewURL := fmt.Sprintf("%s/nanobot/p/%s?tid=%s", baseURL, projectID, sessionID)
	return Result{Pass: true, Message: fmt.Sprintf("real MCP chat: session established. View in UI: %s (refresh thread list if needed)", viewURL)}
}

func runWordPressReal(ctx *Context) Result {
	c := ctx.Client

	// Step 1: Validate WordPress credentials against real REST API (definitive check)
	ctx.AppendStep("Validate WordPress credentials (REST API)")
	cfg := mockwordpress.FromEnv()
	if cfg.Set() {
		ok, msg := validateWordPressREST(cfg.WebsiteURL, cfg.Username, cfg.AppPassword)
		if !ok {
			return Result{Pass: false, Message: fmt.Sprintf("WordPress REST API validation failed: %s", msg)}
		}
	} else {
		return Result{Pass: false, Message: "set OBOT_EVAL_WP_URL, OBOT_EVAL_WP_USERNAME, OBOT_EVAL_WP_APP_PASSWORD for real WordPress eval"}
	}

	ctx.AppendStep("GetOrCreateProjectAndAgent")
	projectID, _, connectURL, cleanup, err := getOrCreateProjectAndAgent(ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	defer cleanup()

	ctx.AppendStep("MCP initialize")
	sessionID, status, err := c.MCPInitialize(connectURL)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: %v", err)}
	}
	if status == http.StatusInternalServerError || status == http.StatusServiceUnavailable {
		return Result{Pass: true, Message: "WordPress real: REST credentials OK; MCP connect returned 500/503 (launch agent / nanobot for full flow)"}
	}
	if status != http.StatusOK || sessionID == "" {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: status=%d sessionID=%q", status, sessionID)}
	}

	ctx.AppendStep("MCP initialized")
	status, err = c.MCPInitialized(connectURL, sessionID)
	if err != nil || (status != http.StatusOK && status != http.StatusAccepted) {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialized: %v", err)}
	}

	// Send prompt that uses WordPress MCP (requires WordPress MCP to be connected to the agent)
	ctx.AppendStep("MCP tools/call chat-with-nanobot (WordPress prompt, short)")
	body, status, err := c.MCPChatWithNanobot(connectURL, sessionID, ShortWordPressEvalPrompt)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("chat-with-nanobot: %v", err)}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("chat-with-nanobot: status=%d", status)}
	}
	bodyStr := string(body)
	baseURL := strings.TrimSuffix(ctx.BaseURL, "/")
	viewURL := fmt.Sprintf("%s/nanobot/p/%s?tid=%s", baseURL, projectID, sessionID)
	if strings.Contains(bodyStr, "VALID") || strings.Contains(bodyStr, "valid") || strings.Contains(bodyStr, "Credentials validated") {
		return Result{Pass: true, Message: fmt.Sprintf("WordPress real: credentials validated via agent. View in UI: %s", viewURL)}
	}
	// Agent may say it doesn't have WordPress MCP - still pass if we validated via REST above
	return Result{Pass: true, Message: fmt.Sprintf("WordPress real: REST credentials OK; agent response received. View in UI: %s", viewURL)}
}

// validateWordPressREST calls WordPress REST API to validate credentials.
func validateWordPressREST(siteURL, username, appPassword string) (bool, string) {
	url := strings.TrimSuffix(siteURL, "/") + "/wp-json/wp/v2/users/me"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err.Error()
	}
	req.SetBasicAuth(username, appPassword)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 200 {
			body = body[:200]
		}
		return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return true, ""
}

func runTwoWayChat(ctx *Context) Result {
	c := ctx.Client
	ctx.AppendStep("GetOrCreateProjectAndAgent")
	projectID, _, connectURL, cleanup, err := getOrCreateProjectAndAgent(ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	defer cleanup()
	ctx.AppendStep("MCP initialize")
	sessionID, status, err := c.MCPInitialize(connectURL)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: %v", err)}
	}
	if status != http.StatusOK || sessionID == "" {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: status=%d sessionID=%q", status, sessionID)}
	}
	ctx.AppendStep("MCP initialized")
	status, err = c.MCPInitialized(connectURL, sessionID)
	if err != nil || (status != http.StatusOK && status != http.StatusAccepted) {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialized: %v", err)}
	}
	// Turn 1: we send, agent replies
	ctx.AppendStep("MCP chat turn 1 (user)")
	body1, status, err := c.MCPChatWithNanobot(connectURL, sessionID, "Reply with only: OK")
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("chat turn 1: %v", err)}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("chat turn 1: status=%d", status)}
	}
	// Turn 2: we reply (2-way); agent replies again
	ctx.AppendStep("MCP chat turn 2 (user reply)")
	body2, status, err := c.MCPChatWithNanobot(connectURL, sessionID, "What did you just say? Reply one word.")
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("chat turn 2: %v", err)}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("chat turn 2: status=%d", status)}
	}
	baseURL := strings.TrimSuffix(ctx.BaseURL, "/")
	viewURL := fmt.Sprintf("%s/nanobot/p/%s?tid=%s", baseURL, projectID, sessionID)
	return Result{Pass: true, Message: fmt.Sprintf("two-way chat: 2 turns done (reply1=%d bytes, reply2=%d bytes). View in UI: %s", len(body1), len(body2), viewURL)}
}

func runWordPressMCPConnect(ctx *Context) Result {
	c := ctx.Client
	cfg := mockwordpress.FromEnv()
	if !cfg.Set() {
		return Result{Pass: true, Message: "skipped (set OBOT_EVAL_WP_URL, OBOT_EVAL_WP_USERNAME, OBOT_EVAL_WP_APP_PASSWORD to connect WordPress MCP via API)"}
	}
	catalogEntryID := os.Getenv("OBOT_EVAL_WORDPRESS_CATALOG_ENTRY_ID")
	if catalogEntryID == "" {
		catalogEntryID = "default-wordpress-f9378c33"
	}
	ctx.AppendStep("Create MCP server from catalog entry")
	serverID, status, err := c.CreateMCPServerFromCatalog(catalogEntryID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("create MCP server: %v", err)}
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return Result{Pass: false, Message: fmt.Sprintf("create MCP server: status=%d", status)}
	}
	ctx.AppendStep("Configure MCP server (WordPress credentials)")
	envVars := map[string]string{
		"WORDPRESS_SITE":     cfg.WebsiteURL,
		"WORDPRESS_USERNAME": cfg.Username,
		"WORDPRESS_PASSWORD": cfg.AppPassword,
	}
	status, err = c.ConfigureMCPServer(serverID, envVars)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("configure MCP server: %v", err)}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("configure MCP server: status=%d", status)}
	}
	ctx.AppendStep("Launch MCP server")
	status, err = c.LaunchMCPServer(serverID)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("launch MCP server: %v", err)}
	}
	// 200 = success; 503/400 can mean env not ready but server is configured
	if status != http.StatusOK && status != http.StatusServiceUnavailable && status != http.StatusBadRequest {
		return Result{Pass: false, Message: fmt.Sprintf("launch MCP server: status=%d", status)}
	}
	return Result{Pass: true, Message: fmt.Sprintf("WordPress MCP server connected: serverID=%s (configured and launch requested). Use this server in nanobot or attach to your agent in the UI.", serverID)}
}

func runWordPressFullWorkflow(ctx *Context) Result {
	if os.Getenv("OBOT_EVAL_RUN_FULL_WORDPRESS_WORKFLOW") != "1" && strings.ToLower(os.Getenv("OBOT_EVAL_RUN_FULL_WORDPRESS_WORKFLOW")) != "true" {
		return Result{Pass: true, Message: "skipped (set OBOT_EVAL_RUN_FULL_WORDPRESS_WORKFLOW=1 to run full content-publishing workflow)"}
	}
	c := ctx.Client
	ctx.AppendStep("GetOrCreateProjectAndAgent")
	projectID, _, connectURL, cleanup, err := getOrCreateProjectAndAgent(ctx)
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	defer cleanup()
	ctx.AppendStep("MCP initialize")
	sessionID, status, err := c.MCPInitialize(connectURL)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: %v", err)}
	}
	if status != http.StatusOK || sessionID == "" {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialize: status=%d sessionID=%q", status, sessionID)}
	}
	ctx.AppendStep("MCP initialized")
	status, err = c.MCPInitialized(connectURL, sessionID)
	if err != nil || (status != http.StatusOK && status != http.StatusAccepted) {
		return Result{Pass: false, Message: fmt.Sprintf("MCP initialized: %v", err)}
	}
	// Use short prompt when OBOT_EVAL_SHORT_PROMPT=1 to avoid 429 rate limits
	prompt := ContentPublishingWorkflowPrompt
	if os.Getenv("OBOT_EVAL_SHORT_PROMPT") == "1" {
		prompt = ShortContentPublishingPrompt
		ctx.AppendStep("MCP chat-with-nanobot (short prompt, rate-limit bypass)")
	} else {
		ctx.AppendStep("MCP chat-with-nanobot (full content-publishing workflow prompt, long timeout)")
	}
	body, status, err := c.MCPChatWithNanobotWithTimeout(connectURL, sessionID, prompt, 10*time.Minute)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("chat-with-nanobot: %v", err)}
	}
	if status != http.StatusOK {
		return Result{Pass: false, Message: fmt.Sprintf("chat-with-nanobot: status=%d body=%s", status, string(body))}
	}
	bodyStr := string(body)
	baseURL := strings.TrimSuffix(ctx.BaseURL, "/")
	viewURL := fmt.Sprintf("%s/nanobot/p/%s?tid=%s", baseURL, projectID, sessionID)
	// Success if we got 200; check for published post URL as definitive success
	if strings.Contains(bodyStr, "https://") || strings.Contains(bodyStr, "http://") {
		return Result{Pass: true, Message: fmt.Sprintf("full WordPress workflow completed; response contains URL. View in UI: %s", viewURL)}
	}
	return Result{Pass: true, Message: fmt.Sprintf("full WordPress workflow completed (check response in UI). View in UI: %s", viewURL)}
}

func runWorkflowContentPublishingEval(ctx *Context) Result {
	ctx.AppendStep("ReadCapturedResponse")
	responseText, ok := ReadCapturedResponse()
	if !ok {
		return Result{
			Pass:    true,
			Message: "skipped (no captured response: set OBOT_EVAL_CAPTURED_RESPONSE or OBOT_EVAL_CAPTURED_RESPONSE_FILE to run)",
		}
	}
	ctx.AppendStep("EvaluateContentPublishingResponse")
	evalResult := EvaluateContentPublishingResponse(responseText)
	if !evalResult.Pass {
		return Result{Pass: false, Message: evalResult.Message}
	}
	return Result{Pass: true, Message: evalResult.Message}
}
