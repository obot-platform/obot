---
draft: true
title: Run your first agent
---

This walkthrough uses **Obot Agent**, the built-in chat and workflow experience. Check its [availability requirements](./availability.md#availability-and-prerequisites) before you begin.

## Prerequisites

An operator must enable `OBOT_ENABLE_AGENTS=true` and restart Obot. An administrator must configure a [model provider](../configuration/model-providers.md) and a [model access policy](../functionality/model-access-policies.md) for your user. If the agent needs MCP tools, grant access to those resources as well.

## Start a conversation

1. Sign in and open the Obot Agent experience at your instance's `/agent` route.
2. Obot loads your project and agent, creating the initial project and agent when you have none.
3. Choose an available model. Start with a simple prompt that does not call external tools, such as asking for a short summary of text you provide.
4. Confirm that a response arrives. If no model is available, review the provider configuration and your model access policy.
5. Enable an approved MCP server for the agent and try a harmless read-only request. Complete required upstream authentication before expecting tools to work.

Review the conversation and usage before adding scheduled work. Configure [persistent storage](../installation/kubernetes-persistent-storage.md) before relying on files surviving workload replacement. For reusable automation, continue to [Workflows and scheduling](./workflows.md).

## Review the first run

After the first conversation, an administrator should review usage and policy behavior. These controls are separate from agent enablement.

## Token Usage {#obot-agent-management-token-usage}

Review [usage and costs](../llm-gateway/audit.md) across users and models. Confirm the expected provider and model handled the request.

:::note
Token counts are reported for the Azure API key provider. Obot estimates spend when a deployment name exactly matches a model in its pricing catalog; deployments with different names cannot be reliably mapped to the underlying model and do not have estimated spend.
:::

## Model Providers {#obot-agent-management-model-providers}

Configure LLM providers and their available models. See [Model Providers](../configuration/model-providers.md) for setup details.

## Model Access Policies {#obot-agent-management-model-access-policies}

Control which users and groups can access which models in Obot Agent. See [Model Access Policies](../functionality/model-access-policies.md) for details.

## AI Judge Policies {#obot-agent-management-message-policies}

Use natural language to enforce content rules on user prompts and tool calls. See [AI Judge Policies](../functionality/ai-judge-policies.md) for details.

## AI Judge Policy Violations {#obot-agent-management-message-policy-violations}

Review policy violations, trends, and blocked content metadata using **Policy Violations** within the **AI Judge Policies** tab. See [AI Judge Policies](../functionality/ai-judge-policies.md) for configuration and coverage.
