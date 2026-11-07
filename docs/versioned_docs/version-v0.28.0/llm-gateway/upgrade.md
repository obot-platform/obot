---
title: "Upgrading from model providers"
---

:::warning Migration is not supported yet
Existing LLM gateway users should hold off on updating to v0.28.0. Migration from v0.27 model providers is not supported at this time, and existing provider credentials, models and access settings are not automatically migrated to the new gateway.

Migration support is planned for a future patch release. Wait for that release and its migration instructions before upgrading an existing installation. If you test v0.28.0, use a separate copy of your database and keep a backup for rollback.
:::

Earlier versions of Obot reached models through per-provider model providers (OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Azure and others), each with its own proxy route. In v0.28.0, the [LLM gateway](./how-it-works.md) is the only way Obot reaches models, and it can't be turned off.

:::note Development installations
Development installations that used the earlier embedded gateway must re-enter that gateway's provider configuration, keys, routing rules and settings. This does not provide migration from v0.27 model providers.
:::

## What happens on upgrade {#what-happens-on-upgrade}

Existing provider credentials, default model aliases and model access policies are not automatically migrated to the gateway.

Fresh installations can configure providers and access through the gateway.

## Client routes {#client-routes}

| Route | Status |
| --- | --- |
| `/api/llm-proxy/v1` | Serves every model in the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats. Use this for new setups. |
| `/api/llm-proxy/openai`, `/api/llm-proxy/anthropic` | Remain available through the gateway, but require configured gateway providers, models and access. Route compatibility does not migrate an existing setup. |
| `/api/llm-proxy/generic-responses`, `/api/llm-proxy/aws-bedrock`, `/api/llm-proxy/aws-bedrock-api-key`, `/api/llm-proxy/azure`, `/api/llm-proxy/azure-entra` | Removed. They return HTTP 410. Point these clients at `/api/llm-proxy/v1` and use gateway model names, such as `bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0`. |

Claude Code's Bedrock Mantle and Foundry modes pointed at the removed routes. Switch to the standard setup in [Using with Claude Code](./connect-clients.md#using-with-claude-code).

## Other changes {#other-changes}

- **Model access policies are profiles.** Each policy becomes a profile on the **Profiles** tab, with the same users, groups and models. Budgets and guardrail policies belong to profiles.
- **Daily token limits are replaced by budgets.** `--daily-user-input-token-limit` and `--daily-user-output-token-limit` only seed the *Daily token limit* budget when the gateway first starts. They default to 10,000,000 input and 100,000 output tokens per user per day, and a flag set below 0 leaves that limit out. A budget covers only its own profile's models, so every profile that exists at the upgrade gets its own copy of the budget, with administrators exempt and users' personal daily token limits still in its place. Every model a user could reach stays limited, but a user in several profiles gets an allowance from each of them. Edit or delete each copy on its profile's page; deleted copies don't come back, and profiles you create later don't get one. With no profiles at all, Obot creates the *Default Policy*, as on a first start, and gives it the budget. The `/api/users/{id}/remaining-token-usage` endpoint is removed.
- **The model provider configuration API is removed.** The configure, deconfigure, reveal and validate endpoints under `/api/model-providers` no longer exist. Listing and getting model providers still work, and return the gateway and any retained legacy providers.
- **The models.dev price sync is removed.** `--model-info-source-url` / `OBOT_SERVER_MODEL_INFO_SOURCE_URL` no longer exists. Model prices now come from [`OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL`](./audit.md#model-pricing).
- **Message policies are LLM judge guardrails.** They're converted into LLM Judge guardrails used by disabled guardrail policies for you to review and enable. A message policy that didn't apply to all users gets its own profile, named after the policy with *(message policy)* added, with the policy's users and groups and no models. `OBOT_SERVER_ENABLE_MESSAGE_POLICIES` is removed. Past message policy violations stay in the database but aren't shown in the UI or returned by the API, and the LLM audit log no longer marks or filters requests a message policy changed. See [AI Judge Policies](../functionality/ai-judge-policies.md).
