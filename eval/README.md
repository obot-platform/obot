# Nanobot Workflow Evals

API-driven evaluation framework for the nanobot workflow feature. No UI automation; evals use REST APIs only. Cases are realistic and may fail if the environment is not fully set up (e.g. launch can return 503)—failures indicate work to do, not broken evals.

## Running evals

### Prerequisites

- A running Obot instance with nanobot integration enabled (`nanobotIntegration: true` in `/api/version`).
- Authentication: project/agent APIs require an authenticated user. Use one of:
  - **Session cookie**: After logging in via the UI, copy the `obot_access_token` cookie value and set:
    ```bash
    export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=<paste-value>"
    ```
  - **Bearer token**: If your deployment supports a JWT or API key with project/agent scope, set:
    ```bash
    export OBOT_EVAL_AUTH_HEADER="Bearer <token>"
    ```
  - **Note**: The standard API key from Obot (GroupAPIKey) only grants `/mcp-connect/` and `/api/me`; it does **not** grant `POST/GET /api/projectsv2` or agents. For CI, a backend change may be needed to issue a test token with Basic scope.

### Run all evals

```bash
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=..."   # or Bearer ...

go test -v ./eval/... -run TestEvalNanobotWorkflow
```

To write results to JSON:

```bash
export OBOT_EVAL_RESULTS_JSON=eval-results.json
go test -v ./eval/... -run TestEvalNanobotWorkflow
# or run TestEvalWriteResults after the above to only write JSON
```

### Skip evals (no env)

If `OBOT_EVAL_BASE_URL` is not set, the test is skipped. This keeps `go test ./...` fast when not running against a live instance.

### Run only the mock tool-output eval (no Obot)

The mock MCP eval runs in-process and does not need a live Obot instance or auth:

```bash
go test -v ./eval/... -run TestEvalMockToolOutput
```

## Eval cases

| Name | Description |
|------|-------------|
| **nanobot_lifecycle** | Create project → create agent → get agent (assert `connectURL`) → update agent → delete agent → delete project. |
| **nanobot_launch** | Create project and agent, then `POST .../launch`. Pass on 200 (success) or 503/400 (unhealthy or not supported in this env). |
| **nanobot_list_and_filter** | List projects, create project, list agents (empty), create agent, list agents (one). Assert created resources appear and are scoped. |
| **nanobot_graceful_failure** | Create agent, delete agent, then call launch. Assert non-crash: 404/400 or handled 5xx. |
| **nanobot_version_flag** | `GET /api/version` and assert `nanobotIntegration` is present (true or false). |
| **nanobot_mock_tool_output** | In-process mock MCP server exposes an `echo` tool; eval calls it with a fixed message and asserts deterministic output. No Obot required. |
| **nanobot_workflow_content_publishing_eval** | Evaluates a **captured** nanobot response from the content-publishing workflow. Expects: published post URL, title (optional), number of sources used, number of tool calls made. |

## Content publishing workflow eval

This eval checks nanobot’s **output** after running the “content publishing workflow” prompt (research → blog post → publish to WordPress). You run the workflow in nanobot, capture the final response, then run the eval on that text.

### 1. Prompt to use in nanobot

The exact prompt is in code as `eval.ContentPublishingWorkflowPrompt` (see `eval/workflow_prompt.go`). In short: create an automated workflow that searches for “latest AI test automation tools and trends 2026”, picks 5 sources, generates a blog post, publishes to WordPress, and returns **only**:

- published post URL  
- title  
- number of sources used  
- number of tool calls made  

### 2. Capture the response

After nanobot finishes:

- Copy the **final assistant reply** (the part that should contain the four items above), or  
- Save it to a file (e.g. `captured_response.txt`).

### 3. Run the eval

**Option A – env var (small response):**

```bash
export OBOT_EVAL_CAPTURED_RESPONSE="published post URL: https://mysite.com/post/123
title: AI Test Automation in 2026
number of sources used: 5
number of tool calls made: 12"
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: ..."
go test -v ./eval/... -run TestEvalNanobotWorkflow
```

**Option B – file (long response):**

```bash
export OBOT_EVAL_CAPTURED_RESPONSE_FILE=./captured_response.txt
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: ..."
go test -v ./eval/... -run TestEvalNanobotWorkflow
```

The eval **passes** if the text contains:

- At least one `http`/`https` URL (published post URL), and  
- At least one of: “number of sources used” (with a number) or “number of tool calls made” (with a number).  

Title is optional but is detected when present (e.g. “title: …” or a short line).

If neither `OBOT_EVAL_CAPTURED_RESPONSE` nor `OBOT_EVAL_CAPTURED_RESPONSE_FILE` is set, the case fails with a message asking you to set one of them.

## Mock MCP server (`eval/mockmcp/`)

A minimal MCP server used by the **nanobot_mock_tool_output** eval:

- **Protocol**: JSON-RPC 2.0 over HTTP (POST). Implements `initialize`, `notifications/initialized`, `tools/list`, `tools/call`.
- **Tool**: Single tool `echo` with argument `message` (string). Returns `{ "content": [{ "type": "text", "text": "<message>" }], "isError": false }`.
- **Usage**: The eval starts the server on `127.0.0.1:0`, calls `tools/call` with `message: "eval-deterministic-output"`, and asserts the returned text matches. You can reuse this server for future evals (e.g. register its URL in a catalog and run through Obot).

## Design notes

- **Task completion**: Lifecycle and list_and_filter assert that the intended state is reached (resources created, connectURL present, list includes new items).
- **Trajectory**: Each case can log steps in `Result.Trajectory` for debugging and future trajectory-quality checks.
- **Determinism**: Environment (Obot + auth) should be stable; the mock server provides deterministic tool output for the tool-output eval.
- **Mock MCP**: The in-repo mock in `eval/mockmcp/` provides a deterministic `echo` tool for the **nanobot_mock_tool_output** eval; it can be extended or reused for e2e evals that register the mock in a catalog.

## Backend / CI notes

- **Auth for CI**: To run evals in CI without a browser, the backend could support a test-only token (e.g. `OBOT_EVAL_TOKEN` or a dedicated API key scope) that grants Basic user access to `POST/GET /api/projectsv2` and agents. Currently only session cookie or a Bearer token that the gateway accepts for full API access will work.
- **Launch**: Launch may return 503 (insufficient capacity, unhealthy) in environments where nanobot containers are not fully configured; the launch eval accepts 200, 503, or 400 as acceptable outcomes.
