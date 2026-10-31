# 2026-10-30: Serve built-in MCP servers in-process as vMCP components

- **Status:** Accepted
- **Date:** 2026-10-30
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None yet.

## Context

Obot needs MCP servers that it provides itself, starting with the Obot Admin server, which helps admins author catalog source files with coding agents. Such a server reads Obot's own state as the connected user.

Every existing way to run one has a cost. A deployed server, such as the system MCP server that preceded it, needs a container per installation and a way to exchange the user's token for one it can use against Obot's API; in hosted installations each deployment costs money. A dedicated endpoint, which an early version of the Obot Admin server used at `/admin/mcp`, needs its own OAuth flow, token class, authorization rules, audit logging, and on/off setting, all duplicating what the MCP gateway already does for vMCPs.

## Decision

Add a `builtin` MCP runtime. A built-in server is an `http.Handler` registered with the MCP gateway under a name; a catalog entry with runtime `builtin` names it in `builtinConfig.name`. Like the remote runtime, it deploys nothing.

Built-in servers are reachable only as vMCP components. When the gateway proxies a vMCP's loopback request to a built-in component, it serves the request with the registered handler in the same process, through an in-memory transport that streams the response. Any other request for a built-in server is refused. Users connect to the vMCP through the standard vMCP OAuth flow, the vMCP's profiles decide who can connect and which tools they get, and the gateway audits the calls as it does for every vMCP.

Obot owns the catalog entries of built-in servers, and only admins and owners see them or can add them to a vMCP. It creates them in the default catalog at startup, keeps them current, and locks them: they can't be created, edited, or deleted through the API, and catalog sync rejects the runtime. A built-in server's tool list is stored on its entry, because it can change only when Obot is upgraded, and a vMCP component's tools can't be overridden; profiles choose who gets which tools.

The Obot Admin server is the first built-in server. Because profiles are configuration and can grant too broadly, it also refuses any caller who isn't currently an admin or owner. The `--disable-admin-mcp` server flag removes its entry and handler; vMCPs that already include it keep the component, which then has no tools. It only reads Obot's state. Listing the tools of an entry without a stored tool list launches the entry's server, so the server leaves that to the user: it returns an `obot mcp list-tools <entry-id>` command. The command creates a short-lived tool list request, opens the entry's Tools tab in the user's browser, and waits. The user confirms there, with the same tool preview the UI already uses, so configuration prompts and OAuth sign-in work as they do elsewhere: Obot launches the server and lists its tools without saving them, the tab shows them, and the user's browser records them on the request, which the command prints. A launch that fails, or that the user declines, is recorded too, so the command reports it at once. The tab answers a request only for its own entry. Only the user who made the request can answer it, from a browser session; an API key can't, so an agent holding the CLI's credentials can't answer it.

## Rationale

Serving in-process through the gateway costs nothing to deploy and reuses the vMCP model for sign-in, access, tool grants, and audit, so a built-in server adds tools without adding a security model. The dedicated endpoint was built first and removed for this reason: it needed a separate token class, OAuth branches, authorization rules, audit redaction, and a settings toggle, and still bypassed vMCP profiles. A deployed server would have needed per-installation infrastructure and a token exchange.

## Consequences

- A built-in server runs in Obot's process and with Obot's privileges, so it must check the caller itself, as the Obot Admin server checks for an admin or owner, and its code is part of Obot's attack surface. Built-in servers can only be added by changing Obot.
- Access is decided per vMCP. There is no global runtime switch for a built-in server; the server flag removes it entirely, and the vMCPs that included it lose its tools but keep working.
- A built-in server's MCP client is the vMCP aggregator, not the user's agent, so the server can't tell what the agent supports, and an input request reaches the agent as whatever the aggregator relays.
- The vMCP aggregator doesn't forward a component's server instructions, so a built-in server's instructions don't reach agents. Agents need its tool descriptions and any accompanying skill instead.
- Built-in catalog entries have no source URL, so vMCPs defined in git catalogs can't reference them; admins add them to vMCPs through the UI or API.
- The launch confirmation happens in the Obot UI because MCP clients can't be relied on to show it: a client answered an MCP input request on its own, with no prompt. Listing an entry's tools therefore needs the obot CLI, logged in, on the machine where the agent runs, and a browser the user can reach; an agent in a remote sandbox has to hand the command to the user. Someone who can drive the user's browser, or who holds an API key with full API access, can still launch entries through Obot's other APIs.
- If a component refuses a caller, the vMCP's discovery fails for that caller. Non-admins can't add the Obot Admin server to their own vMCPs, but a vMCP that includes it and whose profiles grant non-admins is unusable for those users.

## References

- [Gateway in-process transport](../pkg/api/handlers/mcpgateway/builtin.go)
- [Obot Admin server and its catalog entry](../pkg/adminmcp/builtin.go)
- [Runtime type](../apiclient/types/mcpserver.go)
- [vMCP authoring skill](../pkg/localagents/assets/files/skills/obot-author-vmcps/SKILL.md.tmpl), which `obot setup` installs for admins and owners
- [Tool list requests](../pkg/api/handlers/toollistrequests.go) and the [obot mcp list-tools command](../pkg/cli/mcp_list_tools.go)
