# Documentation navigation audit

Discussion draft, 2026-11-13, based on upstream `main` at `819ec1711`.
This file is a maintainer note outside the published documentation tree.
Placements below are proposals; the sidebar and published pages are unchanged.

## Findings

The current documentation has 112 Markdown pages. The sidebar exposes 79 unique
documents, counting both category landing pages and string-form entries. The
remaining 33 pages comprise:

- 13 substantive guides or reference pages accessible through links in other pages.
- 3 specialized reference or migration pages accessible through links in other pages.
- 17 compatibility or routing pages, including 14 with no incoming documentation links.

All 14 pages without a path from the sidebar are compatibility pages whose
replacements are in the navigation. The main problem is discoverability of
specialized material, rather than missing routes to substantive current content.

For example, [Product Analytics](./docs/configuration/product-analytics.md) is
absent from the sidebar, but linked from Network Requirements, Server
Configuration, and Audit Data and Retention. Someone looking for the consent
setting must already know which of those pages to open.

The October 6 refactor (`86994d68a`, #8134) replaced the former Features,
Installation, and Configuration and Operations organization. Product Analytics,
storage, MCP deployment settings, image pull secrets, egress control, static
OAuth, Registry API, the encryption providers, and FAQ all appeared directly in
the sidebar before that refactor. Other omissions represent intentional content
consolidation and preserved bookmarks.

## Recommended first pass

| Page | Proposed placement | Reason |
| --- | --- | --- |
| [Product Analytics](./docs/configuration/product-analytics.md) | **Deploy & Operate**, immediately after Monitoring | An installation-wide consent and configuration decision. Keep the existing cross-link from Security & Governance → Audit data and retention for readers asking what leaves the installation. Label: **Product analytics and consent**. |
| [MCP Server OAuth Configuration](./docs/configuration/mcp-server-oauth-configuration.md) | **MCP Gateway → MCP server configuration**, before the Slack and Vercel tutorials | Shared setup required by remote providers. Label: **Configure static OAuth**. First review its older project/connection wording against the current vMCP flow. |
| [MCP Server Egress Control](./docs/configuration/mcp-server-egress-control.md) | **Security & Governance**, immediately after Isolate workloads | A distinct enforcement control with its own infrastructure prerequisites. Label: **Restrict MCP server egress**. Retain the links from network and deployment guides. |
| [MCP Registry API](./docs/functionality/mcp-registry-api.md) | **Reference**, immediately after CLI and APIs | A protocol/API reference that integrators may seek directly. Label: **MCP Registry API**. Keep conceptual links in Registries & Skills. |

These four additions address distinct tasks without adding a new top-level
category. The existing MCP server configuration subgroup can hold static OAuth;
the current navigation tests explicitly constrain nested groups, so any further
subgroups should be deliberate.

## Other substantive pages missing from the sidebar

These pages are reachable today. The choices below distinguish pages worth
promoting from detail pages that fit naturally under an existing guide.

| Page | Existing route in the documentation | Suggested fit |
| --- | --- | --- |
| [Persistent Storage in Kubernetes](./docs/installation/kubernetes-persistent-storage.md) | Kubernetes deployment, Backup and recovery, Helm values, Data storage and lifecycle | Strong second-pass candidate for **Deploy & Operate**, after Kubernetes (production), labeled **Configure persistent storage**. |
| [MCP Servers in Kubernetes](./docs/configuration/mcp-deployments-in-kubernetes.md) | Add hosted MCP servers, Kubernetes deployment, Capacity, Isolate workloads | Strong second-pass candidate for **Deploy & Operate**, labeled **Configure the MCP runtime**. It configures hosted workloads rather than the Obot control-plane deployment. |
| [Image Pull Secrets](./docs/configuration/image-pull-secrets.md) | Encryption and secrets, Helm values, Troubleshooting | Keep as a detail guide initially; add a prominent related-guide link from **MCP Gateway → Add hosted MCP servers**. If private registries are a frequent setup task, promote it beside the MCP runtime guide under **Deploy & Operate**, labeled **Use private container registries**. |
| [AWS KMS](./docs/configuration/encryption-providers/aws-kms.md) | Encryption and secrets; Amazon EKS | Keep the provider list in **Security & Governance → Encryption and secrets** as the entry point. Make provider setup easy to scan before adding four sidebar entries. |
| [Azure Key Vault](./docs/configuration/encryption-providers/azure-key-vault.md) | Encryption and secrets; Azure AKS | Same treatment as AWS KMS. |
| [Google Cloud KMS](./docs/configuration/encryption-providers/google-cloud-kms.md) | Encryption and secrets; Google GKE | Same treatment as AWS KMS. |
| [Custom Encryption Provider](./docs/configuration/encryption-providers/custom-provider.md) | Encryption and secrets | Same treatment as the cloud providers; retain the warning about existing-data migration. |
| [FAQ](./docs/faq.md) | Deploy & Operate overview; MCP troubleshooting | Optionally place **FAQ** at the end of **Start Here**. Review its older deployment and connection advice before promoting it. Alternatively move individual answers into the relevant troubleshooting pages while preserving anchors. |
| [AWS / Azure / GCP](./docs/operations/cloud.md) | Architecture → Where Obot and MCP servers run | Keep as a short comparison page linked from **Deploy & Operate → Overview**. Its individual deployment guides already appear under Cloud deployments. |

Together with the four first-pass recommendations, these are the 13 substantive
pages missing from the current sidebar.

## Specialized material worth keeping discoverable

| Page | Current entry points | Recommendation |
| --- | --- | --- |
| [Audit Log Exports](./docs/configuration/audit-log-export.md) | Audit data and retention, LLM audit logs, Server configuration | Retain the detailed storage-provider and credential reference behind **Security & Governance → Audit data and retention**. If exports are a frequent task, add **Export audit logs** immediately after that item. This page retains specialized content even though its opening mentions preserved bookmarks. |
| [Agent Authorization Scopes](./docs/functionality/agent-auth-scopes.md) | Encryption and secrets, Control MCP access, client connection guides, CLI and APIs | Retain as the API-key reference behind those guides. If promoting it, use **Security & Governance → Agent identities and API keys**; review overlap with Encryption and secrets first. |
| [AI Judge Policies](./docs/functionality/ai-judge-policies.md) | Guardrails; Upgrading from v0.27 | Keep as an upgrade/migration reference linked from **LLM Gateway → Guardrails** and the upgrade guide. A standalone current-feature entry would obscure the migration to guardrails. |

## Compatibility pages to leave out of the sidebar

These 14 have no incoming Markdown links from the current documentation. Each
preserves an old URL or section anchor and points to current guides. Their
absence from the sidebar is expected.

| Preserved page | Current destination already in navigation |
| --- | --- |
| [User Roles](./docs/configuration/user-roles.md) | Security & Governance → Roles and access policies |
| [Audit Logs & Usage](./docs/functionality/audit-logs-and-usage.md) | Security & Governance → Audit data and retention |
| [Device Management](./docs/functionality/device-management.md) | Device Management (Sentry) → Overview, Enroll devices, Inspect device inventory, Policy enforcement |
| [LLM Gateway](./docs/functionality/llm-gateway.md) | LLM Gateway → Overview, provider setup, client setup, API compatibility, upgrade guide |
| [MCP Access Policies](./docs/functionality/mcp-access-policies.md) | MCP Gateway → Control MCP access |
| [Features Overview](./docs/functionality/overview.md) | Start Here → Explore Obot |
| [Server Scheduling](./docs/functionality/server-scheduling.md) | Deploy & Operate → Capacity |
| [Skill Access Policies](./docs/functionality/skill-access-policies.md) | Registries & Skills → Control skill access |
| [Skills](./docs/functionality/skills.md) | Registries & Skills → Publish skills |
| [User Management](./docs/functionality/user-management.md) | Security & Governance → Roles and access policies |
| [Virtual MCPs](./docs/functionality/virtual-mcps.md) | MCP Gateway → Create vMCP, Share vMCPs, Control MCP access |
| [CLI Setup](./docs/installation/cli-setup.md) | Reference → CLI and APIs |
| [Enabling Authentication](./docs/installation/enabling-authentication.md) | Deploy & Operate → Configure authentication providers |
| [Catalog and Filter Schemas](./docs/reference/schemas.md) | Reference → Catalog schemas; Filter schemas |

Three more compatibility pages still receive links:

- [Encryption Overview](./docs/configuration/encryption-providers/overview.md)
  points to Encryption and secrets. Network Requirements still links to this
  older overview; update that link to the current guide in a later cleanup.
- [MCP Servers](./docs/functionality/mcp-servers.md) points to the current hosted,
  remote, vMCP, configuration, and access guides. Update incoming links to the
  appropriate task and anchor as those pages are edited.
- [Filters and Private-Network Tunnels](./docs/mcp-gateway/filters-tunnels.md)
  points to Filter MCP traffic and Connect private-network servers. Link directly
  to those guides from Add remote MCP servers and Architecture → Where Obot and
  MCP servers run.

Keep these files and explicit anchors working for bookmarks. Restoring them to
the sidebar would duplicate the current task-based guides.

## Released versions

| Version | Documents | Unique sidebar documents | Absent from sidebar |
| --- | ---: | ---: | ---: |
| Current (`/next/`) | 112 | 79 | 33 |
| v0.28.0 (configured latest, site root) | 112 | 79 | 33 |
| v0.27.0 | 104 | 71 | 33 |
| v0.26.0 | 55 | 55 | 0 |
| v0.25.0 | 52 | 52 | 0 |

Current and v0.28.0 omit exactly the same page paths. v0.27.0 also omits the
[Slack tutorial](./versioned_docs/version-v0.27.0/configuration/tutorials/slack-mcp-server.md),
which was restored to current navigation later; its AI Judge Policies page is
still in the sidebar. The other 32 omissions match current.

Start implementation in current docs. Per the docs README, release snapshots
receive corrections only when needed to avoid misleading users of that release.
Consider a separate v0.28.0 navigation correction for the agreed high-priority
entries, since changing `sidebars.ts` alone only affects `/next/`. Treat v0.27.0
and its Slack entry separately; do not copy current feature assumptions into it.

## Method and limits

Compared every `.md`/`.mdx` page under `docs/docs` with the explicit sidebar,
including category document links and shorthand string entries. Normalized
Docusaurus numeric filename prefixes for the cloud architecture guides. Checked
relative Markdown links outside fenced code, walked those links from the sidebar
documents, and inspected every omitted page to distinguish substantive material
from compatibility pages. Repeated sidebar coverage counts for all retained
release snapshots and compared omitted paths for current, v0.28.0, and v0.27.0.

No current page is marked draft or unlisted. There are no site-root Markdown
document links in current content; the site-root links found are image assets.
This is a source navigation audit, not a browser crawl, a content-accuracy review,
or a claim about search-engine discovery. Published deployment state can differ
from the configured latest version in this checkout.

For an implementation, validate relative links and preserved anchors, run the
docs verification and navigation suite, and check the rendered sidebar on both
`/next/` and the latest release if a backport is included. In particular, current
browser tests verify existing sidebar links but do not require every substantive
document to have a discoverable entry point.
