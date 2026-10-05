---
title: "API compatibility"
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

## Limitations {#limitations}

- **Supported gateway providers.** External LLM Gateway clients can use OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Amazon Bedrock API key, Azure, and Azure Entra providers. Other configured providers such as Google Vertex are not exposed through provider-specific gateway routes yet.
- **Access is policy-bound.** You can only call models an administrator has granted you through a [Model Access Policy](../functionality/model-access-policies.md), and `/v1/models` returns only those models. A request for a model you don't have access to is rejected.
- **Send the exact model name.** Use the model name shown on the [Models page](../functionality/llm-gateway.md#the-models-page) exactly as displayed.
- **Azure model discovery is OpenAI-only.** The Azure `/v1/models` and `/openai/v1/models` routes return accessible `OpenAIResponses` deployments. Microsoft Foundry does not expose an Anthropic Models API, so pass `AnthropicMessages` deployment names explicitly.
- **Claude Code model discovery caveats.** Gateway model discovery is off by default and requires Claude Code v2.1.129 or later for the standard Anthropic gateway path. Claude Code's Bedrock Mantle mode may not populate the `/model` picker from Obot, so pass `--model` or select an enabled Mantle model manually.
- **OpenAI-compatible routes use the Responses API.** The OpenAI, Generic Responses Compatible, Bedrock, and Azure OpenAI-compatible routes support the Responses API (`/v1/responses`); the Chat Completions endpoint is not currently supported. Codex uses the Responses API by default.
- **Usage and policies still apply.** Requests count toward Obot [token usage](../functionality/audit-logs-and-usage.md) and, where configured, are subject to [AI Judge Policies](../functionality/ai-judge-policies.md).
- **Audit logs can be exported.** Administrators can create one-time or scheduled exports for LLM gateway audit logs. See [Audit Log Export](../configuration/audit-log-export.md).


## Related topics {#related-topics}

- [Model Providers](../configuration/model-providers.md) — configure supported providers and their models
- [Model Access Policies](../functionality/model-access-policies.md) — grant users access to specific models
- [Obot CLI Setup](../installation/cli-setup.md) — install and configure the `obot` CLI
- [Audit Logs and Usage](../functionality/audit-logs-and-usage.md) — monitor token usage
- [Claude Code: Route Mantle through a gateway](https://code.claude.com/docs/en/amazon-bedrock#route-mantle-through-a-gateway)
- [AWS Bedrock Anthropic model cards](https://docs.aws.amazon.com/bedrock/latest/userguide/model-cards-anthropic.html) — check Anthropic model region availability
- [Audit Log Export](../configuration/audit-log-export.md) — export LLM gateway audit logs
