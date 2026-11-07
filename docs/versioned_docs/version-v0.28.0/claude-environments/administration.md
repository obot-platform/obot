---
title: Configure environments and users
description: Environment keys, profiles, account mappings, Claude Tag, and starting sessions.
---

:::warning Experimental
Claude Environments is experimental. Review effective access and test a new environment before sharing it with users.
:::

## Create an environment

1. A Claude organization Owner enables self-hosted environments and creates an environment in Claude's Cloud environments administration page. Copy its environment key and follow [Anthropic's setup instructions](https://code.claude.com/docs/en/self-hosted-environments).
2. In Obot, open **AI Resources → Claude Environments** and add an environment.
3. Enter a display name and the environment key. Obot derives the organization and environment IDs from the key; there are no separate ID fields to fill in.
4. Configure access profiles and select the shared vMCPs each profile grants. Selection is searchable, with selected vMCPs shown first.
5. Configure worker capacity and actor defaults, then save.

The key is write-only and stored in Obot's credential store. Configure [credential encryption](../security/credentials.md) before storing sensitive production credentials. If encryption is disabled, Obot still accepts the key and stores it without application-level encryption.

An environment record corresponds to one Claude environment and one worker pool. An organization can have multiple environments. Account mappings are shared by all environments with the same organization ID.

## Profiles and vMCP access

Profiles grant access to users and groups. Omitted profiles on API creation use the same Obot Admin default as vMCPs; an explicitly empty profile list grants nobody access. An admin role does not replace the need for profile access when launching a session.

For a user matching several profiles:

| Setting | Effective policy |
|---|---|
| vMCPs | Union of matching profiles. Each vMCP's own access policy also applies. |
| Empty vMCP list | A profile with no selected vMCPs grants no tools. |
| Actor CPU, memory, idle timeout, retention | Each empty field inherits its environment default; then take the maximum value across matching profiles. Disabled retention (`-1`) wins over finite values. |
| Additional egress hosts | Union of environment and matching-profile hosts. Profiles can expand network access. |
| Allow all web destinations | Enabled if the environment or any matching profile enables it. |
| Injected headers | A profile overrides the environment for a destination/header collision; the first matching profile in displayed order wins among profiles. |

For example, two matching profiles granting vMCP A and vMCP B provide both tools, if the user's vMCP policies permit them. Profiles requesting 1 CPU and 2 CPUs resolve to 2 CPUs. A single profile can lower an environment's actor default; the environment is the inheritance default, not an extra profile included in the maximum.

Disabled Obot users are denied access. Mapping a Claude account does not grant profile or vMCP access.

## Worker and actor configuration

| Setting | Default | Where configured |
|---|---|---|
| Worker replicas | 1 | Environment only |
| Worker CPU / memory | 2 CPUs / 4 GiB | Environment only; Kubernetes requests equal these limits |
| Worker image | Deployment worker image | Worker pool configuration |
| Worker node selector / tolerations | Deployment configuration | Shared by every environment; not editable through the UI or API |
| Actor CPU / memory | 1 CPU / 2 GiB | Environment and profile |
| Idle timeout | 5 minutes | Environment and profile; 1–10080 minutes |
| Session retention | 30 days | Environment and profile; `-1` disables cleanup |
| Runner image | Deployment runner digest | Actor **Advanced configuration**, environment only |
| Managed settings | Image defaults | Actor **Advanced configuration**, environment only |

Use Kubernetes resource quantities, such as `500m`, `1`, and `2Gi`. An actor's limits must fit a worker's available capacity. Adding replicas adds workers; it does not increase the size of each worker. Several actors can share a worker within Substrate's capacity constraints.

Worker settings are not profile overrides. Suspend sessions before changing worker images, selectors, or capacity in ways that may replace pods. Actor settings apply to new executions, including resumes. Updating deployment image defaults does not overwrite saved environments.

## Map Claude accounts to Obot users

Users do not link or unlink their own Claude accounts. An administrator uses **Manage mappings** for the organization:

1. Sign in to Claude with an organization administrator account.
2. Open the URL provided by Obot: `https://claude.ai/api/organizations/ORG_ID/members`.
3. Copy the complete JSON response into **Organization members JSON**.
4. Click **Preview mappings**. Review the email matches and choose a different Obot user where needed.
5. Click **Submit mappings** to save.

Expected input is a JSON array of members whose `account` includes `tagged_id`, `uuid`, `full_name`, and `email_address`. Importing this browser response does not require an Anthropic Admin API key. The administrator is responsible for the identities submitted.

Saved assignments take priority when previewing another import. Accounts without assignments are matched by case-insensitive email only when the match is unique. Previously unmapped accounts remain eligible for matching. Accounts omitted from an import are preserved. Each Obot user can map to one Claude account per organization.

### Just-in-time matching

When a human session request arrives, Obot can create a mapping using the account email supplied through the authenticated orchestrator. It requires exactly one active, non-service Obot user with that email who is not already mapped in that organization. Saved assignments take precedence; administrators can inspect and override automatically created mappings in the same UI.

**Not mapped is not a deny rule.** Clearing an assignment makes it eligible for later email matching. Use environment profiles, user status, or other access controls to deny access. Missing/ambiguous matches or absent profile access stop provisioning before a runner launches. Changing or removing assignments also cleans up affected workspaces and credentials; review the impact before submitting.

## Claude Tag from Slack or GitHub

Claude Tag is a service identity, not the Slack or GitHub sender's personal Obot identity. For each environment:

1. Create the Claude Tag service user in Obot's environment configuration.
2. Include that user in a matching environment profile and grant it the required vMCP access and tool credentials.
3. Enable Claude Tag and save.
4. Configure Claude's Slack/GitHub integration to route Tag sessions to this hosted environment, following Anthropic's current integration instructions.

The service user **must be covered by an environment profile**. Human mappings do not apply to it, and a human with an unknown account never falls back to this identity. Everyone able to trigger that environment's Claude Tag sessions uses the service user's access, not their own personal tool permissions.

Disabling Tag revokes its credentials and removes its workspaces. Deleting the environment also retires its service user. Available Slack sender/channel/thread metadata is displayed for context; it is not used for authorization. GitHub initiator metadata is not documented as captured by this implementation.

## Start and inspect a session

Choose the hosted environment in Claude when starting a cloud session. Obot's environment view also provides a copyable CLI command:

```nu
claude "Describe your task" --environment YOUR_CLAUDE_ENVIRONMENT_ID
```

Use the ID shown by Obot, not its internal database ID. Keep using Claude for chat. In Obot, use **My Claude Sessions** to view your sessions; administrators can use **All Claude Sessions** to inspect, suspend, or delete sessions.

Basic users can inspect allowed egress without seeing infrastructure or secrets. Environment cards show session totals, active sessions, and last usage. See [operations](./operations.md) for suspension, retention, and troubleshooting.
