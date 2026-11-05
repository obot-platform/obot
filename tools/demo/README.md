# Supervisor POC demo

The runner exercises the **live MCP gateway**, the **local-agent audit ingest endpoint** and the
**Copilot Studio adapter** on the existing local Obot instance. All patient records, contact
details and attachments are invented. The tool fixture never sends an email, delivers an HTTP
report, writes a care plan or executes a command.

## Before presenting

Use the existing Go server on **http://localhost:8090** and keep its separate Vite UI on
**:5174** running. See `HANDOFF-obot2-provenance.md` for server startup. From the repo root:

```sh
python3 tools/demo/demo.py seed --refresh-findings
```

This creates **37 fresh sessions** in the default 24-hour dashboard window: 36 background
workflows and one pending external-sharing review. The background workflows include ordinary
internal care work and reviewed denials, multiple agent identities, and synthetic Copilot
clinician/coordinator identities. MCP and device reports stitch into one session. The Copilot
adapter contributes separate sessions. Re-running seed adds another batch and preserves all
existing records. Use `--count 12` for a smaller batch.

Setup is automatic and repeatable:

- Checks the live Obot health endpoint.
- Keeps one pre-demo SQLite backup per day in the ignored `tools/demo/.local/` directory.
- Builds the Go fixture from this repository and starts it on **127.0.0.1:9124**. It leaves the
  earlier Acme fixture on :9123 and the Go/Vite servers alone.
- Creates or reuses the **Northstar Health** catalog entry and vMCP profile.
- Pins the fixture's capability labels and creates a policy scoped to that demo server.
  It does not change organization destinations, built-in policies, provider settings or AI
  content-tagger settings.

Requirements: Python 3 (standard library only), Go with the repository's dependencies, a
local Obot instance in dev mode, and localhost MCP access enabled (the existing :8090 setup
already has this). This runner intentionally targets local development instances only.

Open **http://localhost:8090/supervision** after seeding. `--refresh-findings` refreshes the
Supervisor's cached findings once after seeding and may call its configured model. You can
also click **Refresh findings** in **What I noticed**. The core traffic and pass/fail outcomes
do not require a model call. If configured, the Supervisor asynchronously
pre-reviews new approval requests.

## Live scenario 1: legitimate care coordination

```sh
python3 tools/demo/demo.py safe --step
```

The command prints a replay link after reading an external referral attachment. Open that
link beside the terminal. Press Enter to let it read a synthetic patient record, then Enter
again to attempt an **internal care handoff**.

What to say and show:

1. “An external referral attachment enters the context. The session now carries `untrusted`.”
2. “A private EHR tool returns a fictional patient record. Its identifiers add `pii` and
   `confidential`. A device file-read report joins the same session.”
3. “The agent writes the summary to the internal care tool. Obot allows this useful work.”

The runner verifies the recorded allow decision and the two observed surfaces. It ends this
session when the workflow completes. Omit `--step` to run the whole scenario immediately.

## Live scenario 2: external patient sharing held for review

```sh
python3 tools/demo/demo.py protect --step
```

The beginning is the same. The task warrant permits internal care tools and excludes external
reporting. This time the final action attempts to copy the fictional patient
record into a report addressed to **intake.outside.example**. The public attachment contains
an unverified request for that destination. No real attack or outbound transfer occurs.

What to say and show:

1. “The three ingredients are now present: untrusted context, private patient data and a tool
   that sends externally.”
2. Press Enter. “The MCP gateway holds the action **before the synthetic send tool runs**.”
3. Open the printed **Replay** link. Inspect the `require_approval` decision, session labels,
   and PII lineage pointing back to the patient tool result.
4. Open the printed **Review** link or return to the dashboard. The request is waiting for a
   human. A configured Supervisor may add its recommendation asynchronously. Deny it in the
   Approvals page to demonstrate the complete review record.

The runner fails visibly if the gateway allows the external action or if the stored decision
has no lineage. Each run uses a fresh session and can be repeated without resets.

**Approval behavior:** approving records the human's decision. The current integration does
not automatically retry or release a blocked MCP call. Do not promise “approve and it resumes”
in this POC. Denying is the clearest live ending.

## What this POC demonstrates

| Observed behavior | Evidence |
| --- | --- |
| Session stitching | MCP calls and synthetic local-agent file-read reports share a replay |
| Labels | Tool capabilities establish trust/private source labels; deterministic scans identify synthetic contact details |
| Lineage | An external tool argument matches identifiers in an earlier private tool result |
| Enforcement | The actual MCP gateway holds the external tool call before forwarding it |
| Human review | The real approval API/UI records a review outcome |
| SaaS-agent coverage | Seed traffic exercises the real Copilot runtime-monitor handler |

The scripts drive a predetermined workflow, not an autonomous model. They simulate device
reports and Copilot requests through the actual ingest adapters, rather than controlling a
real laptop agent or connecting to a Microsoft tenant. The fixture's writes are simulated.
These distinctions keep the live demonstration repeatable while testing real Obot policies,
gateway enforcement, persistence and UI.

**PHI distinction:** the story uses fictional patient health information. The deterministic
scanner currently detects identifiers, not clinical meaning. Expect `pii`, `confidential` and
`untrusted` labels. A `health` label requires an explicitly configured classifier/tagger;
this runner does not turn the optional AI tagger on. The demo does not establish HIPAA
compliance or support for government-classified systems.

## Local files and recovery

Generated binaries, logs, resource IDs, last-seed session IDs and backups live in
`tools/demo/.local/` and are git-ignored. No credentials are needed by the fixture or saved by
the scripts. The existing `obot.db` is never replaced or edited directly: data changes use
Obot's APIs. SQLite's backup API reads a consistent snapshot before the first demo command
on each day.

A normal Go-server restart preserves the data. If the fixture dies, the next demo command
restarts it. `python3 tools/demo/demo.py setup` prepares only the fixture and demo resources,
without creating workflow sessions. The fixture is deliberately left running so the next
scenario starts immediately.

### Refreshing older presentation data

`python3 tools/demo/refresh_content.py` inventories outdated presentation wording without
changing anything. `--apply` requires the Go server on :8090/:8091 to be stopped, takes a
separate backup, and updates only the allowlisted local POC tables in one transaction.
Restart Go afterward so cached sessions and resource names reload. It preserves stable
resource/event/session IDs, fingerprint hashes and exact canary credentials. This helper is
for this local presentation dataset, not a production migration.

Validation: `python3 -m unittest discover -s tools/demo -p 'test_*.py'` checks the scrub's
bounded scope, identity preservation, nested JSON handling and idempotence. The live
`safe`/`protect` commands themselves assert the stored gateway decisions and stitched
surfaces; `protect` also checks that the denied action has sensitive-data lineage.

## Tool-label review: Meridian Operations

```sh
python3 tools/demo/labels.py
```

This prepares a separate MCP on localhost:9125 and registers **Meridian Operations** in the
catalog. Its seven tools cover account context, supplier submissions, external distribution,
account notes, irreversible workspace retirement, program execution and credential leases.
The tool names and descriptions let you compare the initial keyword-based labels with the
Supervisor's suggested additions and reasons.

Open the printed **Review** link. Select the recommendations you agree with and choose
**Approve selected**. Suggestions stay separate from effective labels until that action.
The command does not approve labels or modify the Northstar care scenarios. Like the care
fixture, these handlers return receipts only and perform no real writes, execution or
credential issuance. The fixture is left running for subsequent review.

## Populate explicitly denied activity

```sh
python3 tools/demo/blocked.py
```

This adds five sessions whose latest attempted operation receives **deny**, rather than
`require_approval`: two external patient-sharing attempts, restricted account-brief delivery,
retained-workspace erasure, and an action after the reviewer contains a payment-ledger
session. It uses the existing Northstar Health and Meridian Operations fixtures. The helper
creates only narrowly scoped rules matching these exact task titles, client identities,
servers and tools, so the two live walkthroughs keep their existing behavior. It does not
approve AI label proposals or add requests to the approval queue. One denied care session
and the contained ledger session remain open for investigation; the other three end.

## Keep both attention categories ready

```sh
python3 tools/demo/held.py
```

This leaves two genuine held care-sharing sessions pending and open for 24 hours, alongside
the existing unapproved tool-label suggestions. Re-running it reuses its own pending requests
instead of adding duplicates. Its policy is scoped to the two specific care tasks, Claude
Code actor and Northstar external-report tool. It does not change organization defaults,
approve label suggestions, or resolve the held requests.
