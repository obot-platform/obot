---
displayed_sidebar: sidebar
title: Product Analytics
---

# Product Analytics

When enabled, Obot reports aggregate product usage 30 minutes after startup and then once per day, within the first ten minutes after midnight UTC. These reports help the Obot team
understand which capabilities are used and prioritize product improvements. Product analytics
applies to the installation as a whole, not to an individual user.

Obot does not include prompts, messages, credentials, URLs, custom MCP server configuration details, or audit-log content in product-analytics reports.

## Consent

For self-hosted installations, Owners and Admins can choose whether to share product usage data in the **Welcome to Obot** dialog
and change their choice at any time from **Platform > Product Analytics**. No product analytics data
is collected without explicit consent.

Set `OBOT_SERVER_PRODUCT_ANALYTICS_MODE` to control analytics for the installation:

- `consent` (default): Owners and Admins choose whether to share usage data.
- `on`: Always share usage data and hide the consent prompt and settings controls.
- `off`: Never share usage data and hide the consent prompt and settings controls.

Obot Cloud deployments are always opted in to product analytics.

## Data included

Every report contains these operational fields. The JSON field names below identify the values sent to Obot's upgrade service:

| Field | JSON field | Definition |
| --- | --- | --- |
| Installation ID | `installationID` | A stable identifier for the Obot installation. |
| License machine ID | `licenseMachineID` | The installation's persisted license machine identifier. |
| Report timestamp | `reportedAt` | The UTC time at which the report was produced. |
| Distribution | `distribution` | The installation category: `Unregistered`, `Registered`, `Enterprise`, or `Cloud`. |
| Engine | `engine` | The normalized runtime engine used by the installation. |
| Current version | `currentVersion` | The running Obot version. |

The report's `metrics` object contains the aggregate usage fields below. An unavailable metric is sent as `null`; a measured count of zero is sent as `0`.

| Field | JSON field within `metrics` | Definition |
| --- | --- | --- |
| Total users | `totalUsers` | Snapshot count of users known to the installation. |
| Active users | `activeUsers` | Distinct users active during the previous complete UTC day. |
| Deployed MCP servers | `deployedMCPServers` | Snapshot count of deployed MCP servers. |
| Custom MCP entries | `customMCPServerEntryCount` | Snapshot count of MCP catalog entries that are not built in. |
| Custom vMCPs | `customVMCPCount` | Snapshot count of vMCPs explicitly created by a person. Edited catalog defaults are not included. |
| vMCP instances | `vmcpInstanceCount` | Snapshot count of user connection instances. |
| Built-in MCP servers | `builtInMCPServers` | Per-server aggregates for built-in catalog entries with a nonzero deployment or user count. The fields in each entry are listed below. |
| Authentication-provider type | `authProviderType` | The configured authentication-provider type, without provider configuration values. An empty string means no configured provider was found. |
| MCP tool-call count | `mcpToolCallCount` | Number of MCP tool calls during the previous complete UTC day. |
| vMCP tool-call count | `vmcpToolCallCount` | Number of vMCP tool calls during the previous complete UTC day. These calls are also included in the MCP tool-call count. |
| LLM audit-log count | `llmAuditLogCount` | Number of LLM audit-log records created during the previous complete UTC day. Audit-log content is not included. |
| Sentry scan count | `sentryScanCount` | Number of Obot Sentry device scans recorded during the previous complete UTC day. |
| Sentry enforcement-event count | `sentryEnforcementEventCount` | Number of Obot Sentry enforcement events recorded during the previous complete UTC day. |
| Managed skill count | `managedSkillCount` | Snapshot count of managed skills. |

Each entry in `builtInMCPServers` contains:

| JSON field | Definition |
| --- | --- |
| `id` | The built-in catalog entry key, or its resource name when no entry key is set. A separate server display name is not sent. |
| `deployments` | Number of deployed servers associated with the built-in catalog entry. |
| `users` | The built-in catalog entry's user count. Individual user identities are not included. |

When built-in usage was measured but no entries have deployments or users, `builtInMCPServers` is an empty array.

## Upgrade checks are separate

The software upgrade check is independent of product-analytics consent. It sends the installation ID
and current version 30 minutes after startup and then every 24 hours, even when analytics consent is undecided or
disabled. Set `OBOT_SERVER_DISABLE_UPDATE_CHECK=true` to disable upgrade checks.
