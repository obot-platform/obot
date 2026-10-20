---
title: "Upgrading from model providers"
---

Earlier versions of Obot reached models through per-provider model providers (OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Azure and others), each with its own proxy route. These are gone. The [LLM gateway](./how-it-works.md) is now the only way Obot reaches models, and it can't be turned off.

:::warning Back up first
The upgrade can't be undone. Once aliases and policies name gateway models and the old providers are deleted, an earlier version of Obot can't serve them. Back up Obot's database before upgrading, and restore it to roll back.
:::

:::note Postgres
When Obot uses Postgres, the gateway keeps its configuration in a separate `<database>_llm_gateway` database on the same server, which Obot creates on first start. If Obot's database user can't create databases (common with managed Postgres), create it before upgrading, owned by that user (`CREATE DATABASE obot_llm_gateway OWNER obot;` for a database and user named `obot`), or set `OBOT_SERVER_LLM_GATEWAY_POSTGRES_DSN` to a database for the gateway. If Obot can't create the database, the gateway doesn't start, and model requests fail with an error that says how to create it. Create the database, owned by Obot's database user, or set `OBOT_SERVER_LLM_GATEWAY_POSTGRES_DSN`, then restart Obot.
:::

## What happens on upgrade {#what-happens-on-upgrade}

The first time the new version starts, Obot moves your configuration to the gateway once:

1. **Credentials are imported.** The credentials of each configured model provider become a gateway provider:

   | Model provider | Gateway provider |
   | --- | --- |
   | OpenAI | `openai` |
   | Anthropic | `anthropic` |
   | Generic Responses Compatible | Custom provider `generic-responses`, with its base URL's trailing `/v1` removed, since the gateway adds it |
   | Amazon Bedrock, Amazon Bedrock API key | `bedrock` |
   | Azure, Azure Entra | `azure` |

   When both variants of Bedrock or Azure are configured, the second becomes another key of the same gateway provider. A gateway provider that already exists is left alone. When the server's `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` environment variable is set, the OpenAI or Anthropic credential isn't imported: the provider gets only the key from the variable (see [OpenAI and Anthropic keys at startup](../configuration/model-providers.md#openai-and-anthropic-keys-at-startup)). Earlier versions replaced that credential with the variable's value on every start, so it's normally the same key.

2. **References move to gateway models.** After each gateway model sync, default model aliases and model access policies that named an old model are re-pointed to the gateway model for the same upstream model (`provider/model`). An undated Anthropic name that the old Anthropic provider listed, such as `claude-sonnet-4-5`, moves to the latest dated version the gateway lists, such as `anthropic/claude-sonnet-4-5-20250929`. A deactivated state, alias or cost override set on the old model carries over. Wildcard patterns in access policies, such as `claude-haiku-4-5*`, get the provider prefix (`anthropic/claude-haiku-4-5*`) of each provider whose models they matched.

3. **Old models and providers are deleted.** Some old models are kept until the gateway lists them: one that an alias or access policy still names, and one an administrator deactivated or gave an alias or cost override, so those settings carry over. An old provider is deleted once it has no models left. Its stored credentials are kept. If importing any provider's credentials fails, no old model or provider is deleted, and the import is retried on the next start.

## Client routes {#client-routes}

| Route | Status |
| --- | --- |
| `/api/llm-proxy/v1` | Serves every model in the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats. Use this for new setups. |
| `/api/llm-proxy/openai`, `/api/llm-proxy/anthropic` | Still work. They are served by the gateway, so existing Claude Code, Codex and SDK setups keep working. |
| `/api/llm-proxy/generic-responses`, `/api/llm-proxy/aws-bedrock`, `/api/llm-proxy/aws-bedrock-api-key`, `/api/llm-proxy/azure`, `/api/llm-proxy/azure-entra` | Removed. They return HTTP 410. Point these clients at `/api/llm-proxy/v1` and use gateway model names, such as `bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0`. |

Claude Code's Bedrock Mantle and Foundry modes pointed at the removed routes. Switch to the standard setup in [Using with Claude Code](./connect-clients.md#using-with-claude-code).

## Other changes {#other-changes}

- **Model access policies are profiles.** Each policy becomes a profile on the **Profiles** tab, with the same users, groups and models. Budgets and guardrail policies belong to profiles.
- **Daily token limits are replaced by budgets.** `--daily-user-input-token-limit` and `--daily-user-output-token-limit` only seed the *Daily token limit* budget when the gateway first starts. They default to 10,000,000 input and 100,000 output tokens per user per day, and a flag set below 0 leaves that limit out. A budget covers only its own profile's models, so every profile that exists at the upgrade gets its own copy of the budget, with administrators exempt and users' personal daily token limits still in its place. Every model a user could reach stays limited, but a user in several profiles gets an allowance from each of them. Edit or delete each copy on its profile's page; deleted copies don't come back, and profiles you create later don't get one. With no profiles at all, Obot creates the *Default Policy*, as on a first start, and gives it the budget. The `/api/users/{id}/remaining-token-usage` endpoint is removed.
- **The model provider configuration API is removed.** The configure, deconfigure, reveal and validate endpoints under `/api/model-providers` no longer exist. Listing and getting model providers still work, and return the gateway and any old provider that hasn't been cleaned up yet.
- **The models.dev price sync is removed.** `--model-info-source-url` / `OBOT_SERVER_MODEL_INFO_SOURCE_URL` no longer exists. Model prices now come from [`OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL`](./audit.md#model-pricing).
- **Message policies are LLM judge guardrails.** They're converted into LLM Judge guardrails used by disabled guardrail policies for you to review and enable. A message policy that didn't apply to all users gets its own profile, named after the policy with *(message policy)* added, with the policy's users and groups and no models. `OBOT_SERVER_ENABLE_MESSAGE_POLICIES` is removed. Past message policy violations stay in the database but aren't shown in the UI or returned by the API, and the LLM audit log no longer marks or filters requests a message policy changed. See [AI Judge Policies](../functionality/ai-judge-policies.md).
