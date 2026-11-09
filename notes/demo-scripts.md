# Demo scripts

Step-by-step demos of what was built on this branch. I ran every step marked **Tested** against
the local k3d cluster on 2026-11-08. Steps marked **Not tested** could not be run from the
sandbox.

## Setup

- Obot: <http://localhost:8080>.
- Users, all with the password `Obot-dev-Passw0rd!`:

  | User | Role | Use it for |
  |---|---|---|
  | `owner@obot.local` | Owner | The admin side of every demo |
  | `dev@obot.local` | Basic | A regular user and their agent |
  | `craig@obot.ai` | Admin | Your Claude account. Claude environment sessions map to this user |

- LLM gateway: your OpenAI key. `llm` is `openai/gpt-5.4` and `llm-mini` is `openai/gpt-5-mini`.
  The Anthropic models aren't enabled yet.
- vMCPs:
  - **Engineering Tools**: Context7, Fetch, Sentry, Notion, DeepWiki, Prisma Postgres, Semgrep, Mux, Exa Search. The `default` profile grants admins; `dev` has a profile for the servers they requested.
  - **Data Tools**: Supabase.
  - **Research Tools**: DeepWiki only. It needs no third-party sign-in, which makes it the easiest vMCP to connect a live agent to.
- Slack: the local deployment posts to a stand-in webhook (`kubectl logs deploy/webhook-echo`).
  For a real Slack demo, set `OBOT_SERVER_MCP_SERVER_REQUEST_SLACK_WEBHOOK_URL` to a Slack
  incoming webhook. `notes/local-k8s-dev/values.yaml` sets it under `config:`.

### Connecting Claude Code to Obot's built-in servers

Add Obot's built-in user server to Claude Code. The first time you use it, Claude Code opens the
browser to sign in to Obot.

```bash
claude mcp add --transport http obot http://localhost:8080/mcp-connect/obot
# Administrators can also add the admin server:
claude mcp add --transport http obot-admin http://localhost:8080/mcp-connect/obot-admin
```

To give the user server to everyone, use the snippets on **Server Requests**:
- `managed-mcp.json`, deployed by MDM.
- The Claude managed settings allowlist entry.

---

## Demo 1: A user asks their agent for an MCP server (video 1)

**Tested.** Agent side: Claude Code CLI as `dev@obot.local`. Admin side: browser as owner.

1. As `dev`, in Claude Code connected to `obot`, ask:
   > I found Mux's MCP server for video (hosted at https://mcp.mux.com/mcp). Check whether Obot
   > already offers it to me. If not, request it through Obot. I want it to check video encoding
   > status from Claude.

   The agent calls `search_mcp_servers`, doesn't find Mux, and calls `request_mcp_server`. It
   reports the request ID and that it is pending.
2. Slack, or the stand-in webhook, gets a message with the server, the requester, how it runs,
   the reason, and a **Review request** button. Check it with
   `kubectl logs deploy/webhook-echo --tail=50`.
3. As owner, open the review link: `/mcp-server-requests/<id>`. It shows the requester, the
   reason, the source, and how the server runs.
   - If the server is already in the catalog, a note says approving reuses that entry.
4. Under **Decide**, choose **An existing vMCP** and pick Engineering Tools. Leave
   **Give the requester access…** checked, add a note, and click **Approve**.
   - The page shows "Added to the catalog and to Engineering Tools".
   - The vMCP gains a component, and dev gets a profile named "Requested by dev@obot.local"
     that grants only this server.
5. Back in dev's Claude Code, ask "Did my Mux request get approved? How do I connect?". The
   agent reports **approved** with the vMCP connect URL.

Variations, all **Tested**:
- **Approve into a new vMCP**: choose **A new vMCP** and enter a name. The new vMCP grants
  admins and the requester.
- **Deny with a reason**: dev's agent then reports the reason.
- **Request a server that is already in the catalog** (seen only by users who can see the
  catalog entry): the agent finds it with `search_mcp_servers`. The result says the user can't
  connect to it yet, and the agent requests it with `existingCatalogEntryID`.
- **Admin by agent**: in Claude Code connected to `obot-admin`, say "Review the pending MCP
  server requests and approve the Mux one into Engineering Tools."

**Not tested:** a real Slack workspace. Only the payload was checked.

## Demo 2: Obot AI (video 2)

**Tested**, with the real `openai/gpt-5.4`.

1. As owner, click **AI** at the top of the nav.
2. Click **Give me a briefing across cost, security, operations and adoption for the last 7
   days**. Expand **Used N tools**: it calls `llm_usage`, `ask_supervisor`,
   `list_claude_sessions` and `adoption_summary`.
3. If a request is pending, ask "Which MCP server requests are waiting for me?", then "Approve
   the X one into a new vMCP called Data Tools." It approves only because you asked; the system
   prompt requires that.
   - To create a pending request, have dev ask their agent for a server, as in demo 1.
4. Sign in as `dev`, open **AI**, and ask "What MCP servers can I use for databases and video?".
   It answers with dev's own access only.

Answers take 10 to 30 seconds. Cost shows $0 because no LLM gateway traffic has been recorded
yet. To fill it, send traffic through the LLM gateway (for example, Claude Code pointed at the
gateway).

### Demo 2b: Widgets in Obot AI's answers (video 2c)

**Tested**, with the real `openai/gpt-5.4`.

Obot AI can place live views from Obot into its answers. The model writes a small
` ```obot-widget ` block, and the page renders the same components the rest of Obot uses. They
load their data with the viewer's own permissions.

| Widget | Shows | Ask, for example |
|---|---|---|
| `approvals` | Held agent actions (the Supervisor's recommendation, Approve and Deny) and pending MCP server requests (Add to a vMCP, Approve, Deny) | "What needs my approval right now?" |
| `session` | A session's risk, labels and counters, and its live event stream, with the Supervisor's lines for held or blocked steps | "Show me dev@obot.local's most recent session and what it did." |
| `sessions` | A list of sessions | "Which sessions are riskiest this week?" |
| `mcp_server_request` | One request; administrators can decide it in place | "Show me the Figma request." |

To have something pending:
1. As `dev`, request a server: `~/ui-check/api.sh dev POST /mcp-server-requests '{"name":"Figma","runtime":"remote","remoteURL":"https://mcp.figma.com/mcp","reason":"…"}'`.
2. Enable the **Hold DeepWiki repository reads** policy (Policies). It's off by default, because
   it holds the DeepWiki reads demo 4 uses.
3. Have dev's agent call DeepWiki's `read_wiki_contents` through Research Tools.

Then, as owner, ask "What needs my approval right now?". You click the buttons. The model never
approves from a widget, and approving through its tools still needs you to ask in words.

## Demo 3: Claude Environments on your cluster (video 3)

**Tested**, with the "Craigs Demo" environment.

1. From a checkout of a GitHub repository the Claude GitHub app can access, run:
   ```bash
   claude --environment ccpool_01BP7K8QxptZ3W1mKrjCzChe "Explain pkg/mcpserverrequest in three sentences. Don't change files."
   ```
   It prints the claude.ai session URL.
2. `kubectl get pods -n obot-agents` shows the worker pod.
3. In Obot, open **Environments** and the **Sessions** tab. The session is mapped to
   `craig@obot.ai`, which Obot matched by email, and shows Running.
4. **Audit Logs → Environments** shows every request from the sandbox: model calls, the git
   clone through Anthropic's git proxy, GitHub MCP and runner status.
5. On the **Dashboard** (Supervisor), the session shows on the egress surface. Ask the
   Supervisor "What are the Claude environment sessions doing right now?".
6. After 5 idle minutes the session goes to **Suspended**, and a workspace snapshot is written
   to the S3 bucket (SeaweedFS in-cluster, bucket `claude-snapshots`).

**Not tested:** resuming a suspended session. Reply in the session on claude.ai and watch it go
Running again in Obot. Attaching from the CLI (`claude --cloud <id>`) isn't enabled for your
account.

## Demo 4: AI content tagger (video 4)

**Tested**, with the real `openai/gpt-5.4` doing the classification.

1. As owner, open **Labels** and scroll to **AI content tagger**.
2. Check **Tag content with AI**.
3. Choose **In batches on an interval** and set the sample rate (100%) and the batch interval
   (15 minutes).
4. Uncheck **Only content the built-in scan found no sensitive data in**, then click **Save**.
5. Have an agent use Research Tools through Obot. For example, Claude Code connected to the
   Research Tools vMCP, asking "Using DeepWiki, show me the Go code in obot-platform/obot that
   encrypts stored credentials".
6. Reload Labels. The panel shows items waiting for the next batch. Click **Tag the queue now**:
   it shows "Tagging the queue…", then "Last batch … tagged N items".
7. Open the session from **Sessions**. It now has `source-code`, added by the tagger, alongside
   `confidential` and `untrusted` from the built-in scan.

The real-time modes behave the same way, without the queue. The tagger only adds labels the
session doesn't already have, so running the same content again adds nothing.

---

## Local environment notes

`notes/local-k8s-dev/` holds the scripts that run this environment in a sandbox without direct
internet access:
- `cluster.sh`: the k3d cluster with the certificate API feature gates and relaxed disk eviction.
- `egress-up.sh`:
  - redirects pod traffic through the transparent egress forwarder (`k8s-egress.go.txt`)
  - mounts the sandbox's MITM CA into pods
  - adds a CoreDNS override for `*.svc`
  - backs `/var/lib/ate` with ext4, because Substrate's overlayfs can't use the node's
    overlay root as an upperdir
- `deploy.sh`: builds and imports a dev image and runs `helm upgrade`.

On a normal machine with internet access, only `cluster.sh`, the `/var/lib/ate` part of
`egress-up.sh`, `s3.yaml` and `deploy.sh` are needed.
