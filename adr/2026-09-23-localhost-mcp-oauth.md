# 2026-09-23: Relay localhost MCP OAuth callbacks through the Obot CLI

- **Status:** Accepted
- **Date:** 2026-09-23
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None supplied.

## Related ODPs

None.

## Context

Some upstream MCP providers require localhost OAuth callbacks. Users still need Obot to hold upstream credentials and govern their MCP traffic.

## Decision

`obot mcp connect <URL>` bridges local STDIO messages to an Obot HTTP connection using the current SDK's transport and OAuth facilities. It registers a loopback callback with Obot and stores only its own Obot OAuth credentials locally.

Remote configuration can opt into localhost callbacks with an optional callback path. Obot derives the provider callback from the validated originating OAuth request's loopback host and port. The CLI receives that callback and redirects the browser to Obot's hosted callback, preserving the query. Obot exchanges the code using the original redirect URI and retains upstream tokens. This applies to direct connections and vMCP components.

The bridge forwards initialization and capabilities unchanged. It supplies negotiated protocol headers and maintains the optional standalone event stream for legacy HTTP sessions because the SDK's low-level transport does not manage those without a terminating client session.

## Rationale

A browser redirect preserves the hosted authorization flow and its browser session. Exchanging provider tokens locally would introduce a new credential-upload boundary. Forwarding wire messages avoids substituting CLI capabilities for the actual client's capabilities.

## Consequences

The CLI and browser must share a reachable loopback interface. Providers must accept an available port. Hosted callbacks remain the default. Local callback flows use static credentials or dynamic registration rather than Obot's hosted client metadata document, which advertises the hosted callback.

The CLI's credential files require restricted permissions. Gateway and CLI versions must both support the feature before it is enabled. The transport bridge requires coverage for negotiated headers, server messages, cancellation, authentication, and process shutdown.

## References

- [Gateway proxy architecture](2026-08-17-transparently-proxy-mcp-traffic.md)
