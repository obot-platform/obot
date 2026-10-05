---
title: "Agents overview"
---

Agents use language models and tools to carry out tasks. Obot provides a built-in conversation and workflow experience and can also host agent workloads in Kubernetes sandboxes. External AI clients can use Obot's gateways while running elsewhere.

## Choose an agent experience

- **Obot Agent** provides conversations, projects, and workflows through the built-in UI. Start with [Run your first agent](./first-agent.md).
- **Hosted Agents** run from agent templates in managed sandboxes. Configure the workload's resources, credentials, and runtime before launching it.
- **External clients** run on user devices or other infrastructure. Connect them through the [MCP](../mcp-gateway/connect-clients.md) or [LLM](../llm-gateway/connect-clients.md) gateway.

See [Configure agent credentials](./identity.md) for how each experience authenticates and [Configure agent runtimes](./runtime.md) for networking and storage.

## Availability and prerequisites

Enable and configure the agent experience you intend to use.

| Workload | Enablement | Runtime and use |
|---|---|---|
| Obot Agent | `OBOT_ENABLE_AGENTS=true`; existing agent deployments remain enabled when the setting is unset | Chat, conversations, skills, and workflows using the MCP runtime |
| Hosted Agents | `OBOT_SERVER_ENABLE_HOSTED_AGENTS=true` | Agent templates launched into sandboxes; the production backend is Kubernetes |
| External AI clients | No hosted-agent feature flag required | Run on user devices and connect to MCP/LLM gateways |

New installations do not enable Obot Agent or Hosted Agents by default. The Hosted Agents backend follows the MCP runtime when unset: Kubernetes uses Kubernetes; other runtimes use the development `fake` backend. **There is no Docker Hosted Agents backend.** Enabling the UI on Docker does not create a production sandbox runtime.

See [Server Configuration](../configuration/server-configuration.md), [Run your first agent](./first-agent.md), and [Runtime, networking, and persistence](./runtime.md). [Workflow sharing](./workflows.md) belongs to Obot Agent, not every external or hosted harness.

Obot Agent is a chat interface built to work directly with MCP. It provides a conversational way for users to interact with MCP servers and accomplish tasks using AI.

## Key Concepts {#obot-agent-key-concepts}

### Conversations {#obot-agent-conversations}

Conversations provide isolated message history while sharing the agent's configuration and resources.

### Workflows {#obot-agent-workflows}

Workflows automate interactions through scheduled or on-demand execution. They can run on recurring schedules or be triggered manually.

### Model Providers {#obot-agent-model-providers}

Obot Agent supports multiple LLM providers including OpenAI, Anthropic, Azure OpenAI, and Amazon Bedrock. Model providers are configured at the platform level and made available to users.

## Learn More {#obot-agent-learn-more}

- [Obot Agent Management](./first-agent.md) - Configure default agent, conversation, and workflow settings
