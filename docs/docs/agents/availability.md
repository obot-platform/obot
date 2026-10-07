---
draft: true
title: "Agents overview"
---

Agents use language models and tools to carry out tasks. Obot Agent provides a built-in conversation and workflow experience. External AI clients can use Obot's gateways while running elsewhere.

## Choose an agent experience

- **Obot Agent** provides conversations, projects, and workflows through the built-in UI. Start with [Run your first agent](./first-agent.md).
- **External clients** run on user devices or other infrastructure. Connect them through the [MCP](../mcp-gateway/connect-clients.md) or [LLM](../llm-gateway/connect-clients.md) gateway.

See [Configure agent credentials](./identity.md) for how each experience authenticates and [Configure agent runtimes](./runtime.md) for networking and storage.

## Availability and prerequisites

Enable and configure the agent experience you intend to use.

| Workload | Enablement | Runtime and use |
|---|---|---|
| Obot Agent | `OBOT_ENABLE_AGENTS=true`; existing agent deployments remain enabled when the setting is unset | Chat, conversations, skills, and workflows using the MCP runtime |
| External AI clients | No Obot Agent feature flag required | Run on user devices and connect to MCP/LLM gateways |

New installations do not enable Obot Agent by default. Set `OBOT_ENABLE_AGENTS=true` to enable it, and configure its model providers and MCP resources.

See [Server Configuration](../configuration/server-configuration.md), [Run your first agent](./first-agent.md), and [Runtime, networking, and persistence](./runtime.md). [Workflow sharing](./workflows.md) is part of Obot Agent.

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
