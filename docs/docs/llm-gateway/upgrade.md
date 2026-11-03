---
title: "Upgrading from model providers"
---

These instructions describe migration support planned for a future patch release. Existing installations should not upgrade to v0.28.0, which introduced the gateway without migration support.

Earlier versions of Obot reached models through per-provider model providers (OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Azure and others), each with its own proxy route. These are gone. The [LLM gateway](./how-it-works.md) is now the only way Obot reaches models, and it can't be turned off.

:::warning Back up first
The upgrade can't be undone. Once aliases and policies name gateway models and the old providers are deleted, an earlier version of Obot can't serve them. Back up Obot's database before upgrading, and restore it to roll back.
:::

:::note Development installations
Development installations that used the earlier embedded gateway must re-enter that gateway's provider configuration, keys, routing rules and settings. The legacy model-provider import described below remains supported.
:::

## What happens on upgrade {#what-happens-on-upgrade}

The first time the new version starts, Obot moves your configuration to the gateway once:

1. **Credentials are imported.** The credentials of each configured model provider become a gateway provider:

   | Model provider | Gateway provider |
   | --- | --- |
   | OpenAI | `openai` |
   | Anthropic | `anthropic` |
   | Generic Responses Compatible | Custom provider `generic-responses`, with its base URL's trailing `/v1` removed, since the gateway adds it |
   | Amazon Bedrock, Amazon Bedrock API key | `bedrock_mantle` |
   | Azure, Azure Entra | `azure` |

   When both variants of Bedrock or Azure are configured, an administrator must choose which configuration to migrate in the migration dialog. No option is selected automatically. The dialog shows each source's endpoint or region and known models, without its credentials. **Decide later** leaves the family pending; reopen the dialog from **Models → Providers**. Other providers migrate independently.

   Only the selected configuration is imported. The other source's credentials, models and references remain available for manual setup, but its old proxy route cannot serve requests. The gateway currently supports one instance of each of these cloud providers; named cloud instances are not part of this upgrade.

   A gateway provider that already exists is preserved and reported as needing manual attention. When `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` is set, the environment key retains precedence over the stored legacy credential. Provider settings, keys and import provenance are saved together as one Obot resource. Interrupted imports resume through resource reconciliation without adding duplicate keys.

2. **References move to gateway models.** After each gateway model sync, default model aliases and model access policies that named an old model are re-pointed to the gateway model for the same upstream model (`provider/model`). An undated Anthropic name that the old Anthropic provider listed, such as `claude-sonnet-4-5`, moves to the latest dated version the gateway lists, such as `anthropic/claude-sonnet-4-5-20250929`. A deactivated state, alias or cost override set on the old model carries over. Wildcard patterns in access policies, such as `claude-haiku-4-5*`, get the provider prefix (`anthropic/claude-haiku-4-5*`) of each provider whose models they matched.

3. **Successfully migrated models are cleaned up.** A source model is deleted only after its provider import is verified, the gateway has applied its configuration and discovered the corresponding model, administrator edits have been copied, and references have moved. Compatibility mappings for old model IDs are saved before cleanup. Unsupported, undiscovered, pending and unselected source models remain intact. An imported old provider is deleted only when it has no models left; its stored credentials are kept. Failed imports retry through reconciliation and on restart.

## Client routes {#client-routes}

| Route | Status |
| --- | --- |
| `/api/llm-proxy/v1` | Serves every model in the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats. Use this for new setups. |
| `/api/llm-proxy/openai`, `/api/llm-proxy/anthropic`, `/api/llm-proxy/generic-responses` | Supported inference and model-listing requests retain their URLs. Bare model names resolve within the original provider. |
| `/api/llm-proxy/aws-bedrock`, `/api/llm-proxy/aws-bedrock-api-key` | Supported Mantle paths, including `anthropic/v1/messages` and `openai/v1/responses`, are adapted to the selected imported Mantle configuration. |
| `/api/llm-proxy/azure`, `/api/llm-proxy/azure-entra` | Supported Messages, Responses and model-listing paths are adapted to the selected imported Azure configuration. |

Existing client base URLs can remain unchanged for these supported operations. Old model resource IDs continue to resolve through saved migration mappings. Legacy routes never silently select another provider when the original configuration is unavailable. An unselected cloud variant or a configuration requiring manual setup returns HTTP 410 with instructions to review **Models → Providers**. Unsupported operations also return HTTP 410; inference requests are not redirected.

For new client setups use `/api/llm-proxy/v1` and provider-qualified model names, such as `bedrock_mantle/claude-sonnet-4-5`. See [Connecting clients](./connect-clients.md).

## Other changes {#other-changes}

- **Model access policies are profiles.** Each policy becomes a profile on the **Profiles** tab, with the same users, groups and models. Budgets and guardrail policies belong to profiles.
- **Daily token limits are replaced by budgets.** `--daily-user-input-token-limit` and `--daily-user-output-token-limit` only seed the *Daily token limit* budget when the gateway first starts. They default to 10,000,000 input and 100,000 output tokens per user per day, and a flag set below 0 leaves that limit out. A budget covers only its own profile's models, so every profile that exists at the upgrade gets its own copy of the budget, with administrators exempt and users' personal daily token limits still in its place. Every model a user could reach stays limited, but a user in several profiles gets an allowance from each of them. Edit or delete each copy on its profile's page; deleted copies don't come back, and profiles you create later don't get one. With no profiles at all, Obot creates the *Default Policy*, as on a first start, and gives it the budget. The `/api/users/{id}/remaining-token-usage` endpoint is removed.
- **The model provider configuration API is removed.** The configure, deconfigure, reveal and validate endpoints under `/api/model-providers` no longer exist. Listing and getting model providers still work, and return the gateway and any old provider that hasn't been cleaned up yet.
- **The models.dev price sync is removed.** `--model-info-source-url` / `OBOT_SERVER_MODEL_INFO_SOURCE_URL` no longer exists. Model prices now come from [`OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL`](./audit.md#model-pricing).
- **Message policies are LLM judge guardrails.** They're converted into LLM Judge guardrails used by disabled guardrail policies for you to review and enable. A message policy that didn't apply to all users gets its own profile, named after the policy with *(message policy)* added, with the policy's users and groups and no models. `OBOT_SERVER_ENABLE_MESSAGE_POLICIES` is removed. Past message policy violations stay in the database but aren't shown in the UI or returned by the API, and the LLM audit log no longer marks or filters requests a message policy changed. See [AI Judge Policies](../functionality/ai-judge-policies.md).
