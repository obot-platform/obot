---
title: Product Analytics
---

# Product Analytics

When enabled, Obot reports aggregate product usage once per day. These reports help the Obot team
understand which capabilities are used and prioritize product improvements. Product analytics
applies to the installation as a whole, not to an individual user.

Obot does not include prompts, messages, credentials, URLs, custom MCP server configuration details, or audit-log content in product-analytics reports.

## Consent

For self-hosted installations, Owners and Admins can choose whether to share product usage data in the **Welcome to Obot** dialog
and change their choice at any time from **Platform > Product Analytics**. No product analytics data
is collected without explicit consent.

Operators can override this choice with `OBOT_SERVER_PRODUCT_ANALYTICS_FORCE_ENABLED=true`, which
enables analytics and prevents users from opting out through Obot.

Obot Cloud deployments are always opted in to product analytics.

## Data included

Every report contains these operational fields:

| Field | Definition |
| --- | --- |
| Installation ID | A stable identifier for the Obot installation. |
| License machine ID | The installation's persisted license machine identifier. |
| Report timestamp | The time at which the report was produced. |
| Distribution | The installation category: `Unregistered`, `Registered`, `Enterprise`, or `Cloud`. |
| Engine | The normalized runtime engine used by the installation. |
| Current version | The running Obot version. |

When available, reports also contain these aggregate usage fields:

| Field | Definition |
| --- | --- |
| Total users | Snapshot count of users known to the installation. |
| Active users | Distinct users active during the previous complete UTC day. |
| Deployed MCP servers | Snapshot count of deployed MCP servers. |
| Custom MCP entries | Snapshot count of MCP catalog entries that are not built in. |
| Built-in MCP servers | Per-server aggregates containing the built-in server ID and name, plus its deployment count and distinct user count. |
| Authentication-provider type | The configured authentication-provider type, without provider configuration values. |
| MCP tool-call count | Number of MCP tool calls during the previous complete UTC day. |
| LLM audit-log count | Number of LLM audit-log records created during the previous complete UTC day. Audit-log content is not included. |
| Sentry scan count | Number of Obot Sentry device scans recorded during the previous complete UTC day. |
| Sentry enforcement-event count | Number of Obot Sentry enforcement events recorded during the previous complete UTC day. |
| Managed skill count | Snapshot count of managed skills. |

## Upgrade checks are separate

The software upgrade check is independent of product-analytics consent. It sends the installation ID
and current version once at startup and every 24 hours, even when analytics consent is undecided or
disabled. Set `OBOT_SERVER_DISABLE_UPDATE_CHECK=true` to disable upgrade checks.
