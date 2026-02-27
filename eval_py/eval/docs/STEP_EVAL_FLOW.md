# Step Eval Test Case – Full Flow (for Dev)

**Test case name:** `nanobot_workflow_content_publishing_step_eval`  
**Purpose:** Send one phased prompt to the nanobot via MCP, get the assistant reply from the event stream, and assert success. All API calls are logged when `OBOT_EVAL_API_LOG` is set.

---

## 1. Prompt used in this test

We send **only Step 1** of the content publishing workflow (first entry of phased prompts):

```
Start the content publishing workflow. Step 1: Use Web/Search MCP to search for "latest AI test automation tools and trends 2026". Select the top 5 credible sources. Do not ask follow-up questions; use defaults if needed.
```

*(Full phased list exists in code for future steps; this test sends only phase 0.)*

---

## 2. End-to-end flow (in order)

### Step 2.1 – Version check

- **API:** `GET {base_url}/api/version`
- **Auth:** Same as rest (Cookie or Authorization header).
- **Assert:** Status 200 and response JSON has `nanobotIntegration == true`.
- **Fail:** If status ≠ 200 or `nanobotIntegration` missing/false.

### Step 2.2 – List projects

- **API:** `GET {base_url}/api/projectsv2`
- **Assert:** Status 200 and at least one project.
- **Use:** We take the first project’s ID for the next steps.
- **Fail:** No projects → “create a project and agent first”.

### Step 2.3 – List agents

- **API:** `GET {base_url}/api/projectsv2/{project_id}/agents`
- **Assert:** Status 200 and at least one agent.
- **Use:** We take the first agent’s ID to build the MCP client.
- **Fail:** No agents or empty agent ID.

### Step 2.4 – MCP client and base URL

- **MCP base URL:** `{base_url}/mcp-connect/ms1{agent_id}`  
  Example: `http://localhost:8080/mcp-connect/ms1nba1r8q9v`
- **Auth:** Same Cookie/Authorization as above; also used for all MCP calls.

### Step 2.5 – MCP initialize (session)

- **API:** `POST {mcp_base_url}` (no path; query/body define method).
- **Body:** JSON-RPC 2.0, method `initialize`, params:  
  `protocolVersion: "2024-11-05"`, `capabilities: { elicitation: {} }`, `clientInfo: { name: "obot-eval-py", version: "0.0.1" }`.
- **Headers:** `Content-Type: application/json`, `Accept: application/json`.
- **Response:** We read `Mcp-Session-Id` from response headers and use it for all following MCP calls.
- **Assert:** Status 200 and non-empty session ID.
- **Fail:** Missing or empty `Mcp-Session-Id`.

### Step 2.6 – MCP notifications/initialized

- **API:** `POST {mcp_base_url}` with same JSON-RPC style.
- **Method:** `notifications/initialized`, params `{}`.
- **Headers:** `Mcp-Session-Id: {session_id}`.
- **Assert:** Status 200 or 202.

### Step 2.7 – Send chat prompt (phase 1)

- **API:** `POST {mcp_base_url}?method=tools/call&toolcallname=chat-with-nanobot`
- **Body:** JSON-RPC 2.0, method `tools/call`, params:
  - `name`: `"chat-with-nanobot"`
  - `arguments`: `{ "prompt": "<Step 1 prompt above>", "attachments": [] }`
  - `_meta`: `{ "ai.nanobot.async": true, "progressToken": "<uuid>" }`
- **Headers:** `Mcp-Session-Id: {session_id}`, `Content-Type: application/json`, `Accept: application/json`.
- **Assert:** Status 200.
- **Note:** This starts the async nanobot run; the actual reply is not in this response body. We get the reply from the event stream next.

### Step 2.8 – Read reply from event stream (SSE)

- **API:** `GET {mcp_base_url}/api/events/{session_id}`
  - Example: `http://localhost:8080/mcp-connect/ms1nba1r8q9v/api/events/{session_id}`
- **Headers:** `Accept: text/event-stream`, `Mcp-Session-Id: {session_id}`, same auth (Cookie/Authorization).
- **Behavior:**
  - Request is sent with `stream=True` (no full-body read).
  - We read the stream **line-by-line** (SSE). We do **not** use `resp.content` or `resp.text` (they would block/timeout on a long-lived stream).
  - **SSE parsing:**
    - Lines `event: XXX` – we flush any previously accumulated event data (so we don’t lose data when the server doesn’t send a blank line before the next `event:`), then set current event type.
    - Lines `data: {...}` – we append the JSON string to the current event’s data list.
    - Blank line – we flush the current event (parse all its `data` lines), then if event type is `chat-done` we stop. Otherwise we reset and continue.
  - From each flushed `data` we parse JSON and collect **assistant text**: objects with `role == "assistant"` and `items[].type == "text"` → we concatenate `items[].text`.
  - We **stop** when:
    - We see a complete `chat-done` event (after flushing), or
    - A safety timeout (e.g. 120s) is reached.
  - We then close our side of the connection (we do not wait for the server to close).
- **Return:** Concatenated assistant text (used as the “response” for the test).
- **Logging:** We log the GET when we get 200 (so the call appears in the API log even while the stream is open), and we log again when we finish with a short summary of the collected response (e.g. byte count or truncated text).

### Step 2.9 – Assert success

- We assert that we got a 200 on the event-stream GET and that we received some response text (length > 0).
- Test passes with a message like: `sent phase 0 prompt, status 200, events reply N bytes`.

---

## 3. APIs and URLs summary

| Step        | Method | URL / path |
|------------|--------|------------|
| Version    | GET    | `{base_url}/api/version` |
| Projects   | GET    | `{base_url}/api/projectsv2` |
| Agents     | GET    | `{base_url}/api/projectsv2/{project_id}/agents` |
| MCP init   | POST   | `{base_url}/mcp-connect/ms1{agent_id}` (JSON-RPC `initialize`) |
| MCP notif  | POST   | Same URL (JSON-RPC `notifications/initialized`) |
| Chat send  | POST   | Same URL + `?method=tools/call&toolcallname=chat-with-nanobot` (JSON-RPC `tools/call`) |
| Event stream | GET  | `{base_url}/mcp-connect/ms1{agent_id}/api/events/{session_id}` |

---

## 4. Event stream details (for dev)

- **Content-Type:** `text/event-stream`.
- **Events we care about:** `history-start`, `history-end`, `chat-done`.
- **Data format:** Each `data:` line is JSON. We only use objects with `role == "assistant"` and extract `items[].text` for `type == "text"`.
- **Important:** The server may send many `data:` lines and then `event: history-end` (or `event: chat-done`) **without** a blank line before the next `event:`. So we **flush and process the current event when we see a new `event:` line** so we don’t lose the last chunk of assistant content.
- We **never** read the stream with `resp.content`/`resp.text`; we only use `resp.iter_lines()` and parse incrementally.

---

## 5. Auth

- All requests use the same auth: either `Cookie: obot_access_token=...` or `Authorization: ...`, as provided to the test (e.g. via `OBOT_EVAL_AUTH_HEADER`).
- MCP requests also send `Mcp-Session-Id` after we get it from `initialize`.

---

## 6. What we log (when OBOT_EVAL_API_LOG is set)

Every request/response is logged: method, URL, status, and body (or a short summary for the event-stream response). So the dev can see the exact sequence: version → projectsv2 → agents → MCP initialize → notifications/initialized → tools/call (chat) → GET event stream (and then the event-stream response summary).

---

*Generated from the Python step eval implementation (eval_py/eval/cases.py, mcp_client.py, client.py, workflow_prompt.py).*
