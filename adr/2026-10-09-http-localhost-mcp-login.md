# 2026-10-09: Use a temporary CLI listener for localhost OAuth callbacks

- **Status:** Accepted
- **Date:** 2026-10-09
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

Some MCP servers require their OAuth clients to use a localhost callback. Obot acts as the OAuth client, but its hosted callback cannot meet that requirement.

## Decision

Use `obot mcp login <attempt URL>` to run a temporary callback listener on the user's computer. The CLI opens the authorization page and relays the callback to Obot, which exchanges the code and stores the credentials. The CLI exits when authentication completes.

Each command is scoped to a pending authentication attempt and its credential owner. Client authorization, the inspector, and tool selection use the same flow. MCP clients continue connecting through Obot's HTTP URLs.

## Rationale

A local listener satisfies the server's callback requirement while keeping provider credentials and MCP traffic under Obot's control. The CLI is needed only during authentication.

## Consequences

The CLI and browser must run on the same computer. Login attempts expire after ten minutes, and providers must accept an available callback port. The CLI does not store tokens. Servers without this requirement continue using Obot's hosted callback.

## References

- [Localhost OAuth setup](../docs/docs/mcp-gateway/register-remote.md#localhost-oauth-callbacks)
- [Login handler](../pkg/api/handlers/mcpgateway/oauth/local_login.go)
- [CLI listener](../pkg/cli/internal/mcplogin/run.go)
