# 2026-10-30: Serve built-in MCP servers in-process as vMCP components

- **Status:** Accepted
- **Date:** 2026-10-30
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

Coding agents authoring catalog files need live entry references, tool names, group IDs and configuration requirements. A local schema cannot verify those values or show which tools a profile grants. Giving an agent general administration credentials would grant more authority than authoring requires.

## Decision

Obot registers named HTTP handlers for the `builtin` runtime. Only authenticated vMCP component requests can reach them. The gateway streams requests and responses in process and reuses vMCP authentication, profile grants and audit. The runtime deploys no pod and consumes no compute entitlement.

The first handler, `obot-admin`, independently checks that the caller is currently an admin or owner. Obot maintains its catalog entry and tool list at startup. An admin adds the component to a vMCP through the UI or API and selects tool grants through profiles. Git-managed vMCPs require source-backed entry references.

The Admin MCP tools support read-only authoring. They discover metadata, including allowed Kubernetes Secret names and keys, and validate submitted files against current state. Agents write the files locally. Git review and catalog sync remain the path for applying them. Validation can fetch OpenAPI schemas under the catalog network policy. A successful preview does not reserve capacity for sync.

## Rationale

A dedicated public Admin MCP endpoint would duplicate the gateway's access and audit model. A separate deployment would need infrastructure and a credential exchange. Serving the handler as a component avoids both while keeping the caller's role check close to the privileged operations.

Live tool discovery can launch an entry's server. For entries without a stored tool list, the authoring tool directs the caller to `obot mcp list-tools`. The CLI creates a short-lived request. Its owner confirms in the entry's Tools tab, where configuration and OAuth prompts already run. An API key cannot complete the request. This keeps launch authority out of the authoring tool set.

## Consequences

Built-ins share Obot's process and privileges. The transport propagates cancellation and contains panics on its handler goroutine. Installations should upgrade all serving replicas before adding built-in components and remove those components before downgrading to a release that lacks the runtime.

## References

- [Built-in transport](../pkg/api/handlers/mcpgateway/builtin.go)
- [Admin handler and catalog entry](../pkg/adminmcp/builtin.go)
- [Catalog authoring skill](../pkg/localagents/assets/files/skills/obot-author-vmcps/SKILL.md.tmpl)
- [CLI discovery](../pkg/cli/mcp_list_tools.go) and [tool-list request authorization](../pkg/api/handlers/toollistrequests.go)
