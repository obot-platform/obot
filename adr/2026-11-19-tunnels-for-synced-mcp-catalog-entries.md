# 2026-11-19: Tunnel assignments for synced MCP catalog entries

- **Status:** Proposed
- **Date:** 2026-11-19
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [Support MCP Tunnel assignment to synced catalog entries #8393](https://github.com/obot-platform/obot/issues/8393)

## Context

Today, automating tunnel setup requires creating the MCP catalog entry outside Git because the catalog API rejects tunnel assignments for synced entries.
The goal is to keep the entry definition in Git while administrators provision the tunnel, run the tunnel agent, and attach the tunnel through their infrastructure automation.
Catalog sync reads definitions from the configured Git repository and creates or updates catalog entries in Obot.
Administrators provision MCP tunnels separately because their identities and credentials are specific to an Obot deployment.
A tunnel assignment stores the ID of an existing MCP tunnel on an MCP catalog entry, telling Obot which tunnel to use to reach that entry’s remote MCP server.
Direct-destination validation can also reject a private fixed URL before an entry exists to receive a tunnel assignment.

## Decision

Git owns the remote destination and may declare `remoteConfig.tunnelRequired: true`.
Remote entries with a literal fixed URL or a hostname are eligible for tunnel assignment.
Eligible entries without the flag also support optional tunnel assignments.
Git rejects tunnel IDs and does not support tunnel requirements for templated destinations.

Obot adds a new admin endpoint at `/api/mcp-catalogs/{catalog_id}/entries/{entry_id}/tunnel`.
GET reads the tunnel assignment, PUT sets or replaces it, and DELETE clears it.
The tunnel endpoint stores the tunnel ID in the entry’s existing `remoteConfig.tunnelName` field and supports replacement with PUT.
The server validates the selected tunnel and its allowed destinations against the current entry.
A separate PR in the [Obot Terraform provider repository](https://github.com/obot-platform/terraform-provider-obot) will add a resource that manages the tunnel assignment through this API.
Destroying that Terraform resource clears the tunnel assignment and preserves the Git-owned entry and the MCP tunnel.

When importing an entry marked `tunnelRequired` from Git, Obot checks the destination’s syntax and defers the tunnel allowlist check until an administrator assigns a tunnel.
The catalog entry and its vMCP configuration can exist before a tunnel is assigned.
A vMCP component whose catalog-entry snapshot contains `tunnelRequired: true` cannot establish an MCP session until the snapshot contains a tunnel ID.
A previously deployed snapshot that permits direct routing can continue to use it until that snapshot is updated.
While the tunnel assignment is missing, MCP connection attempts, tool previews, and upstream OAuth fail before making an outbound request.
An unavailable tunnel never triggers a direct connection.

Catalog sync reconciles Git changes against the entry’s current tunnel assignment before creating Git-synced vMCP snapshots.
If a destination or runtime change is incompatible, sync reports the conflict and retains the last valid entry and dependent snapshots.
Catalog sync and tunnel assignment writes detect version conflicts and revalidate against current state before retrying.

## Rationale

A Git flag that merely allows an administrator to assign a tunnel would still allow direct routing, so Obot would still apply its direct network policy and could reject a private URL during import.
Setting `tunnelRequired` lets Obot import the private URL and require a tunnel for connections using that configuration.
An unflagged entry must pass ordinary import validation and can use a direct connection when it has no tunnel assignment.
An administrator can still assign an allowed tunnel to that entry, for example to send traffic through a specific network.

The tunnel endpoint lets Terraform change just the tunnel assignment while Git continues to update the entry definition.

## Consequences

A vMCP component connects using the tunnel ID saved in its catalog-entry snapshot.
A successful catalog sync updates Git-managed vMCP snapshots to reflect a new, replaced, or cleared tunnel assignment.
Once a snapshot reflects a cleared tunnel assignment, a component that requires a tunnel becomes unavailable.
After the tunnel assignment is cleared from its snapshot, a component that permits direct routing can connect subject to Obot’s outbound network policy.
For a manually managed vMCP, an operator must explicitly upgrade the component snapshot to pick up the new tunnel assignment.
A pinned snapshot can continue using its previous tunnel after an administrator changes or clears the tunnel assignment.
Operators must account for those snapshots before retiring a tunnel or relying on a new routing requirement.

## References

- [Virtual MCP servers](2026-09-01-virtual-mcps.md)
