# 2026-10-02: Scope CLI login to a pending UI authentication attempt

- **Status:** Accepted
- **Date:** 2026-10-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None supplied.

## Related ODPs

None.

## Context

Preconfiguration, temporary tool previews, and the inspector already create OAuth attempts with a specific MCP configuration and credential owner. Providers requiring localhost callbacks need a local listener for these UI flows.

## Decision

For UI-initiated localhost OAuth, return a short-lived URL referencing the existing pending OAuth state. The UI displays `obot mcp login --url <attempt URL>` instead of its provider Authenticate link.

Obot selects a loopback port before creating the provider authorization request. The CLI retrieves only the authorization URL, redirect URI, and state; binds that port; opens the browser; and redirects the provider callback to Obot. Obot exchanges and stores the upstream token under the original pending state's identity, then redirects to the CLI's completion endpoint. The CLI exits after completion without creating an MCP session, registering an Obot OAuth client, or reading or storing tokens.

The UI retries its original operation after authentication. Existing temporary resource cleanup remains responsible for discarding preview resources and credentials. Native-client `mcp connect` authorization retains its existing contract.

## Rationale

Reusing pending state preserves the exact configuration and credential owner, including system-owned temporary previews. A general MCP connection login would authenticate a different resource or identity and introduce unnecessary Obot token handling.

## Consequences

The attempt URL is a capability, expires after ten minutes, and exposes no client secret, PKCE verifier, or access token. The CLI and browser must run on the same computer. If the selected port is occupied or the attempt expires, the UI must generate another attempt. Provider callback paths remain part of the pending state rather than CLI flags.

## References

- [Localhost MCP callback relay](2026-09-23-localhost-mcp-oauth.md)
- [UI login handler](../pkg/api/handlers/mcpgateway/oauth/local_login.go)
- [CLI relay](../pkg/cli/internal/mcplogin/run.go)
