# 2026-09-29: Add a `none` MCP runtime backend for remote-only deployments

- **Status:** Accepted
- **Date:** 2026-09-29
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#7978](https://github.com/obot-platform/obot/issues/7978) - allow running without a
  runtime backend when no MCP servers are hosted. Implemented in
  [obot-platform/obot#8055](https://github.com/obot-platform/obot/pull/8055).

## Related ODPs

None.

## Context

Since the proxy rework (#6261), remote MCP servers and composites of them no longer
get a container: Obot itself is the proxy. A container runtime is only needed for
hosted MCP servers (npx, uvx, containerized) and hosted agents. The Docker backend was
still initialized unconditionally at startup, so an Obot without access to a Docker
API exited, and a Docker deployment that only connects remote servers still had to
mount the host Docker socket, which lets users who can deploy MCP servers run code on
the host.

## Decision

`OBOT_SERVER_MCPRUNTIME_BACKEND=none` selects a runtime backend that initializes and
contacts no container runtime:

- `remote` and `vmcp` servers work as with the other backends; hosted runtimes are
  refused with `ErrNotSupportedByBackend`, and so are logs, details and restarts.
  Shutting a server down is a no-op, since nothing is ever deployed.
- When Obot calls itself (filters, system MCP servers, composite loopbacks), URLs that
  target Obot are rewritten to the local HTTP listener, `http://localhost:<port>`, as
  the Docker backend does with its host address. Remote URL validation keeps the
  global settings and allows exactly `localhost:<port>`, as the Kubernetes backend
  does, so those calls also work with the default `DisallowLocalhostMCP=true`.
- The system MCP server controller does not retry a launch that fails with
  `ErrNotSupportedByBackend` (e.g. the containerized built-in `obot` server when
  agents are enabled): it logs a warning. Other launch errors are still retried.

## Rationale

- An explicit backend rather than a Docker backend that tolerates a missing Docker
  API: every Docker operation would need its own "no Docker" path, while `none`
  states the limit once and is testable on its own.
- An explicit opt-in rather than detecting that Docker is unreachable: a transient
  Docker failure must not silently turn a deployment into a remote-only one.
- The self-call rewrite and the single allowlisted `localhost:<port>` reuse the
  existing Docker and Kubernetes patterns instead of adding a new mechanism, and
  open nothing else on localhost.
- Not retrying an unsupported launch: it cannot succeed on a later attempt with the
  same backend, and retrying only fills the logs.
- Not a Docker socket proxy or rootless Podman (the alternatives in #7978): creating
  containers stays root-equivalent on the host, or the server still gets a runtime it
  does not use. Not Kubernetes alone: too much infrastructure for a gateway to remote
  servers.

## Consequences

- A remote-only deployment can omit the Docker socket mount, which removes the host
  code execution path through deployed MCP servers. The default backend is still
  `docker`: using `none` is an explicit operator choice.
- With `none`, hosted MCP servers are refused, and containerized system MCP servers,
  including the built-in `obot` server that agents need, do not run. The hosted-agents
  backend is unchanged: for non-Kubernetes runtimes it still resolves to the
  process-local `fake` backend, which does not refuse agents, so agents do not work
  end to end. Unlike what #7978 proposed, hosted agents are not refused with an
  explicit error; `OBOT_HOSTED_AGENTS_BACKEND=disabled` turns them off.
- Future code that launches containers must handle `ErrNotSupportedByBackend`
  (refuse with a clear error, or stop retrying) instead of assuming a runtime exists.
- The UI still offers hosted server options with `none`; the API refuses them with an
  explicit error. Hiding them is possible follow-up work.

## References

- `pkg/mcp/none.go`, `pkg/mcp/manager.go` (backend selection)
- `pkg/controller/handlers/systemmcpserver/systemmcpserver.go` (`launchError`)
- `docs/docs/installation/docker-deployment.md`, `docs/docs/configuration/server-configuration.md`
