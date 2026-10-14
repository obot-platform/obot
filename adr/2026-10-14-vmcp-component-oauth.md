# 2026-10-14: Scope OAuth client overrides to vMCP components

- **Status:** Accepted
- **Date:** 2026-10-14
- **Supersedes:** [Virtual MCPs](2026-09-01-virtual-mcps.md) (exclusive catalog ownership of static OAuth credentials only)
- **Superseded by:** None

## Related issues

- [obot-platform/obot#8153](https://github.com/obot-platform/obot/issues/8153), including the clarification selecting per-component configuration.

## Related ODPs

- [Virtual MCPs](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md), whose catalog credential model this decision extends.

## Context

One shared OAuth client per catalog entry cannot represent two vMCPs using different registered applications for the same remote server. The required flag also conflates optional client configuration with a prerequisite for using the server.

## Decision

Administrators may configure an encrypted static OAuth client override for a remote component of a UI-managed vMCP. Ownership follows the vMCP ID and immutable component ID, independently of names, runtime sharing mode, snapshots, and individual user connections. Component credentials take precedence over the existing catalog credential reference. Without either, the existing automatic registration flow remains available unless static OAuth is required.

Credentials are managed through dedicated admin-only endpoints, outside the vMCP manifest and ordinary configuration reveal endpoints. Git-managed definitions do not support override management in this version. Users continue to authorize their own accounts; sharing an OAuth client does not share user tokens.

Credential changes invalidate that component's existing user authorizations and pending callbacks. Durable component revision annotations support reconciliation and refresh cached readiness. Callback completion atomically consumes pending state and stores its token. Preview identities include the credential revision so earlier preview authorization cannot be reused.

## Rationale

Component ownership provides independent OAuth applications without duplicating catalog entries. Catalog fallback preserves deployed behavior and administrator defaults. Reusing encrypted credentials keeps secrets outside snapshots and avoids a second secret-storage mechanism.

## Consequences

- Catalog rotation does not revoke components using their own override.
- Saving or clearing an override requires users of that component to authorize again.
- Required components may be saved before credentials exist, but cannot connect until configured.
- Component removal and vMCP deletion must clean up owned credentials.
- Future GitOps support needs a separate configuration contract; arbitrary credential references are not accepted from clients.
