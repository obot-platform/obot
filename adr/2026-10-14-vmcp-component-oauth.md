# 2026-10-14: Scope OAuth client overrides to vMCP components

- **Status:** Accepted
- **Date:** 2026-10-14
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#8153](https://github.com/obot-platform/obot/issues/8153), including the clarification selecting per-component configuration.

## Related ODPs

- [Virtual MCPs](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md), whose catalog credential reference remains the fallback for components without an override.

## Context

One shared OAuth client per catalog entry cannot represent two vMCPs using different registered applications for the same remote server. The required flag also conflates optional client configuration with a prerequisite for using the server.

## Decision

Administrators may configure an encrypted static OAuth client override for a remote component of a UI-managed vMCP. Ownership follows the vMCP ID and immutable component ID, independently of names, runtime sharing mode, snapshots, and individual user connections. Component credentials take precedence over the existing catalog credential reference. Without either, the existing automatic registration flow remains available unless static OAuth is required.

The Client ID and secret are stored in the existing encrypted credential store under a context derived from the vMCP ID and component ID. They are managed through dedicated admin-only endpoints; the manifest contains neither these values nor a configurable reference to the override. Credential status exposes the Client ID and source, but never returns the secret. Ordinary configuration reveal endpoints do not include these credentials. Git-managed vMCP definitions are read-only in the UI. In this version, component OAuth management follows that policy: the credential endpoints reject changes for Git-managed vMCPs, even though the credentials are stored separately. This is a management-policy restriction, not a limitation of the credential store; supporting Git-managed overrides requires an explicit configuration and ownership contract. Users continue to authorize their own accounts; sharing an OAuth client does not share user tokens.

Credential changes invalidate that component's existing user authorizations and pending callbacks. Durable component revision annotations support reconciliation and refresh cached readiness. Callback completion atomically consumes pending state and stores its token. Preview identities include the credential revision so earlier preview authorization cannot be reused.

## Rationale

Component ownership provides independent OAuth applications without duplicating catalog entries. Catalog fallback preserves deployed behavior and administrator defaults. This adds an override without superseding the earlier vMCP decision to keep static OAuth secret material outside snapshots.

The credential store follows the existing approach for creator-supplied fixed configuration: values are stored separately from the manifest. A separate OAuth credential also keeps its client secret out of ordinary configuration reveal responses. The dedicated endpoints enforce the current admin-only management policy and invalidate component authorizations when credentials change.

Keeping even the override reference out of the manifest is a narrower implementation choice, rather than a requirement of admin-only management. The vMCP ID and immutable component ID already identify the credential, so a separate reference is unnecessary for this UI-managed implementation. The tradeoff is that exporting or copying a manifest does not carry the override; credentials must be configured separately for the destination vMCP. A future manifest-based configuration contract could use an explicit secret reference without embedding the secret itself.

## Consequences

- Catalog rotation does not revoke components using their own override.
- Saving or clearing an override requires users of that component to authorize again.
- Required components may be saved before credentials exist, but cannot connect until configured.
- Component removal and vMCP deletion must clean up owned credentials.
- Future GitOps support needs a separate configuration contract; arbitrary credential references are not accepted from clients.
