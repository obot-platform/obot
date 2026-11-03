# 2026-10-11: Add authorization callbacks to custom MCP filters

- **Status:** Proposed — minimal HTTP slice implemented locally; uncommitted
- **Date:** 2026-10-11
- **Supersedes:** None
- **Superseded by:** None

This draft was requested alongside the ODP and UX refinement. Per the ADR
lifecycle, it must be reconciled with the accepted ODP and actual implementation
before merging as an Accepted record in the implementation PR.

## Implemented first slice

The local implementation adds an optional HTTPS AuthZEN endpoint and separate
authorization signing secret to the existing custom Webhook URL filter. It reuses the manifest,
CRUD, attachment resolver, compiled selectors, gateway request processor and
audit event. No default policy, extra filter resource, parallel dispatch
pipeline, or client extension is introduced.

The authorization stage runs before content filters for matching client
JSON-RPC requests across methods. It requires an explicit boolean decision;
ordinary denials return 403, explicit rate-limit denials return 429, and evaluation failures return 503. Content request and
response contracts are unchanged. A content filter cannot change a method or
identifier after that target has been authorized. An unready content runtime
does not remove required authorization.

The initial envelope contains authenticated principal identity, gateway target,
method, typed action selectors, catalog association and the versioned
`obot-mcp-requests-v2` mapping. The current authentication path supplies user identities. Upstream removed hosted-agent authentication; agent identities and verified OBO assertions remain future work. The vMCP envelope currently identifies the selected connection (`vmcpi...`),
not the parent vMCP used for filter attachment and audit scope, and does not claim
component-level resolution. Notifications and server-originated
messages remain outside authorization. For vMCP tool search, the existing generic-call resolver supplies the underlying tool to both authorization and content hooks. Selectors for the call_tool wrapper continue to apply, and content mutations are checked before restoring the wrapper.

Credentials reuse encrypted storage but are excluded from the content runtime
and reveal API. Endpoint binding prevents a partial credential/manifest update
from using a replacement signing secret for the previous destination. Older-client
updates preserve omitted authorization settings; disabling is explicit. HTTP
transport reuses existing remote-MCP network restrictions, rejects redirects,
and bounds callbacks to five seconds and the stage to fifteen seconds.

HTTP 429 is classified as denied in MCP audit presentation and outcome queries,
including upstream rate limits. This is a status-based classification; the
authorization detail distinguishes a policy denial from an upstream response.
Existing rows are reclassified on read without migration.

Decision metadata extends existing webhook statuses on the protected request's
audit log. There is no new store or filter-scoped query in this slice.
MCP authorization tools, authorization-only configurations, disclosure controls,
environmental enrichment, and verified OBO delegation remain follow-ups. The
broader decisions below describe that proposed direction rather than capabilities
already implemented.

See the [implemented contract](../docs/docs/functionality/filters.md#authzen-authorization)
for payload and operational details.

### MCP operation and server identity mapping

Use the exact MCP method as `action.name` for every operation. Typed
`action.properties` carry tool/prompt names, resource URIs, or completion
reference and argument name. Keep `resource.type` as `mcp-server` and its ID as
the resolved deployed `MCPServer` ID for ordinary servers, distinct from a per-user
connection's `MCPServerInstanceID`. The experimental vMCP path instead uses its
`VMCPInstance` ID because that is the gateway's resolved connect target. Add optional authoritative `context.obot.catalog_id` and
`catalog_entry_id` from the existing server configuration, requiring no extra
lookup. Do not duplicate method/identifier under context.

This lets policies consistently distinguish discovery from invocation and target
one deployment or all deployments of a catalog entry. For vMCPs, the current
payload lacks the parent vMCP ID and catalog fields; attachments and audit scope
use the parent while the envelope uses the connection. Explicit parent/instance
enrichment requires a follow-up mapping decision before claiming consistent
parent-level policy targeting. No arguments
or completion values are disclosed. This changes the unshipped experimental
mapping from v1 to v2; update receivers together with the gateway, while preserving
existing content contracts and the boolean/optional rate-limit response contract.
See the method table in the implemented contract for exact properties.

### Authorization callback authentication

Use the existing `X-Obot-Signature-256` HMAC-SHA256 convention over the
serialized AuthZEN body, with an optional, separately stored authorization signing
secret. Bearer authentication is excluded from this slice. Content webhooks
retain their existing optional secret and unchanged payload/signature contract.
One filter may point its two endpoints at one service, enabling a combined
deployment without coupling the two callback contracts or credentials.

When configured, receivers verify the raw body before evaluating policy.
Without a signing secret, Obot sends unsigned HTTPS requests and the receiver
decides whether to accept them. Explicit authorization decisions and failure
handling are unchanged. Obot persists a server-managed signing marker separately
from the credential, so losing a configured secret cannot downgrade a signed
callback to unsigned. API clients cannot clear that marker by omitting it or
setting it to false. This retains the existing
body-only signature scheme; it does not add timestamp freshness or promise
replay prevention. Evaluation IDs support receiver-side deduplication for stateful
decisions. Any future freshness extension must be explicit and must preserve
existing content compatibility.

## Related issues

- Obot customer-demand tracking issue: TBD; no existing issue identified.
- [MCP ext-auth #14](https://github.com/modelcontextprotocol/ext-auth/issues/14) —
  gateway-enforced AuthZEN authorization proposal; alignment, not an adopted MCP
  extension requirement.
- [MCP ext-auth #15](https://github.com/modelcontextprotocol/ext-auth/issues/15) —
  contextual authorization work; optional future integration.

## Related ODPs

- AuthZEN authorization filters — local ODP at
  `obot-design-proposals/proposals/2026-10-09-authzen-authorization-hooks/README.md`;
  published ODP/PR link TBD.
- [Virtual MCPs](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md)
  — provides context for attachment inheritance, resolved target identity, and
  audit navigation; not superseded by this decision.

## Context

Customers need external policy decisions and per-principal rate limits for MCP
requests. Obot authenticates users and can assemble trusted identity
and request context. Existing content filters have a different message-processing
contract and must remain compatible.

Inspection of the running v0.26.3 image confirmed Filters already provides method
and identifier selectors, an MCP/vMCP/registry resource picker, and an Audit Logs
tab. vMCP audit links use the central audit view scoped by MCP ID. A separate
Authorization navigation area and decision-log store would duplicate these
concepts. However, the current filter audit tab scopes to the filter's backing
server, not to policy evaluations on protected requests.

## Proposed decision

1. Extend the existing custom filter with an optional authorization callback.
   Keep Type as Webhook URL, Remote, NPX, UVX, or Containerized. Webhook filters
   may configure their existing content endpoint and a separate authorization
   endpoint; MCP filters may configure their existing content tool and a separate
   authorization tool on the same server. A filter may implement either or both.
   An absent authorization block preserves legacy behavior; optional contentEnabled
   defaults true so the existing default content tool remains compatible. No
   authorization policy/filter is installed by default; only configured matching
   callbacks impose an additional decision.
2. Obot supplies an AuthZEN envelope as the authorization HTTP body or MCP tool
   arguments. The proposed MCP binding requires structuredContent with boolean
   decision and no isError; it is an Obot binding, not a standard AuthZEN MCP
   transport. Legacy webhook JSON-RPC bodies/signatures/status codes and MCP
   content-hook arguments/results remain unchanged. Authorization payload controls
   live in its optional form section. Preserve shared runtime credentials for MCP;
   HTTP authorization credentials remain separate from content signing secrets.
   Execute all matching authorization callbacks before all content callbacks,
   once per logical client request after authentication and enrichment. Reuse
   existing method/identifier selectors without a tools/call-only gate, including
   resource, prompt, discovery, and other method-only requests. Authorization cannot mutate identity/messages. Missing authorization
   tools or readiness failures block execution; internal evaluations cannot recurse.
3. Obot owns the versioned enrichment vocabulary and pinned mapping profile.
   Subject identity is authenticated; owner is separate from a verified OBO user.
   Agent-supplied delegation claims are insufficient. COAZ is optional. Forwarded
   source IP requires installation-level proxy trust and validated extraction;
   a per-filter disclosure toggle cannot grant trust. Otherwise retain peer IP
   and omit forwarded source IP. User-Agent remains caller-reported.
4. Explicit AuthZEN denial produces HTTP 403, except an optional Obot-owned
   `context.obot.reason: rate_limited` hint produces HTTP 429. An optional
   `retry_after_seconds` integer (1–86400) sets Retry-After and JSON-RPC retry data.
   Messages are gateway-generated, without arbitrary policy detail or internal
   hook prefixes. This additive response-context binding preserves boolean-only
   receivers and leaves quota state in the PDP; it is not a standard AuthZEN
   or MCP extension. Missing/invalid caller credentials
   remain 401. PDP authentication, malformed response, timeout, and availability
   failures block execution with a distinct 5xx. No automatic retry or decision
   caching initially; external stateful limits count admission attempts unless a
   separate settlement contract is designed.
5. Add typed authorization evaluations to the existing protected-request audit
   event. Include stable filter ID, correlation, mapping version, result,
   duration, and safe diagnostics. Record pre-upstream denials/errors and keep
   permit separate from final request outcome. The filter Audit Logs tab and
   Operations Audit Logs query the same records with intersecting filter-ID,
   target, and actor constraints under existing viewer permissions. Reuse
   retention, export, and payload controls; do not add a decision-log store.

## Rationale

One Filters workspace reuses familiar targeting and lifecycle controls while
callback-specific dispatch preserves compatibility. The gateway owns identity
and enforcement, so customers only implement the decision service. A shared
audit event explains both authorization and later execution without double
counting logical requests or maintaining parallel retention/access rules.

The implementation extends existing filter dispatch, runtime connections, and
audit plumbing. New adapters assemble enriched authorization inputs and decode
explicit decisions; the existing content invocation/result path remains intact.
A reference policy service is a development aid, never automatically enforced.

### Reuse boundaries

Extend the existing webhook manifest/CRUD, `WebhookHelper` attachment lookup,
`ServerHookConfig` selector compilation, and gateway `hookProcessor`. Reuse
`SessionManagerHookRunner` target/client/CallTool plumbing beneath separate
content and authorization decoders. Add a direct HTTP authorization adapter to
the same invocation stage. Use the current blocked-request response writer,
audit context/collector, custom form, selector picker, and audit view.

No second filter resource, attachment matcher, runtime pool, gateway route,
request parser, or audit store is proposed. Preserve one runtime for a combined
MCP filter. Build trusted enrichment once and apply each callback's disclosure
settings separately. Keep contract-specific results typed; a shared transport
does not make legacy accept/mutate decoding valid for authorization.

The current readiness gate and permissive content-result decoding cannot be
reused unchanged for authorization: missing readiness or a missing decision
must block execution rather than remove a required callback from consideration.
The ODP records the concrete source integration points and regression criteria.

## Consequences

- Legacy filters need no payload changes or migration. Old API clients must not
  erase authorization configuration. Combined MCP filters reuse their backing
  runtime; callback readiness cannot silently skip required authorization.
- Shared audit queries need an indexed filter-ID association. The existing
  `mcp_id=sms1<filter-id>` view is insufficient for a direct external PDP.
- Proxy trust/extraction needs a canonical implementation. v0.26.3's forwarded-IP
  extraction paths are observations, not an authorization trust policy.
- Disabling/removing authorization is an explicit audited administrative action.
  Rollback must account for binaries that do not understand the added authorization settings;
  configuration cannot silently become unenforced.
- The ODP must resolve argument mutation boundaries, multi-component target
  resolution and internal-hop suppression, deadlines/errors, audit schema, and
  rollback before implementation. OBO issuance remains follow-up design work.
- Authorization starts off on existing filters. Enabling it preserves selectors
  and attachments, including existing wildcard/default matching. No selector
  migration is required. Notifications and server-originated requests need
  explicit subject/direction and denial semantics; content behavior is unchanged.

## References

- [ADR lifecycle](README.md)
- [Released filter editor](https://github.com/obot-platform/obot/blob/v0.26.3/ui/user/src/lib/components/admin/FilterForm.svelte)
- [Released selectors and resources](https://github.com/obot-platform/obot/blob/v0.26.3/ui/user/src/lib/components/admin/SelectorsAndResourcesFormSegment.svelte)
- [Released filter audit view](https://github.com/obot-platform/obot/blob/v0.26.3/ui/user/src/routes/mcp-servers/FilterView.svelte)
- [Released vMCP audit navigation](https://github.com/obot-platform/obot/blob/v0.26.3/ui/user/src/lib/components/vmcps/VMcpMenuActions.svelte)
