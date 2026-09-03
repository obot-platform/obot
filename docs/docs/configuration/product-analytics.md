---
title: Product Analytics
---

# Product Analytics

When enabled, Obot reports aggregate product usage once per day. These reports help the Obot team
understand which capabilities are used and prioritize product improvements. Product analytics
applies to the installation as a whole, not to an individual user.

Obot does not include prompts, messages, credentials, URLs, configuration values, or audit-log
content in product-analytics reports.

## Consent

An Owner or Admin is prompted to choose whether to share product analytics when an installation has
no recorded decision. Closing or dismissing the prompt does not grant or deny consent. The prompt
stays dismissed for the rest of that login session and appears again during a later login session if
no decision has been recorded.

Until an Owner or Admin explicitly opts in, Obot does not send a product-telemetry request. An
installation that opts out also sends no product-telemetry request.

Owners and Admins can change the installation's decision at any time from **Platform > Product
Analytics**.

An operator can force-enable product analytics by setting
`OBOT_SERVER_PRODUCT_ANALYTICS_FORCE_ENABLED=true`. In that mode analytics is enabled and the consent
prompt, navigation item, and editable setting are unavailable. Users cannot opt out through Obot.

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
