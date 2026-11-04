# AuthZEN receiver playground

A standalone, standard-library Python receiver and browser UI for exercising the local Obot authorization-filter implementation. No production Obot code depends on it.

The running preview uses:

- UI: http://localhost:8090
- Obot UI: http://localhost:5174/mcp-servers?view=filters
- Authorization callback: `https://172.17.0.1:9443/authorize`
- Content callback: `http://172.17.0.1:8090/filter`
- Official [Everything MCP server](https://github.com/modelcontextprotocol/servers/tree/main/src/everything): `http://172.17.0.1:3001/mcp`

The bridge IP is specific to this Docker sandbox. Both callbacks use one Python process. The content callback accepts messages unchanged with HTTP 200; it does not receive or infer identity. Obot's existing managed webhook converter invokes it. Authorization consumes the AuthZEN envelope and returns HTTP 200 with `{"decision":true}` or `{"decision":false}`. Obot translates ordinary denial to HTTP 403 and explicit quota denial to HTTP 429.

## Try it

1. Send an `echo` request with the default allow policy.
2. Add `echo` to **Blocked actions**, save, and send again. Expect HTTP 403.
3. Clear blocked actions and add a **Rate limits** row for user ID `2`, with 2 calls per 60 seconds, save, and send three requests. The third returns HTTP 429 with a retry time. Reset quotas to retry immediately.
4. Expand a receiver activity entry to inspect the actual Obot envelope. Block its subject ID to deny that identity.
5. Use **Load tools** to discover Everything's tools, or try `tools/list`, `resources/list`, and `prompts/list`. The attached filter matches all methods; quota counting applies only to `tools/call`.

The “Try through Obot” API uses the fixed connection configured on disk and calls the actual Obot gateway. It does not accept an identity override. The caller forwards only the browser’s `obot_access_token` cookie to the fixed local gateway. Sign in to Obot on `localhost` in the same browser first. No credentials are stored by the receiver. The preview now uses local authentication; Bill’s verified user ID is `2`. Multi-identity quota isolation is also covered by unit tests.

The default policy is an explicit playground configuration, not a built-in Obot policy. Quotas count allowed authorization evaluations per subject type, subject ID, server ID, and action, including evaluations whose subsequent upstream request fails. This is a single-process fixed-window demonstration, without distributed accounting, replay protection, or production policy storage.

## Run the receiver

Python 3.10+ is sufficient. Use a server certificate whose SAN includes the address Obot will call, issued by a CA trusted by the Obot backend. Never disable TLS verification. For this sandbox, certificates and state are in `/tmp/obot-authzen-demo`; the preview backend trusts the demo CA through `/data/demo-ca-bundle.crt` and `SSL_CERT_FILE`. The demo certificates expire after seven days.

```sh
python3 tools/authzen-demo/server.py \
  --cert /tmp/obot-authzen-demo/server.crt \
  --key /tmp/obot-authzen-demo/server.key
```

The UI and callback listeners bind all interfaces for access from the desktop and Docker network. The UI has no authentication; run only in the local sandbox. Only `/authorize` and `/filter` are exposed on the TLS listener. The administrative API is JSON-only with a same-origin browser check. These are development conveniences, not production access controls.

Policy is stored at `/tmp/obot-authzen-demo/policy.json`. Saving changed rate limits resets quotas; saving authorization policy does not. Restart preserves policy but clears quotas and the last 100 receiver events. Obot's audit log remains the system of record and includes authorization decision, filter ID, and evaluation ID; the activity table is receiver diagnostics, not an alternative audit store.

Create `/tmp/obot-authzen-demo/connection.json` with:

```json
{"serverID":"<Everything catalog entry ID>","filterID":"<filter ID>"}
```

The server ID may be an Obot connectable catalog-entry ID or deployed server ID. Obot's API is expected at `http://127.0.0.1:8080` with local authentication enabled. This same-host browser-session runner is for the local playground; external clients should use Obot’s supported MCP authentication flow.

## Everything and filter setup

Start the pinned official server separately:

```sh
npm install --prefix /tmp/obot-everything @modelcontextprotocol/server-everything@2026.8.31
node /tmp/obot-everything/node_modules/@modelcontextprotocol/server-everything/dist/index.js streamableHttp
```

In Obot, add a remote server catalog entry using `http://172.17.0.1:3001/mcp`. Add one custom webhook filter scoped to that entry, using the content callback above. Enable its AuthZEN authorization callback using the HTTPS endpoint above. Leave signing secret blank for unsigned mode and launch the filter. Leave method selectors empty to cover all requests supported by the authorization stage. Private-IP endpoints must be explicitly permitted in this local preview (`--disallow-private-ipmcp=false`).

Authorization signing is optional. To enable verification, set `AUTHZEN_DEMO_SIGNING_SECRET` on the receiver before starting it and set the same authorization signing secret in Obot. The receiver validates `X-Obot-Signature-256` using HMAC-SHA256 over the exact raw request body and rejects invalid or missing signatures. Do not put secrets in the source tree. The demo content endpoint intentionally remains unsigned; it is independent of authorization signing.

Rate limits have their own editable table and save action. Add/remove rows for user or agent IDs, with independent call limits and windows. `*` matches all IDs/types; exact identity IDs take precedence, then exact types. A zero limit explicitly grants unlimited calls for that matching row. Duplicate identity/type rows are rejected.

Quota denial messages in receiver activity name the identity, tool, quota, window, and retry time. The receiver emits the optional `context.obot.reason: rate_limited` and `retry_after_seconds` decision fields. Obot responds with HTTP 429, Retry-After, and a clean JSON-RPC message plus structured retry data. Ordinary policy denial remains HTTP 403.

## Validation

```sh
python3 -m unittest discover -s tools/authzen-demo -v
```

Tests cover allow/deny policy, subject/action blocks, quota isolation and expiration, discovery exclusion, policy persistence/validation, and signature tampering. The local preview was also exercised through the headed browser and real Obot gateway: discovery, successful echo, explicit denial, quota denial, content callbacks, and authorization entries in Obot's audit logs.

## Request mapping

The receiver consumes `obot-mcp-requests-v2`. `action.name` is always the MCP
method, and `action.properties.tool` selects the tool for `tools/call`. Quotas
key on verified subject type/ID, deployed server ID, and tool. Blocked actions
accept a method (blocking all calls of that method) or a tool name. Activity
shows both method and selector. The resource is always `mcp-server`; optional
`context.obot.catalog_id` and `catalog_entry_id` identify its catalog association.
The old `context.obot.request` duplication is removed. This playground must be
updated alongside the gateway when switching from the experimental v1 mapping.
