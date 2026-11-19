# 2026-11-17: Assign MCP tunnels to synced catalog entries

- **Status:** Proposed
- **Date:** 2026-11-17
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [Obot #8393 — Support MCP tunnel assignment to synced catalog entries](https://github.com/obot-platform/obot/issues/8393)

## Related ODPs

- [Virtual MCPs](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md) defines the catalog snapshot and backing-server model.
- [Thin composites](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-08-16-thin-composites/README.md), superseded by Virtual MCPs, discusses surfacing an invalid upstream such as a removed tunnel at the component boundary.

## Context

Git owns synchronized catalog entries. Obot currently accepts a tunnel ID on directly managed remote entries and rejects it in synced manifests. Terraform can provision a tunnel with `obot_mcp_tunnel`, but its catalog-entry resource manages directly created entries. An operator cannot use it to route an existing Git-owned entry through a tunnel. [Original tunnel implementation](https://github.com/obot-platform/obot/pull/7311) · [Terraform provider](https://github.com/obot-platform/terraform-provider-obot#mcp-tunnels)

The [Virtual MCPs ADR](2026-09-01-virtual-mcps.md) puts deployed upstream definitions in component snapshots, which drive both shared and dedicated backing `MCPServer` resources. The [transparent proxy ADR](2026-08-17-transparently-proxy-mcp-traffic.md) places tunnel transport, upstream credentials, and network policy at the gateway's upstream connection boundary. An assignment needs to respect both boundaries.

## Concepts

- **Catalog entry.** A definition of one MCP server, including how Obot connects to it.
- **vMCP and component.** A virtual MCP (vMCP) is the endpoint clients connect to. Each MCP server added to it is a component.
- **Snapshot.** Obot already saves a copy of an entry's configuration when a component is added. For UI- and API-managed vMCPs, later entry edits take effect when an admin updates the vMCP.
- **Tunnel.** A connection that lets Obot reach an MCP server through a tunnel client's network.
- **Tunnel assignment.** A setting attached to a catalog entry that specifies which tunnel Obot uses to reach its MCP server.

## Decision

An Obot admin uses Terraform to assign a tunnel to an existing Git-synced catalog entry. Obot stores one assignment per entry, outside the Git catalog, and uses the tunnel to reach the entry's published MCP URL. Clients still connect to the same vMCP.

- **Eligibility.** A synced remote entry needs a literal HTTP(S) `fixedURL` and an explicit `entryKey`. If the URL fails Obot's direct-network checks, the Git author sets `tunnelRequired` and a supported `minObotVersion`.
- **Expected URL.** The admin sets `expected_url` in Terraform and applies it with Obot Admin or Owner credentials. Obot compares it with the entry's `fixedURL` from Git. If a Git edit makes the URLs differ, Obot marks the assignment invalid until the admin updates and reapplies Terraform. The Git catalog and Terraform can have different owners, so a catalog edit cannot silently send tunneled traffic to a different host.
- **Rollout.** A vMCP that adds the entry after a valid assignment exists uses the tunnel right away. vMCPs already connecting directly keep doing so until an admin runs **Update vMCP** for UI- or API-managed vMCPs, or runs a catalog sync with the explicit adopt-route-changes option for Git-managed vMCPs. Switching a live route can break the connection if the tunnel client is offline and requires users to reauthorize upstream OAuth where applicable.
- **Access.** Only Obot Admins and Owners can manage assignments. The assignment grants no new access to the entry or vMCP.
- **Failure.** The tunnel allowlist must cover the expected URL. If the tunnel is missing, disconnected, or denies the request, Obot returns an error and never falls back to direct access.

The primary use case is a customer-authored entry for a private MCP endpoint. Eligible shipped entries can use a tunnel for customer-controlled egress to their published URL. Private targets for URL templates or hostname-only entries are future work. Hosted MCP server egress is out of scope.

## Proposed configuration shape

```yaml
type: entry
entryKey: inventory
name: Inventory
minObotVersion: "<first supporting release>" # Older Obot ignores tunnelRequired and would connect directly.
runtime: remote
remoteConfig:
  fixedURL: https://inventory.internal.example/mcp
  tunnelRequired: true # Require a valid tunnel assignment for this entry.
```

```hcl
locals {
  inventory_url = "https://inventory.internal.example/mcp"
}

resource "obot_mcp_tunnel" "private_network" {
  display_name = "Private network"
  allowed_urls = ["https://inventory.internal.example/*"] # Cover MCP, OAuth, and redirect destinations.
}

# Proposed assignment resource.
resource "obot_mcp_tunnel_assignment" "inventory" {
  catalog_id   = "default"                          # Catalog containing the entry.
  source_id    = "example-source-id"                # Opaque ID from the catalog inventory.
  entry_key    = "inventory"                        # entryKey in the Git manifest.
  expected_url = local.inventory_url                # Set explicitly in Terraform to match Git fixedURL. Never read from the live catalog.
  tunnel_id    = obot_mcp_tunnel.private_network.id # Generated ID of the tunnel to use.
}
```

Obot lowercases the scheme and host and drops default ports before comparing URLs. Paths and queries remain significant. Terraform refresh reports the computed assignment status and reason.

For a source created in the same apply, use the catalog source resource's computed `source_id`, for example `obot_mcp_catalog_source.team.source_id`. The source resource still waits for catalog sync and uses its URL for `id` and import. [Terraform catalog source resource](https://github.com/obot-platform/terraform-provider-obot/blob/main/docs/resources/mcp_catalog_source.md)

Entries can share a tunnel if its allowlist covers their destinations. If OAuth uses another origin, allowlist its endpoints and ensure the tunnel client can reach them.

When the vMCP adopts the route, its component snapshot passes the tunnel ID to the backing MCP server.

```yaml
runtime: remote
remoteConfig:
  url: https://inventory.internal.example/mcp
  tunnelName: mt1example
```

## Terraform workflow

1. Sync the Git catalog. Terraform can wait for an initial sync up to the create timeout. Assignment creation requires an eligible entry.
2. Provision `obot_mcp_tunnel`, store its token through the existing credential workflow, and deploy the tunnel client where it can reach the MCP server.
3. Apply `obot_mcp_tunnel_assignment`. The client can connect later. Terraform supports reading, importing, updating, and deleting the assignment.
4. Run **Update vMCP** for existing UI- or API-managed vMCPs, or sync the catalog with explicit route adoption for Git-managed vMCPs. Confirm the tunnel works.

## Rationale

| Placement | Benefits | Costs |
| --- | --- | --- |
| Remote server through a catalog-entry assignment (proposed) | One Terraform assignment covers the entry across vMCPs. A mixed vMCP can reach each upstream through its own tunnel or directly. Catalog probes and backing servers can reuse the same transport and OAuth handling. | Every adopting vMCP uses the same route for that entry. The operator needs a way to review affected deployments and roll out the change. Two routes to the same entry would require separate entries. |
| Per vMCP component | Each deployment can choose its own route for the same entry. An operator can stage changes one vMCP at a time. | Terraform needs every target vMCP and component ID. Configuration is repeated across consumers, Git-managed vMCPs need their own operational override, and catalog probes have no vMCP component to consult. |
| One tunnel per vMCP | A single setting is simple when all upstreams are on one network. | A mixed vMCP may contain hosted servers, public remotes, and private remotes in different networks. It would need exceptions or per-component settings to route those upstreams. |

Writing the assignment directly to an individual backing `MCPServer` would couple Terraform to generated resources. Dedicated servers can appear as users connect, and both sharing modes can be rebuilt from the vMCP component snapshot. The entry-level assignment supplies the same data to each reconciliation.

## Consequences

- Assignments identify entries by catalog ID, opaque source ID, and `entryKey`. The catalog inventory exposes one stable ID per source. The ID contains no URL or credentials. Credential rotation preserves it. Source replacement requires a new assignment.
- Catalog sync validates Git definitions independently of assignments. For entries with `tunnelRequired`, sync checks URL syntax and defers route validation until assignment. Other entries keep direct-network validation. Synced definitions reject tunnel IDs. Only synced remote entries can declare `tunnelRequired`, and they must use the version gate with a floor at or above the first supporting release. Publish them only to installations that enforce that gate. [Catalog version range ADR](2026-10-07-catalog-entry-obot-version-range.md)
- A saved assignment becomes orphaned if its entry disappears or is detached. A rename that keeps the source ID and `entryKey` preserves the assignment. API-managed vMCPs referencing the old entry object may still report a missing source and need to reselect the renamed entry. Removing `tunnelRequired` leaves a valid assignment in effect. Changing the entry to an ineligible runtime invalidates the assignment. Other catalog changes follow the existing snapshot policy.
- Catalog probes and new deployments use the current assignment. An invalid assignment, a required tunnel with no valid assignment, or a detached source blocks new deployments and catalog probes. Direct routes still use the normal network policy. Check the allowlist when saving or adopting a route and on every tunneled request, including upstream OAuth.
- Component probes, MCP traffic, and upstream OAuth use the deployed route. Git-managed vMCPs currently rebuild snapshots during catalog sync. Under this decision, periodic sync reports pending route changes and keeps the deployed route, whether direct or tunneled. A rejected update preserves the affected vMCP's last valid deployment while other vMCPs and catalog removal processing continue. To return to direct access, remove the assignment and any tunnel requirement, then refresh under the direct-network policy.
- Bind pending OAuth flows and grants to the deployed route revision, including direct access. Users must reauthorize after a route change even if the URL stays the same. Reject stale callbacks before code exchange. An exchange already in progress cannot publish a grant usable on the new route.
- Tunnel deletion and allowlist narrowing must account for assignments and all serving snapshots, including connection snapshots retained by instances and snapshots whose source entries were removed. Assignment writes, snapshot adoption, and tunnel policy changes must enforce this invariant under concurrent writes. To avoid breaking serving deployments, require rollout to another route or retirement before allowing tunnel deletion. Terraform alone cannot complete that rollout. Use staged teardown and return blocking references on a rejected delete. Allowlist narrowing must preserve each referenced `fixedURL`. Removing a rule for a discovered OAuth or redirect destination can still break authentication or redirects at request time.
- Report the assignment, eligible target, adopted route, and tunnel connectivity distinctly. Distinguish a missing source or invalid assignment from a valid change awaiting adoption. A successful Terraform apply establishes the assignment. Runtime readiness also depends on rollout and a connected client.

## References

- [Managed MCP tunnel implementation](https://github.com/obot-platform/obot/pull/7311)
- [Terraform tunnel resource](https://github.com/obot-platform/terraform-provider-obot/blob/main/docs/resources/mcp_tunnel.md)
- [Terraform catalog data source](https://github.com/obot-platform/terraform-provider-obot/blob/main/docs/data-sources/mcp_catalog.md)
