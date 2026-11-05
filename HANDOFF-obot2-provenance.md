# Handoff: Obot 2.0 "Provenance" (branch `obot-2-provenance`)

This branch adds a cross-surface **session graph**, a single **Obot Policy** engine, and an
**AI Supervisor** that watches every agent session, decides gray-area actions, explains itself,
and recommends what to do. It is a large, self-contained feature set in `pkg/provenance` plus
surface adapters, a store, a REST API, and a "Supervisor" admin UI.

Everything below is committed on `obot-2-provenance` (5 commits off `main`). The working tree is
clean except an untracked `keys` file (OpenAI/Anthropic keys, git-ignored, do not commit).

## Start here (read in this order)

1. `obot-moonshot.md` (repo root) — the vision/narrative.
2. `adr/2026-10-28-provenance-session-graph-and-obot-policy.md` — the architecture decision record. **This is the single best overview of how it fits together.**
3. `docs/docs/supervision/{overview,policies,supervisor,surfaces}.md` — user-facing docs.
4. `pkg/provenance/doc.go` then `engine.go`, and `engine_test.go` (the tests walk the full "lethal trifecta" demo end to end).

## Commits

```
af4a15d45 feat(ui): keep the Supervisor's briefing and findings for 30 minutes
865889e39 fix(ui): reload the session and policy lists when their filters change
87f0321fa feat(provenance): optional AI content tagger
9f3628c94 feat(provenance): put the AI Supervisor at the center of the experience
61fad7c5e feat(provenance): Obot 2.0 session graph, Obot Policy and the AI Supervisor
```

## What it is

Obot observes an agent in five places and feeds them all into one engine:

| Surface | Adapter / hook point | Budget |
|---|---|---|
| `llm` | LLM gateway plugin (`pkg/guardrails/llmprovenance.go`) — ingests tool results, decides proposed tool calls | 10s |
| `mcp` | Always-on native filter in the MCP hook chain (`pkg/api/handlers/mcpgateway/provenance.go`) | 10s |
| `device` | Sentry decision endpoint, after the fleet allowlist (`pkg/api/handlers/enforcement.go`) | 10s |
| `egress` | Claude Environments Envoy ext_proc (`pkg/claudecredentials/audit_provenance.go`) | 3s |
| `copilot` | Microsoft Copilot Studio external threat-detection provider (`pkg/api/handlers/copilotstudio.go`) | <1s |

Each adapter builds a normalized `provenance.Action` and calls `Engine.Decide` / `Engine.Ingest`.
The engine stitches events into one **session**, carries the session's **taint** (labels of
everything it read), tracks **fingerprint lineage**, and evaluates **Obot Policy** (CEL) with
effects `allow / log / redact / downshift / quarantine / judge / require_approval / deny` in
`shadow` or `enforce` mode.

## Code map

- **Core (pure, I/O-free):** `pkg/provenance/{doc,event,labels,fingerprint,session,policy,engine,store,simulate,memstore,content_classifier}.go`
  - Data tagging: `fingerprint.go` `Classify` (deterministic regex + Luhn) and `labels.go` `ToolCapabilityHeuristics` (keyword tool labels). The engine inherits content trust/sensitivity from the producing tool's capability labels in `Ingest`.
- **Supervisor (AI):** `pkg/provenance/supervisor/{supervisor,assistant,gateway_decider,openai_decisions,draft,content}.go` — bounded-decision primitive (`Decider`) over the LLM gateway or an OpenAI Decisions-shaped endpoint. Implements `Judge`, `TriageApproval`, `DraftPolicy`, `Narrate`, `ClassifyTool`, `ClassifyContent`, plus `Brief` and `Ask` (for the home-page briefing and the "Ask the Supervisor" box).
- **Red team:** `pkg/provenance/redteam/` — runs the 10 assurance scenarios against the engine.
- **Surface helpers:** `pkg/provenance/{llm,device,copilot}/` (pure, tested).
- **Store:** `pkg/gateway/types/provenance.go` (9 GORM tables), `pkg/gateway/client/provenance.go` (implements `provenance.Store`, plus `RunProvenanceRetention`). Registered in `pkg/gateway/db/db.go` AutoMigrate.
- **API:** `pkg/api/handlers/provenance.go` (sessions, policies, approvals, kills, honeytokens, tool-labels, settings, overview, narrative, simulate, draft), `provenance_supervisor.go` (`/supervisor/status`, `/supervisor/insights`, `/supervisor/ask`), `provenance_redteam.go`, `copilotstudio.go`. Routes in `pkg/api/router/router.go`, authz in `pkg/api/authz/authz.go`.
- **Wiring:** `pkg/services/config.go` ~line 965-976 — constructs the engine over the GORM store, seeds built-ins, sets the Supervisor and content classifier, schedules retention. The LLM gateway gets it via `gatewayServer.SetProvenance`.
- **UI:** `ui/user/src/routes/{supervision,sessions,sessions/[id],policies,policies/[id],approvals}/`, components in `ui/user/src/lib/components/supervision/`, service `ui/user/src/lib/services/provenance.ts`, i18n `ui/user/messages/supervision/{en,ja,ko,zh-CN}.json`, nav badge in `Layout.svelte`.

## How to run it locally

The repo has a running instance pattern already used this session (keys in `./keys`,
format `OPENAI_API_KEY=...` / `ANTHROPIC_API_KEY=...`):

```bash
# from repo root, with ./keys present
set -a && . ./keys && set +a
OBOT_SERVER_HTTP_LISTEN_PORT=8090 OBOT_SERVER_URL=http://localhost:8090 \
  OBOT_SERVER_DISALLOW_LOCALHOST_MCP=false go run main.go server --dev-mode
# UI dev server (Vite) on :5174, proxied behind :8090. UI is at http://localhost:8090/supervision
```

**Gotchas (these cost real time this session):**

- **The Go server does NOT hot-reload** — only the SvelteKit UI does. After any Go change you must restart the server process. (The MCP session-stitching and budget fixes were invisible until a restart.)
- The server binds an **internal port 8091** in addition to 8090; after killing it, wait for 8091 to free before restarting.
- Providers must be configured and models synced before the Supervisor works. This session registered OpenAI + Anthropic via `POST /api/llm-gateway/admin/providers` + `/keys`, then `POST /api/llm-gateway/sync-models`, then set the `llm` / `llm-mini` default model aliases (`PUT /api/default-model-aliases/{llm,llm-mini}`) to real model IDs. Env keys (`OPENAI_API_KEY`/`ANTHROPIC_API_KEY`) also auto-import providers on startup — exporting `./keys` before launch is the reliable path.
- **`make test` is shadowed by the repo's `test/` directory** — use `go test ./...` directly.

## Build / test / lint

```bash
go build ./... && go test ./...          # provenance pkgs + handlers all green
golangci-lint run ./pkg/provenance/... ./pkg/api/...   # repo config adds decorder/grouper/modernize
make generate                             # deepcopy + openapi; commit the generated diffs
cd ui/user && pnpm run ci                 # format + lint + check (0/0 expected)
cd ui/user && pnpm exec vitest run src/routes/supervision src/routes/sessions ...
```

**Known pre-existing failures (NOT from this branch — verified against a clean `main` worktree):**
`pkg/gateway/server` `TestLLMUsageRecorderRecordsEveryRequestOnce` and
`TestRecordingResponseRecordsUnmeteredResponses` (fail on main), and `pkg/clauderunner`
`TestOrchestratorProcessPortsAndShutdown` (passes in isolation; flakes under full-suite load).

## Demo harness (for a live walk-through)

In `/private/tmp/.../scratchpad/demo/` (session scratch — regenerate if gone):
- `mcpserver/acme-crm-demo` — streamable-HTTP MCP server on :9123 (`/mcp`), tools `search_public_docs`, `lookup_customer`, `list_customers`, `send_report`, `update_ticket`. `DEMO_CANARY=<v>` plants a honeytoken in `lookup_customer`.
- `device/obot-device-sim` — Sentry device simulator: `enroll -credential ode1-...`, `token`, `decide -json`, `audit -json`.
- `drive.py` — drives all five surfaces with real OpenAI/Anthropic calls, enables enforcement, exercises honeytoken/kill/redteam/narrative/draft. All content is synthetic/benign.

Run `python3 drive.py` against a configured server to repopulate the graph. Note the demo data
**ages out of the 24h default window** — see rough edges.

## The optional AI content tagger — where it landed

- **Built, committed (`87f0321fa`), wired (`config.go:976` `SetContentClassifier(sup)`), and live-capable.**
- **OFF by default** (`Settings.ContentTagging.Enabled = false`). Nothing sends content to a model right now.
- It is **additive**: the deterministic `Classify` always runs first and its labels are never removed; the tagger only adds fuzzy labels (e.g. "internal strategy" with no structured identifier) that regex can't catch.
- Gated: `enabled`, `sampleRate`, `minBytes` (200), `onlyUnlabeled` (true), `capabilities` (reads-private/reads-untrusted), `mode` (async default | sync), `budgetSeconds` (3). Cached by content hash. Runs through the governed LLM gateway (the Supervisor implements `ContentClassifier`).
- **To turn it on:** GET `/api/provenance/settings`, set `contentTagging.enabled=true`, PUT the whole object back. Requires a Supervisor configured (it is, via `SetContentClassifier`).
- **Caveat worth deciding on:** this is the one feature that sends tool-result *content* to a model. For a product whose pitch is "we don't ship your data around," keep it opt-in and ideally point it at a self-hosted model. Recommendation stands: **leave it off for the booth demo** unless pointed at a local model.
- **No UI toggle yet** — settings-only via PUT. A settings page is a natural next task.
- The live server's stored settings row predates the feature, so its `ContentTagging` is zero-valued rather than the code defaults — re-seed or PUT a full settings object to normalize.

## Known rough edges / next tasks

Correctness / polish:
1. **Supervisor model setting rejects provider-style names** (`anthropic/claude-sonnet-4-6` fails on the `/`); only the internal model resource ID works. Fix the validation or accept both.
2. **Policy drafting fails with Anthropic models** (Sonnet) — Anthropic rejects the JSON schema ("additionalProperties must be explicitly set to false"); works with `gpt-5-mini`. Fix the schema emitted by the Supervisor's structured-output call.
3. **Users render as "user 3"** instead of names/emails in insights and some lists — resolve to a display name.
4. **Duplicate insights** — same title for different sessions appears twice; de-dupe or group.
5. **Killed session header still says "Open"** (the data sets `killed` but not `endedAt`) — treat killed as closed in the UI, or set `endedAt` on kill.
6. **Demo data ages out of the 24h default window.** Both the Supervisor home (24h/7d toggle) and the Sessions page default to 24h. Consider defaulting to 7d, and/or regenerate demo data on show day.
7. The home-page briefing falls back to a deterministic sentence on the first request after a server restart (model warm-up); next refresh uses the model. Briefing + insights are cached 30 min (`provenance_supervisor.go` `supervisorInsightsTTL`).

Architecture / follow-through:
8. **obot-sentry (separate repo) must be updated** to send `sessionID`, `toolUseID`, `input`, `cwd` on PreToolUse so device decisions carry session context. The exact JSON is documented in the ADR and `docs/docs/supervision/surfaces.md`.
9. **Copilot Studio auth is a Microsoft Entra token**, not an Obot API key — the `/api/copilot-studio/{runtime-monitor,validate}` routes need a proxy that validates Entra and injects an Obot key, or Entra validation on the route.
10. **Egress ext_proc runs in `STREAMED` mode** in the Helm chart; switching to `BUFFERED_PARTIAL` would let body-based denials land before any bytes leave the sandbox (chart change, no Go change).
11. **LLM-surface tool-result dedupe is positional** (re-sent trailing results can re-ingest); tool capability labels there are keyed by `agent+tool` since the gateway can't see the serving MCP server.
12. **The ~9 legacy policy types still exist** alongside Obot Policy and are not yet folded into it (the ADR calls this out as follow-up).
13. Session state is per-replica write-through, last-writer-wins; fine in practice (taint only grows) but note it for multi-replica correctness.
14. **Decision budgets** were raised to 10s (LLM/MCP/device) because `gpt-5-mini` takes ~6s; egress 3s, Copilot <1s. If you change the default decision model, re-check the budgets (constants in `llmprovenance.go`, `mcpgateway/provenance.go`, `enforcement.go`, `audit_provenance.go`).

## The pitch deck

A 14-slide booth deck (7-slide pitch + 7-slide "Under the hood" appendix) is published as a
claude.ai Artifact: **https://claude.ai/artifact/RJo3yTBCPnzi5ELuCxTvJe** (private — share from the
Share menu). Source files are in the scratchpad `deck/` folder. Dark theme (`#0D1117` bg, white
text, `#4C8DFF` blue accent). Every number on the technical slides is drawn from the code.

## Not done / explicitly out of scope

- The feature is **uncommitted-to-upstream** — this is a feature branch off `main`, nothing is pushed or PR'd. Per repo CLAUDE.md, push to the author's fork and open the PR with `--repo obot-platform/obot --head <user>:<branch>`, no closing keywords.
- No migration of existing/old data; this was built as "Obot 2.0" without preserving legacy UX that didn't fit.
- `make generate` outputs (deepcopy/openapi) are committed; re-run and re-commit if you touch `apiclient/types` or the storage v1 types.

## Running the live instance on :8090 (current state, for takeover)

There is a working instance up right now, populated with the demo data. It is **two separate
processes**, launched by hand (not a single `make dev`):

1. **Go API server on :8090** — a standalone `go run main.go server --dev-mode` with the env
   below. It also binds an internal **:8091**. It reads `./obot.db` (holds the demo data) and
   `./keys`. This is the process you relaunch after Go changes (the Go server does **not**
   hot-reload).
2. **SvelteKit UI dev server (Vite) on :5174** — a `pnpm run dev --port 5174` started earlier by
   `make dev`. The :8090 server **proxies the UI to it**, so it must stay up. The app is served at
   **http://localhost:8090** — do not open :5174 directly.

Why :8090 and not the default :8080? :8080 had a stale Docker container earlier; we moved the
server to :8090 and the user wants to keep it there.

### Relaunch the Go server

Use the committed-to-worktree helper (manages only the Go server, waits for :8091 to free, waits
for health):

```bash
./run-8090.sh          # logs to /tmp/obot-8090.log
```

Or inline, equivalent to how it was launched:

```bash
# stop the old one and wait for the internal port to free
for p in $(lsof -nP -tiTCP:8090 -sTCP:LISTEN); do kill "$p"; done
for _ in $(seq 1 30); do lsof -nP -iTCP:8091 -sTCP:LISTEN >/dev/null 2>&1 || break; sleep 1; done
# relaunch
set -a && . ./keys && set +a
OBOT_SERVER_HTTP_LISTEN_PORT=8090 OBOT_SERVER_URL=http://localhost:8090 \
  OBOT_SERVER_DISALLOW_LOCALHOST_MCP=false \
  nohup go run main.go server --dev-mode > /tmp/obot-8090.log 2>&1 &
# wait for it
until curl -sf localhost:8090/api/healthz >/dev/null; do sleep 2; done
```

### If the UI dev server (:5174) dies

The app will 502 through :8090 without it. Restart it (needs Node ^24 || >=26 on PATH — the
repo uses `mise`/`make dev` for this):

```bash
cd ui/user && pnpm run dev --port 5174
# e.g. PATH="$(mise where node@24)/bin:$PATH" pnpm run dev --port 5174  if the default Node is older
```

Do not kill this when restarting the Go server — `./run-8090.sh` only touches :8090/:8091.

### Don't lose the demo data

`./obot.db` holds all 53 demo sessions. Restarting the Go server keeps it (same file). Deleting
`obot.db` or running a fresh `make dev` on a different DB loses the demo graph — repopulate with
the demo harness (`scratchpad/demo/drive.py`) if that happens.

## Takeover updates (2026-10-30, worktree changes)

- Administrators and auditors now land on `/supervision`; Dashboard, Sessions, Approvals and
  Policies are the first navigation items. The previous infrastructure dashboard remains at
  `/dashboard` under Operations. Power-user/basic-user destinations and explicit redirects are
  preserved.
- The home page has one findings area, **What I noticed**, with **Refresh findings**. The
  redundant briefing card was removed. Needs your attention is a short review queue linking to
  the full approval evidence/actions. Expired approvals no longer count as pending.
- Saved MCP entries and vMCPs link to **Review tool labels**. AI recommendations show current
  and proposed capability labels with reasons. Administrators explicitly select and approve;
  auditors can view. This also works with Git-managed resources: approval writes a resource-ID
  scoped provenance override, never changes the source manifest/repository. Suggestions use
  saved tool previews, and approval rejects an outdated inventory. Generate tool previews first;
  a vMCP needs previews that match its pinned component definition.
- Durable POC tooling is now in `tools/demo/README.md`. Run `python3 tools/demo/demo.py safe
  --step` for internal care coordination, or `python3 tools/demo/demo.py protect --step` for
  external patient sharing held for human review. These scripts exercise real Obot gateways
  with invented patient records and a fixture that performs no actual outbound transfer.
  `seed --refresh-findings` adds current activity and refreshes the findings cache. Stored
  sample prose uses realistic Northstar Health names. Local backups/state are ignored under
  `tools/demo/.local/`; preserve them along with `obot.db`.
- `obot-supervisor.html` is the editable local deck (16 slides). Slide 11 illustrates PHI
  stitching/labels/taint/lineage; slide 15 marks third-party adapters as planned. The original
  Claude artifact link is not updated by editing this file. Follow-up implementation notes:
  `ROADMAP-obot2-provenance.md`.
- Use native CDP for browser inspection per the user's preference. Do not invoke browser-harness.

When launching from an agent execution tool, keep `go run main.go server --dev-mode`
in a persistent foreground exec session with the same exported environment and logs redirected
to `/tmp/obot-8090.log`. In this tool environment, a `nohup` child can be reaped when its
launching command completes even after the script reports healthy. The shell helper remains
suitable for an ordinary persistent terminal.

## POC iteration updates

- `obot-supervisor-executive.html` is a separate seven-slide pitch deck. The first five slides
  explain the product and PHI story; the final two cover the session record and decision path.
  The earlier POC closing slide was removed. The sixteen-slide technical deck is separate.
- The dashboard attention queue combines held actions and label reviews. Label suggestions are
  grouped by resource rather than creating a row per tool. Each group opens its exact review.
- `/tool-labels` is the central Labels view (Resources, Label library, Observed tools). It shows
  effective assignments, provenance, pending/outdated suggestions, and AI tagging status.
- `python3 tools/demo/labels.py` prepares Meridian Operations and pending recommendations.
  `python3 tools/demo/blocked.py` creates five genuinely denied activity stories.
  `python3 tools/demo/held.py` leaves two genuine held sessions pending for 24 hours and reuses
  them on rerun. Do not resolve those held reviews while preparing the presentation.
- After explicit user batch confirmation, removed the 77 audited unused conversion profiles.
  Eight vMCPs remain; connections, catalog entries, credentials, and activity history were preserved.
  Backup: `tools/demo/.local/obot-before-vmcp-cleanup-20261030-105859.db`.
- Ask the Supervisor occupies a dedicated right column at the top of the dashboard (first on mobile).
  Enabled shadow policies recommended by insights or saved as AI drafts appear in Needs your
  attention with Review policy links; enforced/disabled policies are excluded using fresh policy state.
- User preference: iterate quickly for this POC. Use focused checks and live smoke checks;
  avoid broad regression suites unless a change actually warrants them. Use native CDP.

- Policy drafting derives input field types from the Go decoder and retries malformed JSON
  once with correction feedback. Test inputs use JSON strings on the model wire to keep the
  structured-output grammar small, then decode into the typed Go input; object replies from
  text-only models remain supported. Transport failures are not retried. Drafts still start
  in shadow and require explicit saving/enforcement.
