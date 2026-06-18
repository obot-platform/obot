---
title: Obot Agent
---

# Obot Agent

Obot Agent is a chat interface built to work directly with MCP. It provides a conversational way for users to interact with MCP servers and accomplish tasks using AI.

:::note
Obot Agent features are disabled by default for new deployments. To enable them, set the `OBOT_ENABLE_AGENTS=true` environment variable on the server. Deployments that already had agents before upgrading remain enabled automatically. See the [configuration reference](../configuration/server-configuration.md) for details.
:::

## Key Concepts

### Conversations

Conversations provide isolated message history while sharing the agent's configuration and resources.

### Workflows

Workflows automate interactions through scheduled or on-demand execution. They can run on recurring schedules or be triggered manually.

### Model Providers

Obot Agent supports multiple LLM providers including OpenAI, Anthropic, Azure OpenAI, and Amazon Bedrock. Model providers are configured at the platform level and made available to users.

## Learn More

- [Obot Agent Management](../functionality/obot-agent-management.md) - Configure default agent, conversation, and workflow settings
