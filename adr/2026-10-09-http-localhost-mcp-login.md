# 2026-10-09: Keep MCP connections on HTTP with scoped localhost login

- **Status:** Accepted
- **Date:** 2026-10-09
- **Supersedes:** [Localhost callback relay](2026-09-23-localhost-mcp-oauth.md), [UI login attempts](2026-10-02-ui-localhost-mcp-login.md)
- **Superseded by:** None

## Related issues

None supplied.

## Related ODPs

None.

## Context

Providers that require localhost OAuth callbacks need a local listener during authentication. Requiring a persistent STDIO bridge for these providers changes client configuration and prevents normal HTTP registry installation unnecessarily.

## Decision

Advertise the normal HTTP connection URL for all providers. During browser authorization, show `obot mcp login --url <attempt URL>` when a provider requires a localhost callback. Reuse the inspector and tool-selection login helper for client-initiated attempts, preserving the original OAuth request and credential owner.

Select a temporary loopback port independently of the MCP client's registered callback. The CLI binds that port, opens the provider authorization URL, and relays the callback to Obot. Obot validates the approving user's identity for client-linked attempts, exchanges and stores the token, consumes the pending state, and notifies the CLI so it exits. The authorization screen shows a waiting state and watches the existing pending attempt without creating new OAuth states. After the attempt is consumed or expires, Obot checks authentication and either issues a replacement command or continues to the original client callback. A refresh action lets the user request a new command, including when the callback port is busy. Aggregate connections finish only after all required components authenticate.

## Rationale

The local listener is needed only for provider authentication. Separating it from MCP transport supports ordinary HTTP clients, including clients whose callback is hosted, while retaining Obot's token custody and governance.

## Consequences

The attempt URL remains a ten-minute capability exposing no secrets or tokens. The browser and login CLI must run on the same computer. Expired attempts and occupied callback ports require a fresh attempt. Hosted callbacks and temporary preview credential ownership retain their existing behavior. The STDIO bridge command is removed; the CLI only assists provider authentication with `mcp login`. Registry conversion no longer inspects user-specific component snapshots to decide transport.

## References

- [Scoped login handlers](../pkg/api/handlers/mcpgateway/oauth/local_login.go)
- [CLI login relay](../pkg/cli/internal/mcplogin/run.go)
