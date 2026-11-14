# Documentation navigation audit

Discussion draft, 2026-11-13, based on upstream `main` at `819ec1711`.
This file is a maintainer note outside the published documentation tree.
Revised after discussion: improve existing topic pages without expanding the sidebar.
Product Analytics is linked from an expanded consent section in Audit data and
retention, with its disclosure checked against the telemetry payload. Static OAuth
is consolidated into MCP server configuration with temporary redirects from its
old page. MCP server egress control is now a direct child of Deploy & Operate,
with a cross-link from the catalog page. These changes cover current and v0.28.0
docs; other consolidation suggestions below remain proposals.

## Findings

At the initial audit, current documentation had 112 Markdown pages. The sidebar exposed 79 unique
documents, counting both category landing pages and string-form entries. The
remaining 33 pages comprise:

- 13 substantive guides or reference pages accessible through links in other pages.
- 4 specialized reference or migration pages accessible through links in other pages.
- 16 compatibility or routing pages, including 14 with no incoming documentation links.

Correction after closer redirect review: MCP Servers is a mixed page containing
legacy deployment guidance as well as forwarding sections. It should not have
been counted as a pure stub. There are 16 pure stubs and 17 omitted pages that
retain substantive guides, reference, or migration content.

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

Use the existing topic pages to make the retained material discoverable. Avoid
adding sidebar entries solely because an older page is absent from navigation.

| Topic | Existing page to develop | Proposed treatment |
| --- | --- | --- |
| [Product Analytics](./docs/configuration/product-analytics.md) | **Security & Governance → Audit data and retention** | Added a consent/privacy summary here, linked to the retained detailed disclosure. Verified all six installation fields, fourteen metrics, and three built-in-server subfields; corrected the claim that server display names are sent. No new sidebar entry. |
| [MCP Server OAuth Configuration](./docs/mcp-gateway/server-configuration.md#static-oauth) | **MCP Gateway → MCP server configuration** | Consolidated setup, credential status, rotation, and availability here. The old URL now redirects to this section in current and v0.28.0 docs. No new sidebar entry. v0.27.0 retains its original reference because it has no shared configuration page. |
| [MCP Server Egress Control](./docs/configuration/mcp-server-egress-control.md) | **Deploy & Operate → MCP server egress control** | Added a direct sidebar entry after Network requirements. Linked the catalog page's per-entry `egressDomains` and `denyAllEgress` settings to the cluster prerequisites and YAML examples. Remote endpoints are outside this control. |
| [MCP Registry API](./docs/functionality/mcp-registry-api.md) | **MCP Gateway → vMCP overview**, with the existing discovery link from **Registries & Skills → MCP Servers** | Explain programmatic discovery alongside the gateway's other capabilities and link to the API details. Keep Reference → CLI and APIs as a secondary entry point. |

Product Analytics, Static OAuth, and egress placement are implemented in this
pass. The other fourteen substantive pages already have entry points within
existing guides and remain for review. OAuth uses the agreed page-level 302
behavior; old section fragments are not individually mapped.

## Other substantive pages missing from the sidebar

These pages are reachable today. The suggestions below improve entry points from existing guides without adding
sidebar entries.

| Page | Existing route in the documentation | Suggested fit |
| --- | --- | --- |
| [Persistent Storage in Kubernetes](./docs/installation/kubernetes-persistent-storage.md) | Kubernetes deployment, Backup and recovery, Helm values, Data storage and lifecycle | Keep the detailed guide behind **Deploy & Operate → Kubernetes (production)** and make its storage prerequisite link prominent. |
| [MCP Servers in Kubernetes](./docs/configuration/mcp-deployments-in-kubernetes.md) | Add hosted MCP servers, Kubernetes deployment, Capacity, Isolate workloads | Use **MCP Gateway → Add hosted MCP servers** as the primary entry point, with a cross-link from Kubernetes deployment. It configures hosted workloads rather than the Obot control-plane deployment. |
| [Image Pull Secrets](./docs/configuration/image-pull-secrets.md) | Encryption and secrets, Helm values, Troubleshooting | Keep as a detail guide initially; add a prominent related-guide link from **MCP Gateway → Add hosted MCP servers**. Describe the private registry use case in that existing guide. |
| [AWS KMS](./docs/configuration/encryption-providers/aws-kms.md) | Encryption and secrets; Amazon EKS | Keep the provider list in **Security & Governance → Encryption and secrets** as the entry point. Make the provider setup links easy to scan within that page. |
| [Azure Key Vault](./docs/configuration/encryption-providers/azure-key-vault.md) | Encryption and secrets; Azure AKS | Same treatment as AWS KMS. |
| [Google Cloud KMS](./docs/configuration/encryption-providers/google-cloud-kms.md) | Encryption and secrets; Google GKE | Same treatment as AWS KMS. |
| [Custom Encryption Provider](./docs/configuration/encryption-providers/custom-provider.md) | Encryption and secrets | Same treatment as the cloud providers; retain the warning about existing-data migration. |
| [FAQ](./docs/faq.md) | Deploy & Operate overview; MCP troubleshooting | Move useful answers into the relevant existing onboarding and troubleshooting pages while preserving old anchors. Review older deployment and connection advice before reusing it. |
| [AWS / Azure / GCP](./docs/operations/cloud.md) | Architecture → Where Obot and MCP servers run | Keep as a short comparison page linked from **Deploy & Operate → Overview**. Its individual deployment guides already appear under Cloud deployments. |

Together with the four topics above, these are the 13 substantive pages missing
from the current sidebar.

## Specialized material worth keeping discoverable

| Page | Current entry points | Recommendation |
| --- | --- | --- |
| [Audit Log Exports](./docs/configuration/audit-log-export.md) | Audit data and retention, LLM audit logs, Server configuration | Retain the detailed storage-provider and credential reference behind **Security & Governance → Audit data and retention**. Make the export setup link prominent within the existing audit page. This page retains specialized content even though its opening mentions preserved bookmarks. |
| [Agent Authorization Scopes](./docs/functionality/agent-auth-scopes.md) | Encryption and secrets, Control MCP access, client connection guides, CLI and APIs | Retain as the API-key reference behind those guides. Review overlap with Encryption and secrets before moving additional content into that existing page. |
| [AI Judge Policies](./docs/functionality/ai-judge-policies.md) | Guardrails; Upgrading from v0.27 | Keep as an upgrade/migration reference linked from **LLM Gateway → Guardrails** and the upgrade guide. A standalone current-feature entry would obscure the migration to guardrails. |

The [MCP Servers](./docs/functionality/mcp-servers.md) page also retains legacy
single-user, multi-user, and catalog-entry guidance. Keep this material available
until it is deliberately relocated; redirecting the whole page would hide it.

## Compatibility pages to leave out of the sidebar

These 14 have no incoming Markdown links from the current documentation. Each
preserves an old URL or section anchor and points to current guides. Their
absence from the sidebar is expected.

| Preserved page | Current destination already in navigation |
| --- | --- |
| [User Roles](./docs/security/policy-coverage.md) | Security & Governance → Roles and access policies |
| [Audit Logs & Usage](./docs/security/audit-data.md) | Security & Governance → Audit data and retention |
| [Device Management](./docs/device-management/how-sentry-works.md) | Device Management (Sentry) → Overview, Enroll devices, Inspect device inventory, Policy enforcement |
| [LLM Gateway](./docs/llm-gateway/how-it-works.md) | LLM Gateway → Overview, provider setup, client setup, API compatibility, upgrade guide |
| [MCP Access Policies](./docs/mcp-gateway/access.md) | MCP Gateway → Control MCP access |
| [Features Overview](./docs/security/model.md) | Security & Governance → Overview |
| [Server Scheduling](./docs/operations/capacity.md) | Deploy & Operate → Capacity |
| [Skill Access Policies](./docs/registries/publish-skills.md) | Registries & Skills → Control skill access |
| [Skills](./docs/registries/publish-skills.md) | Registries & Skills → Publish skills |
| [User Management](./docs/security/authentication.md) | Security & Governance → Roles and access policies |
| [Virtual MCPs](./docs/mcp-gateway/server-types.md) | MCP Gateway → Create vMCP, Share vMCPs, Control MCP access |
| [CLI Setup](./docs/reference/cli-api.md) | Reference → CLI and APIs |
| [Enabling Authentication](./docs/security/authentication.md) | Deploy & Operate → Configure authentication providers |
| [Catalog and Filter Schemas](./docs/reference/catalog-schemas.md) | Reference → Catalog schemas; Filter schemas |

Two more compatibility pages still receive links:

- [Encryption Overview](./docs/security/credentials.md)
  points to Encryption and secrets. Network Requirements still links to this
  older overview at the initial audit; this now links directly to the current guide.
- [Filters and Private-Network Tunnels](./docs/functionality/filters.md)
  points to Filter MCP traffic and Connect private-network servers. Link directly
  to those guides from Add remote MCP servers and Architecture → Where Obot and
  MCP servers run; these links have now been updated.

The original 16 pure stubs have temporary Cloudflare Pages 302 redirects in
`static/_redirects`, covering the latest release at the root (v0.28.0), `/next/`,
and v0.27.0, with and without trailing slashes. Old section fragments are
intentionally not remapped, as agreed in discussion. The redirects use each
guide's main replacement; authentication setup goes to Configure authentication
providers, and the combined schemas page goes to Catalog schemas; Filter schemas
remains in the Reference navigation. MCP Servers remains available because it retains legacy
guidance. The 50 redundant Markdown copies (17 current, 17 v0.28.0, and 16
v0.27.0) have now been deleted. Internal links were updated, and the Docusaurus
client-redirect plugin generates static HTML fallbacks from the same Cloudflare
rules. Deleted guides no longer appear in Markdown exports or agent indexes. The additionally
consolidated OAuth page has equivalent 302 rules for current and v0.28.0 only,
pointing directly to the new Static OAuth section.

## Released versions at the initial audit

| Version | Documents | Unique sidebar documents | Absent from sidebar |
| --- | ---: | ---: | ---: |
| Current (`/next/`) | 112 | 79 | 33 |
| v0.28.0 (configured latest, site root) | 112 | 79 | 33 |
| v0.27.0 | 104 | 71 | 33 |
| v0.26.0 | 55 | 55 | 0 |
| v0.25.0 | 52 | 52 | 0 |

At the initial audit, current and v0.28.0 omitted exactly the same page paths. v0.27.0 also omitted the
[Slack tutorial](./versioned_docs/version-v0.27.0/configuration/tutorials/slack-mcp-server.md),
which was restored to current navigation later; its AI Judge Policies page is
still in the sidebar. The other 32 omissions matched current. Adding egress
control increased current and v0.28.0 sidebar coverage to 80 documents. After
deleting the 17 stub files from each, those versions now contain 95 documents,
with 15 substantive pages outside the sidebar.
Product Analytics is one of those 15 and now has an expanded entry point in the
existing audit guide; the other 14 remain for review.

Start implementation in current docs. Per the docs README, release snapshots
receive corrections only when needed to avoid misleading users of that release.
Consider a separate v0.28.0 correction for any agreed improvements to existing
topic pages, since changing current docs alone only affects `/next/`. Treat v0.27.0
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
