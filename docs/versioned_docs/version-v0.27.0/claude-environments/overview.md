---
title: Claude Environments (experimental)
description: Run Claude self-hosted sessions on Obot-managed Kubernetes infrastructure.
---

:::warning Experimental
Claude Environments is experimental and disabled by default. APIs, configuration, and snapshot compatibility can change. Evaluate it in a non-production cluster and keep work you need in source control. A supported Kubernetes version alone does not establish compatibility with this integration.
:::

Claude Environments lets an organization run Claude cloud sessions on its own Kubernetes infrastructure. Users chat in Claude, while Obot controls who can start a session, which vMCPs it can use, its network access, and its resource limits. Obot uses [Agent Substrate](https://github.com/agent-substrate/substrate) to run isolated actors and suspend idle workspaces.

This replaces the former Hosted Agents feature. There is no separate agent configuration or chat interface in Obot: administrators configure environments and profiles, and users select an environment in Claude.

## What you need

- Obot deployed in Kubernetes with a compatible cluster, snapshot bucket, and Substrate database. Docker deployments cannot run this feature.
- An administrator who can install cluster-scoped resources and configure cloud storage access.
- A Claude Team or Enterprise organization with self-hosted environments enabled by its Owner. Anthropic's feature is also in public beta and has its own eligibility restrictions; check [Anthropic's availability documentation](https://code.claude.com/docs/en/self-hosted-environments#availability-and-limitations).

Start with [cluster compatibility](./cluster-requirements.md), then [install the integration](./installation.md) and [configure an environment](./administration.md).

## How requests reach Obot

Each Obot replica runs an Anthropic orchestrator process for each configured environment. The orchestrator authenticates with the environment key and polls Anthropic over an outbound connection. Its spawn hook calls Obot locally when a work order arrives. Anthropic does not need an inbound connection to Obot, and Helm does not deploy a separate orchestrator pod.

Obot validates the session identity, resolves its Obot user, and checks environment profiles before provisioning. Substrate supplies worker pools and actors; the runner connects back to Claude. Model inference and session communication still reach Anthropic. Self-hosting does not make Claude an offline service.

A Claude session has one persistent Obot session record. Reconnects can create new executions and actors within that session while restoring its workspace snapshot. An actor is not a Kubernetes pod: multiple actors can share a worker pod.

## Access and visibility

- Administrators create environments, import account mappings, and manage all sessions.
- Users see environments granted through profiles, a read-only view of their tool and network access, and **My Claude Sessions**. Infrastructure settings and secrets are not shown in that read-only view.
- Administrators can inspect **All Claude Sessions**, including the associated Obot user, state, last-used time, repositories, and available Slack metadata.
- Egress logs appear in **Audit Logs → Claude Environments**. Payload access additionally requires the auditor role.

On Docker, the sidebar entry explains the Kubernetes requirement. On Kubernetes with the feature disabled, the entry is hidden. Egress audit navigation is hidden when the integration is unavailable.

See [operations and troubleshooting](./operations.md) for lifecycle and recovery, and [network access and auditing](./security.md) for the security boundaries.
