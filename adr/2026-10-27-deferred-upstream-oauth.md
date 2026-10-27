# 2026-10-27: Defer upstream OAuth until an actual challenge

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** None
- **Superseded by:** None

## Related issues

[Obot #8384](https://github.com/obot-platform/obot/issues/8384).

## Related ODPs

None. This design was agreed in the implementation discussion and explicitly authorized for local implementation.

## Context

Some MCP servers allow discovery and listing but require OAuth for later operations. Preflight cannot reliably detect those requirements. Fan-out can encounter several independent challenges in one request.

## Decision

Capture actual upstream 401 and Bearer `insufficient_scope` 403 responses. Keep component identity and challenge parameters in a short-lived shared database record behind a random `obot:upstream:` scope. Bind it to the authenticated user, connection, resource, namespace and originating OAuth client when known.

Return the opaque scope to authenticated clients as HTTP 403 with `error="insufficient_scope"` so they request an additional OAuth grant instead of refreshing an existing token. Ordinary Obot authentication failures remain HTTP 401.

After first-level identity and connection validation, claim the record for one OAuth transaction. Save its requirements in that transaction before deleting the source. The existing component UI, metadata discovery, registration, PKCE, pending state, callback and token store handle authorization directly from the captured parameters. Every captured component must finish before issuing a downstream code. Repeated polls reuse a pending attempt; a short-lived shared counter bounds starts per component, user, connection and client.

Opaque scopes are echoed in downstream tokens but grant no permission. Missing, expired and consumed scopes are inert. Refresh and access-token validation remain independent of temporary records. Only the client retries the rejected operation.

## Rationale

This handles deferred challenges without speculative probes, operation replay, or a durable requirement/grant state machine. Shared storage permits different replicas to handle the request, authorization and callback.

## Consequences

Request-specific catalogs isolate feature families and selected calls. Obot includes the caller's current component OAuth grant metadata in the discovery revision, read from shared storage on each request. Removing credentials therefore forces rediscovery across replicas without probing unrelated operations or disabling catalog caching. Auth failures are not cached across users; fan-out completes all participating components. Target membership and configuration are validated again before upstream OAuth and callback completion. Temporary records and attempt budgets use the existing OAuth cleanup lifecycle.

## References

- [Challenge capture](../pkg/api/handlers/mcpgateway/upstream_oauth.go)
- [Transaction handoff and progress](../pkg/api/handlers/mcpgateway/oauth/upstream.go)
- [Shared persistence](../pkg/gateway/client/mcpoauthchallenge.go)
