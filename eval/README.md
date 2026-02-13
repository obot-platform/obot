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
  - **Bootstrap token** (initial setup only): If the instance is still in bootstrap mode (no admin user created yet), you can use the bootstrap token. Either as a cookie (same value set by `POST /api/bootstrap/login`) or as Bearer:
    ```bash
    export OBOT_EVAL_AUTH_HEADER="Cookie: obot-bootstrap=setuptoken"
    # or
    export OBOT_EVAL_AUTH_HEADER="Bearer setuptoken"
    ```
    Bootstrap auth is disabled once a non-bootstrap admin exists; then use session cookie or Bearer above.
  - **Note**: The standard API key from Obot (GroupAPIKey) only grants `/mcp-connect/` and `/api/me`; it does **not** grant `POST/GET /api/projectsv2` or agents. For CI, a backend change may be needed to issue a test token with Basic scope.

### Run all evals

```bash
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=..."   # or Bearer ...

go test -v ./eval/... -run TestEvalNanobotWorkflow
```

### Example: values from your browser (curl / DevTools)

You can copy the cookie and IDs from the network tab or from curl commands:

1. **Cookie** – From any request to `localhost:8080`, use the `Cookie` header value:
   ```bash
   export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=YOUR_VALUE_HERE"
   ```
   The value is everything after `obot_access_token=` (often looks like `base64|timestamp|signature`).

2. **Project ID** – From the nanobot URL `/nanobot/p/<id>` or from `GET /api/projectsv2` (each item has `id`). Example: `pv21hg7tk`.

3. **Agent ID** – From `GET /api/projectsv2/<projectId>/agents` (each agent has `id` or `name`). Or from the MCP connect URL: if the UI calls `/mcp-connect/ms1nba14r6cq`, the connect ID is `ms1nba14r6cq`; the **agent ID** is the part after the `ms1` prefix, e.g. `nba14r6cq`.

4. **Full example** (same agent as the one open in the UI, so the eval chat appears there):
   ```bash
   export OBOT_EVAL_BASE_URL=http://localhost:8080
   export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=djIuYjJKdmRGOWhZMk5sYzNOZmRHOXJaVzR0WlRobE1tSTNZVFkxTURCa05qQXhaV1l6Tm1NMU9HVXdZMll6TUdJeE0yRS43Qllac3ZIeDJlb29LM29aWHFUVW93|1770929727|ldqjysKA2kCMR29RqXSmgDJyaNJ7ftNz8k4SICbO_CQ="
   export OBOT_EVAL_PROJECT_ID=pv21hg7tk
   export OBOT_EVAL_AGENT_ID=nba14r6cq
   go test -v ./eval/... -run TestEvalNanobotWorkflow
   ```
   After the test, the **View in UI** link will open the thread in the same project/agent you have open.

To write results to JSON:

```bash
export OBOT_EVAL_RESULTS_JSON=eval-results.json
go test -v ./eval/... -run TestEvalNanobotWorkflow
# or run TestEvalWriteResults after the above to only write JSON
```

### Skip evals (no env)

If `OBOT_EVAL_BASE_URL` is not set, the test is skipped. This keeps `go test ./...` fast when not running against a live instance.

### Run real Obot + real MCP evals (no mocks)

These cases use **real** Obot and **real** MCP (no in-process mocks):

1. **nanobot_real_mcp_chat** – Needs only Obot URL + auth. Gets or creates a project and agent, opens an MCP session to the agent’s `connectURL`, and sends a `chat-with-nanobot` prompt. Definitive pass: HTTP 200 and non-empty response.

2. **nanobot_wordpress_real** – Needs Obot URL + auth **and** WordPress config. Validates credentials against the real WordPress REST API, then (optionally) sends a WordPress validation prompt via the agent. Set:

**Use the same agent as the UI** (so eval chat appears in the project you have open):

```bash
export OBOT_EVAL_PROJECT_ID="<project-id>"   # from nanobot URL: /nanobot/p/<project-id>
export OBOT_EVAL_AGENT_ID="<agent-id>"       # from agent settings or API
```

If both are set, the eval uses that project/agent instead of creating or picking another. Then the chat thread created by the eval belongs to that agent and is visible in the nanobot UI for that project.

WordPress real env and run:
   ```bash
   export OBOT_EVAL_BASE_URL=http://localhost:8080
   export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=..."
   export OBOT_EVAL_WP_URL="https://testsite041stg.wpenginepowered.com"
   export OBOT_EVAL_WP_USERNAME="editorbot"
   export OBOT_EVAL_WP_APP_PASSWORD="your-application-password"
   go test -v ./eval/... -run TestEvalNanobotWorkflow
   ```
   For the agent to reply “VALID”, connect the WordPress MCP server to your agent in Obot (UI or API). The eval still passes if REST validation succeeds and the agent responds.

**Run the full WordPress content-publishing workflow** (research → blog post → publish to WordPress): (1) In Obot, add and configure the WordPress MCP server and connect it to your nanobot agent (site URL, username, application password). (2) Set `OBOT_EVAL_RUN_FULL_WORDPRESS_WORKFLOW=1` and run the test; the **nanobot_wordpress_full_workflow** case sends the full prompt and can take several minutes. The output includes a View in UI link to verify the published post. To avoid **429 rate limits**, set `OBOT_EVAL_SHORT_PROMPT=1` so the workflow case uses a short prompt (list one post, reply with title or EVAL_OK). WordPress eval cases also use short prompts (`ShortWordPressEvalPrompt`, `ShortContentPublishingPrompt` in `eval/workflow_prompt.go`).

### See eval chat in the UI

When **nanobot_real_mcp_chat** or **nanobot_wordpress_real** passes, the test output includes a **View in UI** URL, e.g.:

`http://localhost:8080/nanobot/p/<projectId>?tid=<sessionID>`

- Open that URL in the same browser where you're logged in (or use the same session cookie). The `tid` is the thread (MCP session) created by the eval; the page will open that thread so you see the eval chat.
- If you set `OBOT_EVAL_PROJECT_ID` and `OBOT_EVAL_AGENT_ID` to the project/agent you have open in the nanobot UI, the new thread belongs to that agent. Refresh the thread list in the sidebar (or reload the page) to see the new thread; or use the printed URL with `?tid=...` to open it directly.

### Troubleshooting: "missing required config" in the UI

If opening the nanobot UI or the View-in-UI link returns an error like:

`failed to ensure server is deployed: failed to get mcp server config: error code 400 (Bad Request): missing required config: ANTHROPIC_BASE_URL, OPENAI_BASE_URL, ...`

then the **nanobot agent’s MCP server credential was never created or failed to be created**. That credential is created by the Obot controller when the agent (and its MCP server) is created. It injects LLM proxy URLs, API keys, and MCP tokens into the credential so the nanobot container can talk to Obot.

**What to check on the Obot side:**

1. **Controller and gateway** – The controller must run `ensureCredentials` for the nanobot agent, which calls the gateway to create an API key and an MCP token for the user. Ensure:
   - The Obot controller is running and can reach the gateway (same process or configured gateway URL).
   - The gateway supports creating API keys and Obot MCP tokens (standard when running Obot with the built-in gateway).

2. **User exists in the gateway** – Credential creation looks up the user by ID (`UserByID`). The user is created when they first log in. If the agent was created under a user that doesn’t exist in the gateway, credential creation can fail.

3. **Controller logs** – When an agent is created or updated, the controller creates or refreshes the credential. If that fails, the controller logs the error (e.g. failed to create API key or MCP token). Check controller logs around the time the agent was created.

4. **Re-trigger credential creation** – If the credential was missing due to a transient failure, you can try updating the agent in the UI (e.g. change display name and save) so the controller runs again and retries `ensureCredentials`.

Once the credential exists and contains the required keys, the nanobot UI and the View-in-UI link should load without that error.

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
| **nanobot_wordpress_mock** | In-process mock WordPress MCP server; calls `validate_credential` (optionally against real WordPress when config env vars are set) and `create_post`. |
| **nanobot_real_mcp_chat** | **Real Obot + real MCP** (no mock): get or create project/agent, get `connectURL`, MCP `initialize` → `initialized` → `tools/call` `chat-with-nanobot` with a fixed prompt; assert 200 and response. |
| **nanobot_two_way_chat** | Two-way conversation via API: send a message, get reply, send a follow-up in the same session, get reply. Uses short prompts to avoid rate limits. |
| **nanobot_wordpress_real** | **Real Obot + real WordPress**: (1) Validate WordPress credentials via REST API (`OBOT_EVAL_WP_*` env vars). (2) MCP session to agent and send WordPress validation prompt; pass if REST OK and agent responds. For full flow, connect WordPress MCP to your agent in Obot. |
| **nanobot_workflow_content_publishing_eval** | Evaluates a **captured** nanobot response from the content-publishing workflow. Expects: published post URL, title (optional), number of sources used, number of tool calls made. |
| **nanobot_wordpress_full_workflow** | Sends the **full** content-publishing workflow prompt (research → blog → publish to WordPress) via real agent. Set `OBOT_EVAL_RUN_FULL_WORDPRESS_WORKFLOW=1` to run; requires WordPress MCP connected to agent. Can take several minutes. |
| **nanobot_wordpress_mcp_connect** | Connects to the **real** WordPress MCP server via Obot API: create server from catalog entry, configure with `WORDPRESS_SITE` / `WORDPRESS_USERNAME` / `WORDPRESS_PASSWORD`, then launch. Set `OBOT_EVAL_WP_*`; optional `OBOT_EVAL_WORDPRESS_CATALOG_ENTRY_ID` (default `default-wordpress-f9378c33`). |

### WordPress MCP connect API (what the UI does)

To connect to the WordPress MCP server by API (same as the UI): (1) `POST /api/mcp-servers` with body `{"catalogEntryID":"default-wordpress-f9378c33","manifest":{}}` to create the server; (2) `POST /api/mcp-servers/{id}/configure` with body `{"WORDPRESS_SITE":"...","WORDPRESS_USERNAME":"...","WORDPRESS_PASSWORD":"..."}`; (3) `POST /api/mcp-servers/{id}/launch` with body `{}`. The eval client provides `CreateMCPServerFromCatalog`, `ConfigureMCPServer`, and `LaunchMCPServer`; the **nanobot_wordpress_mcp_connect** case runs all three when `OBOT_EVAL_WP_*` is set.

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

If neither `OBOT_EVAL_CAPTURED_RESPONSE` nor `OBOT_EVAL_CAPTURED_RESPONSE_FILE` is set, the case is skipped (counts as pass).

## Mock WordPress MCP server (`eval/mockwordpress/`)

A mock WordPress MCP server used by the **nanobot_wordpress_mock** eval. It exposes WordPress-like tools (`validate_credential`, `get_site_settings`, `create_post`, `list_posts`) over the same JSON-RPC 2.0 over HTTP protocol as other MCP servers.

- **Without config**: The mock runs in-process; `validate_credential` fails (missing credentials). The eval still passes if the mock responds correctly; for a full pass with real validation, set the config below.
- **With config**: Set these env vars to validate against a real WordPress instance (e.g. staging). The mock then calls the WordPress REST API (`GET /wp-json/wp/v2/users/me`) with Basic auth (username + application password) to verify credentials.

```bash
export OBOT_EVAL_WP_URL="https://yoursite.wpenginepowered.com"
export OBOT_EVAL_WP_USERNAME="editorbot"
export OBOT_EVAL_WP_APP_PASSWORD="your-application-password"
go test -v ./eval/... -run TestEvalNanobotWorkflow
```

To run only the WordPress mock eval (no Obot or auth required if you only run this case):

```bash
export OBOT_EVAL_WP_URL="https://yoursite.wpenginepowered.com"
export OBOT_EVAL_WP_USERNAME="editorbot"
export OBOT_EVAL_WP_APP_PASSWORD="your-application-password"
go test -v ./eval/... -run TestEvalNanobotWorkflow
```

**Note**: The backend does not implement WordPress MCP; it only proxies MCP at `/mcp-connect/{mcp_id}`. This mock is for evals only and mimics the WordPress MCP tool set.

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
