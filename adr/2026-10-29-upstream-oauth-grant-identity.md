# 2026-10-29: Persist upstream OAuth grant identity

- **Status:** Accepted
- **Date:** 2026-10-29
- **Supersedes:** None
- **Superseded by:** None

## Related issues

[Obot #8384](https://github.com/obot-platform/obot/issues/8384).

## Related ODPs

None. These corrections to the deferred OAuth implementation were explicitly authorized during PR review.

## Context

The [deferred OAuth flow](2026-10-27-deferred-upstream-oauth.md) stores upstream tokens separately from authorization-request progress. A failed progress write must not require another exchange of a consumed authorization code. Discovery caches must also distinguish successive logins with identical scopes, including account changes.

## Decision

Store a random grant ID and the captured challenge identity with each successful token replacement. Refresh preserves that metadata and rejects writes from a superseded grant. Discovery revisions include the grant ID. Polling may reconcile an unfinished requirement only from a token matching its user, connection, resource URL, OAuth request and challenge, after validating current component membership and configuration.

User connections go through vMCPs. Captured challenges use the existing vMCP authorization transaction and component authentication flow; no direct-server login flow is added.

Token configuration lookup requires the exact MCP URL, preventing credentials and scopes for an old resource from carrying into a new connection.

## Consequences

The existing database migration adds two token metadata columns. Legacy grants acquire an identity on their next authorization; refresh does not change their cache identity. A missing or superseded token cannot prove completion of an old authorization request. Challenge expiration and authorization failures still require a new attempt. No MCP operation is replayed by the gateway.
