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
A tunnel assignment links an existing MCP tunnel to an MCP catalog entry, telling Obot which tunnel to use to reach that entry’s remote MCP server.
Direct-destination validation can also reject a private fixed URL before an entry exists to receive a tunnel assignment.

## Decision

Git owns the remote destination and may declare `remoteConfig.tunnelRequired: true`.
Synced remote entries with an explicit `entryKey` and a literal HTTP(S) `fixedURL` are eligible for tunnel assignment.
Eligible entries without the flag support optional tunnel assignments.
Git rejects concrete tunnel IDs.
Entries using `tunnelRequired` must set `minObotVersion` to the first release supporting it or later and be published only to installations that enforce that version gate.

### Tunnel assignment identity and approval

Obot stores the tunnel assignment on the existing catalog entry, using `remoteConfig.tunnelName` for the tunnel ID and an Obot-managed field for the approved fixed URL.
Obot resolves the entry by catalog ID, opaque catalog source ID, and the explicit Git `entryKey`.
The catalog inventory exposes a server-assigned ID for each configured source.
When a rename changes the entry object ID, catalog sync carries the tunnel assignment forward only for an exact, unambiguous match on the same catalog ID, source ID, and entry key.
Sync preserves concurrent admin changes when carrying the tunnel assignment forward and retries on a version conflict.
Replacing the source or changing the entry key requires a new tunnel assignment.

### Admin API

Obot adds `GET`, `PUT`, and `DELETE` at
`/api/mcp-catalogs/{catalog_id}/sources/{source_id}/entries/{entry_key}/tunnel`.
Obot resolves the entry from the path on each request, so Terraform can manage the tunnel assignment in one resource without first looking up the entry's object ID.

- `PUT` creates or replaces the tunnel assignment. It accepts `{"tunnelName":"mt1example","expectedURL":"https://inventory.internal.example/mcp"}` and returns the saved assignment.
- `GET` returns the saved tunnel ID and approved URL, the current Git URL, and the assignment's validity and reason. It reports a changed URL as an invalid assignment. It returns `404` when the entry or assignment is absent.

When setting a tunnel assignment, Obot requires exactly one matching synced entry, verifies that the expected URL matches its current Git-defined URL, and checks the selected tunnel's allowlist.
A Git URL change invalidates the tunnel assignment even when the existing allowlist covers the new URL.
To approve the new URL, an administrator sends a new PUT with that URL as `expectedURL` or updates `expected_url` in Terraform and applies.
Removing `tunnelRequired` leaves a valid tunnel assignment in effect.

### Terraform

A separate PR in the [Obot Terraform provider repository](https://github.com/obot-platform/terraform-provider-obot) will add a resource that manages tunnel assignments through this API.
The existing `obot_mcp_catalog_source` resource will expose the opaque ID as a computed `source_id` while retaining its existing URL-based identity and sync wait.
Referencing that output makes the tunnel assignment depend on source creation and initial catalog sync.
For a source managed outside Terraform, the operator can obtain its ID from catalog inventory.
The assignment resource uses a reviewed `expected_url` from Terraform configuration, so a Git edit cannot implicitly approve a new destination.

```hcl
resource "obot_mcp_tunnel_assignment" "inventory" {
  catalog_id   = "default"
  source_id    = obot_mcp_catalog_source.team.source_id
  entry_key    = "inventory"
  expected_url = "https://inventory.internal.example/mcp"
  tunnel_id    = obot_mcp_tunnel.private_network.id
}
```

Destroying the Terraform assignment resource clears the tunnel assignment and preserves the Git-owned entry and the MCP tunnel.

### Routing and rollout

When importing an entry marked `tunnelRequired` from Git, Obot checks the destination’s syntax and defers the tunnel allowlist check until an administrator assigns a tunnel.
The catalog entry and its vMCP configuration can exist before a tunnel is assigned.
A vMCP component whose catalog-entry snapshot contains `tunnelRequired: true` cannot establish an MCP session until the snapshot contains a tunnel ID.
A previously deployed snapshot that permits direct routing can continue to use it until that snapshot is updated.
While the tunnel assignment is missing, MCP connection attempts, tool previews, and upstream OAuth fail before making an outbound request.
An unavailable tunnel never triggers a direct connection.

Catalog sync records the latest Git entry so administrators can approve a changed destination.
Before building Git-managed vMCP snapshots, Obot resolves the entry's tunnel assignment and revalidates the approved URL and the tunnel allowlist.
An invalid tunnel assignment blocks adoption of the affected route and preserves the last valid deployed snapshots.
Obot reports the conflict on the affected components and continues reconciling unrelated entries and vMCPs.
Assignment and snapshot writes detect version conflicts and revalidate against current state before retrying.
Status distinguishes the saved tunnel assignment, the route adopted by each affected component, and tunnel connectivity.

Upstream OAuth pending flows and grants are bound to the deployed route revision, including direct routes.
Moving between direct access and a tunnel or between tunnels requires fresh upstream authorization.
Callbacks and in-flight exchanges for a superseded route cannot publish grants usable by the new route.

## Rationale

A Git flag that merely allows an administrator to assign a tunnel would still allow direct routing, so Obot would still apply its direct network policy and could reject a private URL during import.
Setting `tunnelRequired` lets Obot import the private URL and require a tunnel for connections using that configuration.
An unflagged entry must pass ordinary import validation and can use a direct connection when it has no tunnel assignment.
An administrator can still assign an allowed tunnel to that entry, for example to send traffic through a specific network.

Storing the tunnel assignment on the catalog entry ties its lifetime to that entry.
The stable lookup lets Terraform address a renamed entry through the same API identity while catalog sync carries the administrator-approved fields forward.
The tunnel allowlist controls where the tunnel can send requests, while the approved URL records which destination the operator intended for this entry.

## Consequences

A vMCP component connects using the tunnel ID saved in its catalog-entry snapshot.
A successful catalog sync updates Git-managed vMCP snapshots to reflect a new, replaced, or cleared tunnel assignment.
Once a snapshot reflects a cleared tunnel assignment, a component that requires a tunnel becomes unavailable.
After the tunnel assignment is cleared from its snapshot, a component that permits direct routing can connect subject to Obot’s outbound network policy.
For a manually managed vMCP, an operator must explicitly upgrade the component snapshot to pick up the new tunnel assignment.
A pinned snapshot can continue using its previous tunnel after an administrator changes or clears the tunnel assignment.
Obot blocks tunnel deletion or allowlist narrowing while a saved tunnel assignment or serving snapshot still depends on the affected route, including snapshots whose source entry has been removed.
These checks also apply when assignments, snapshots, and tunnel policies change concurrently.
Operators may need to clear the tunnel assignment, update or retire affected snapshots, and then remove the tunnel in a separate Terraform apply.

## References

- [Virtual MCP servers](2026-09-01-virtual-mcps.md)
- [Related tunnel assignment proposal](https://github.com/cloudnautique/obot/commit/1ec25bb304be04011d680e4bad5a73049d3d4240)
