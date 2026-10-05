---
title: Data storage and lifecycle
---

Obot stores platform records, activity history, and workload files separately. Each needs its own plan for surviving restarts, removing old data, and recovering from a failure.

## Where data is stored

<a id="keep-the-database-across-container-replacement" name="keep-the-database-across-container-replacement" />

**Use an external PostgreSQL database for production**, configured through `OBOT_SERVER_DSN`. See [Kubernetes production deployment](../installation/kubernetes-deployment.md).

The bundled PostgreSQL database is for testing and evaluation. In that setup, the [Docker deployment](../installation/docker-deployment.md) uses a `/data` volume to retain its database and local files.

| Data | Storage |
|---|---|
| Platform configuration, identities, and metadata | External PostgreSQL in production; bundled PostgreSQL for testing and evaluation |
| MCP and LLM audit records | The Obot database |
| Device scan history | The Obot database; scans can include captured configuration files containing sensitive content |
| Obot Agent workspace files | Workload filesystem unless persistent workspace storage is configured |
| Hosted Agents pool data | A per-pool volume, separate from Obot Agent workspaces |
| Published workflow packages | Object storage or the local published-artifact directory under `/data` |

## Keep files across restarts

The Obot server's data volume, agent workspaces, and published workflows serve different purposes. Configuring one does not automatically preserve the others.

- **Testing and evaluation:** Retain the Docker `/data` volume to keep the bundled database and local files when replacing the container.
- **Agent workspaces:** Files created by Obot Agent need persistent workspace storage to survive workload replacement. Hosted Agents use separate per-pool volumes. Follow the settings for the [agent runtime you use](../agents/runtime.md).
- **Published workflows:** Publishing creates a stored package containing the workflow's files. Configure object storage or preserve the local artifact directory. For multiple Obot replicas, use storage they can all access. See [published workflow storage](../agents/workflows.md#workflow-sharing-published-workflow-storage).

The [Kubernetes persistent storage guide](../installation/kubernetes-persistent-storage.md) covers persistent storage for local workflow artifacts and Obot Agent workspaces.

## Decide how long to keep activity records

Retention controls when Obot automatically deletes old records. MCP audit logs, LLM audit logs, and device scans each have a separate retention setting; changing one does not change the others. Choose the retention period for each in [Server configuration](../configuration/server-configuration.md).

If you need MCP or LLM audit records after their retention period ends, [export them](../security/audit-data.md#audit-log-export-creating-exports) before cleanup removes them. Configure how long exported files are kept in the destination storage separately.

## Understand what encryption protects

Application encryption is disabled by default. When configured, it protects selected stored fields, such as credential secrets and gateway request and response bodies. Other metadata can remain readable. The [encryption coverage table](../security/credentials.md#overview-encrypted-resources-and-fields) lists the protected fields.

Configure encryption for exported logs, storage volumes, and backups through the systems that store them. Obot's application encryption does not encrypt those stores as a whole.

Turning encryption on also does not encrypt all previously stored data automatically. An existing installation may need a separate data migration; account for that when planning the change.

## Understand what deletion and access changes remove

Some resources contain copies of other resources. Removing the source or changing access does not necessarily remove those copies.

| Action | Effect |
|---|---|
| Delete an MCP catalog entry | Existing vMCPs can keep using their saved snapshot of the entry. Review, update, or delete the affected vMCPs separately. See [vMCP snapshots and updates](../mcp-gateway/publish.md#virtual-mcps-snapshots-and-updates). |
| Remove a Git-managed vMCP definition from its catalog source | A successful sync deletes that vMCP. This differs from deleting a catalog entry used by an independently managed vMCP. See [Git-managed vMCPs](../mcp-gateway/publish.md#virtual-mcps-snapshots-and-updates). |
| Remove a skill source or a regular user's last access grant | The affected skills are no longer available to that user for discovery or installation through Obot. Copies already installed on clients remain. See [skill access](../registries/publish-skills.md#skills-access-control). |
| Change who can access a published workflow version | Access changes apply to that version. Other versions have their own access lists. See [workflow version access](../agents/workflows.md#workflow-sharing-access-and-subjects). |

## Back up the stores you need to recover

Persistent storage keeps files across workload replacement; backups let you recover from deletion, corruption, or loss of that storage. Back up the database and the file or object stores your installation uses, and retain access to any encryption keys needed to restore them. Follow [Backup and recovery](../operations/backup.md) to plan and test a restore.
