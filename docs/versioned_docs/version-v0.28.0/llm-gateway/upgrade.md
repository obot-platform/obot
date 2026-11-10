---
title: "Upgrading from v0.27"
---

:::warning Upgrade directly to v0.28.1 or later
Existing installations using model providers or the LLM gateway should upgrade directly to **v0.28.1 or later**. v0.28.0 has no migration from v0.27 model providers; the migration below requires **v0.28.1**.
:::

:::warning Back up first
Back up Obot's database before upgrading. The upgrade can't be undone: earlier versions can't serve migrated aliases and policies after the old providers are deleted. Restore the backup to roll back.
:::

The [LLM gateway](./how-it-works.md) replaces the legacy model-provider backends. All model requests now use the gateway, which can't be turned off.

## Postmigration checklist {#postmigration-checklist}

:::warning Review access before returning to service
Automatic migration does not replace an administrator's review of provider configuration and model access.
:::

1. **Providers:** Under **LLM Gateway → Providers**, resolve import outcomes and confirm expected models are discovered and available. Existing gateway configuration wins conflicts.
2. **Profiles:** Review users, groups, explicit model grants, default aliases and wildcard patterns under **LLM Gateway → Profiles**. Add provider prefixes to unmatched legacy patterns. See [Profiles](../functionality/model-access-policies.md).
3. **Pooling:** Review each Azure and Bedrock key's allowed models, accounts, endpoints and regions. Requests and retries can use either key; Bedrock initially allows every model, while Azure retains deployment names.
4. **Access:** Test both allowed and denied requests as ordinary users from representative groups. An administrator's successful request does not verify ordinary users' access.

## What happens on upgrade {#what-happens-on-upgrade}

On first startup, Obot imports legacy configuration into the gateway.

### Providers

- The generic provider's base URL loses its trailing `/v1`; the gateway adds it.
- Azure and Bedrock authentication variants become separate keys in the same provider pool, preserving credentials, Azure endpoints and Bedrock regions.
- Requests and retries can use either key for the same model. Review allowed models and account/region requirements.
- Bedrock keys initially allow every model. Azure keeps legacy deployment names; if none exist, configure deployment names or Azure account discovery under **LLM Gateway → Providers**.

### Model references

- Default aliases and model access policies move to the gateway model for the same upstream model (`provider/model`).
- Undated Anthropic names move to the latest dated version the gateway lists.
- Deactivated state, aliases and cost overrides carry over.
- Display names and usage fill empty gateway fields; existing values in those fields win conflicts.
- Matching wildcard patterns gain a provider prefix, for example `claude-haiku-4-5*` → `anthropic/claude-haiku-4-5*`. Unmatched patterns need manual review.

### Cleanup

- Successfully migrated models are removed once configuration, discovery and reference updates are complete. Imported providers are removed when no models remain.
- Unsupported, undiscovered and failed models remain intact; stored provider credentials are retained.
- Failed imports retry automatically and on restart.

## Client routes {#client-routes}

:::warning Deprecated provider endpoints
Legacy provider endpoints still support compatible operations, but are deprecated and will be removed in a future release. Migrate existing clients to `/api/llm-proxy/v1` with provider-qualified model names, such as `bedrock_mantle/claude-sonnet-4-5`. See [Connecting clients](./connect-clients.md).
:::

In **v0.28.0**, only OpenAI and Anthropic legacy routes remained available; the others returned HTTP 410. **v0.28.1** restores supported operations through the gateway:

| Route | Status |
| --- | --- |
| `/api/llm-proxy/v1` | Serves every model in the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats. Use this for new setups. |
| `/api/llm-proxy/openai`, `/api/llm-proxy/anthropic`, `/api/llm-proxy/generic-responses` | Supported inference and model-listing requests retain their URLs. Bare model names resolve within the original provider. |
| `/api/llm-proxy/aws-bedrock`, `/api/llm-proxy/aws-bedrock-api-key` | Supported paths, including `anthropic/v1/messages` and `openai/v1/responses`, use the current `bedrock` provider when configured, otherwise `bedrock_mantle`. |
| `/api/llm-proxy/azure`, `/api/llm-proxy/azure-entra` | Supported Messages, Responses and model-listing paths are adapted to the current `azure` provider pool. |

- Retained routes use their provider's current keys and pool. Provider boundaries, model access policies, budgets and audit logging still apply.
- AWS routes prefer `bedrock`, otherwise `bedrock_mantle`. Model IDs aren't converted between adapters, and failed inference doesn't retry against the other adapter.
- Unsupported operations, missing AWS providers and unresolved AWS models return HTTP 410; use `/api/llm-proxy/v1` with an appropriate model.
- See [Deprecated provider endpoints](./compatibility.md#deprecated-provider-endpoints) for operation and model-name details.

## Other changes {#other-changes}

- **Model access policies become profiles**, preserving users, groups and models. Budgets and guardrail policies belong to profiles; see [Profiles](../functionality/model-access-policies.md).
- **Daily token limits become per-profile budgets:**
  - `--daily-user-input-token-limit` and `--daily-user-output-token-limit` seed the *Daily token limit* budget only on initial gateway startup. Defaults remain 10,000,000 input and 100,000 output tokens per user daily; flag values below 0 omit that limit.
  - Each existing profile gets a budget covering its own models. Administrators are exempt; personal daily limits override the budget's limits. Users in several profiles receive each profile's allowance.
  - Edit or delete budgets on each profile. Deleted budgets aren't recreated; later profiles don't receive one automatically. With no profiles, Obot creates *Default Policy* with the budget.
  - `/api/users/{id}/remaining-token-usage` is removed.
- **Model-provider configuration APIs are removed:** configure, deconfigure, reveal and validate under `/api/model-providers`. Listing and getting providers still return the gateway and remaining legacy providers.
- **models.dev price sync is removed.** `--model-info-source-url` / `OBOT_SERVER_MODEL_INFO_SOURCE_URL` is replaced by [`OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL`](./audit.md#model-pricing).
- **Message policies become LLM Judge guardrails:**
  - Converted guardrail policies start disabled; review and enable them. Policies scoped to specific users/groups get a profile named `<policy name> (message policy)` with those subjects and no models.
  - `OBOT_SERVER_ENABLE_MESSAGE_POLICIES` is removed. Past violations remain stored but disappear from the UI/API; audit logs no longer mark or filter requests changed by message policies.
  - See [AI Judge Policies](../functionality/ai-judge-policies.md) and [Audit usage and costs](./audit.md).
