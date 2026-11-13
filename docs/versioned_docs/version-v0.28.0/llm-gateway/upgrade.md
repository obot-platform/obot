---
title: "Upgrading from v0.27"
---

:::warning Upgrade directly to v0.28.1 or later
Existing installations using model providers or the LLM gateway should upgrade directly to **v0.28.1 or later**. v0.28.0 has no migration from v0.27 model providers; the migration below requires **v0.28.1**.
:::

:::warning Back up first
Back up Obot's database before upgrading. The upgrade changes stored configuration and model references. Restore the backup to roll back to an earlier version.
:::

The [LLM gateway](./how-it-works.md) replaces the legacy model-provider backends. All model requests now use the gateway, which can't be turned off.

## Postmigration checklist {#postmigration-checklist}

:::warning Review access before returning to service
Automatic migration does not replace an administrator's review of provider configuration and model access.
:::

1. **Providers:** Under **LLM Gateway → Providers**, resolve import outcomes and confirm expected models are discovered and available. Existing gateway configuration wins conflicts.
2. **Profiles:** Review users, groups, explicit model grants, default aliases and wildcard patterns under **LLM Gateway → Profiles**. Review wildcard patterns against the gateway model names; migration leaves wildcard patterns unchanged. See [Profiles](../functionality/model-access-policies.md).
3. **Pooling:** Review each Azure and Bedrock key's allowed models, accounts, endpoints and regions. Unified-endpoint requests can select any eligible key in the pool. Legacy routes prefer their own imported key when it is enabled and supports the requested model. Bedrock initially allows every model, while Azure retains deployment names.
4. **Access:** Test both allowed and denied requests as ordinary users from representative groups. An administrator's successful request does not verify ordinary users' access.

## What happens on upgrade {#what-happens-on-upgrade}

On first startup, Obot imports legacy configuration into the gateway.

### Providers

- An existing gateway provider is left unchanged; its legacy credentials are not merged into it. Review existing configuration before relying on a legacy route.
- The generic provider's base URL loses its trailing `/v1`; the gateway adds it.
- Azure and Bedrock authentication variants become separate keys in the same provider pool, preserving credentials, Azure endpoints and Bedrock regions.
- Unified-endpoint requests can select any eligible key for the same model. Legacy routes prefer their own imported key when it is enabled and supports the requested model. Review allowed models and account/region requirements.
- Bedrock keys initially allow every model. Azure keeps legacy deployment names; if none exist, configure deployment names or Azure account discovery under **LLM Gateway → Providers**.

### Model references

- Default aliases and explicit model IDs in profiles move to the discovered gateway model for the same upstream model (`provider/model`). Profile subjects and associations stay unchanged.
- Undated Anthropic names map to the latest dated version the gateway lists.
- Migration does not copy model settings. Review active state, aliases, display names, usage and cost overrides on the gateway models.
- Wildcard patterns remain unchanged. Review their matches against the gateway model names and adjust them as needed.

### Retained configuration and retries

- Legacy models, providers and stored credentials remain intact. Reference migration does not delete them or change source or destination models.
- Unmatched explicit references remain unchanged for manual review and are retried until matching gateway models are available.
- Failed provider imports retry; a completed import is not repeated after its gateway provider is deleted.

## Client routes {#client-routes}

:::warning Deprecated provider endpoints
Legacy provider endpoints still support compatible operations, but are deprecated and will be removed in a future release. Migrate existing clients to `/api/llm-proxy/v1` with provider-qualified model names, such as `bedrock_mantle/anthropic.claude-haiku-4-5`. See [Connecting clients](./connect-clients.md).
:::

**v0.28.1** restores supported legacy operations through the gateway:

| Route | Status |
| --- | --- |
| `/api/llm-proxy/v1` | Serves every model in the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats. Use this for new setups. |
| `/api/llm-proxy/openai`, `/api/llm-proxy/anthropic`, `/api/llm-proxy/generic-responses` | Supported inference and model-listing requests retain their URLs. Bare model names resolve within the original provider. |
| `/api/llm-proxy/aws-bedrock`, `/api/llm-proxy/aws-bedrock-api-key` | Supported paths, including `anthropic/v1/messages` and `openai/v1/responses`, use the `bedrock_mantle` provider. |
| `/api/llm-proxy/azure`, `/api/llm-proxy/azure-entra` | Supported Messages, Responses and model-listing paths are adapted to the current `azure` provider pool. |

- Retained routes prefer their matching imported key. If it is absent, disabled, or does not support the requested model, they use the provider's normal key pool. Model access policies, budgets, guardrails and audit logging still apply; routing rules and fallbacks can change the serving provider or model.
- Both AWS legacy routes initially select `bedrock_mantle`. The v0.27 AWS providers used Mantle; their IAM and API-key credentials import as separate keys of that provider.
- Unsupported legacy operations remain unavailable. Use `/api/llm-proxy/v1` with a configured provider and an accessible model for new setups.
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

### Existing AWS gateway configurations

The provider picker distinguishes **AWS Bedrock Runtime** (`bedrock`) from **AWS Bedrock Mantle** (`bedrock_mantle`). Choose Mantle for native Mantle IDs such as `anthropic.claude-haiku-4-5`; use Runtime for Runtime model IDs and inference profiles.

Existing Runtime configurations are left untouched. If you previously configured a Mantle model under Runtime, configure the Mantle provider and select its model in your client, profile, or default model setting. Retained legacy credentials can populate the Mantle provider on startup, but gateway references already rewritten to Runtime models are not automatically retargeted.
