---
title: "Upgrading from v0.27"
---

The [LLM gateway](./how-it-works.md) replaces the v0.27 model-provider backends. All model requests use the gateway, which cannot be turned off.

:::note Migration availability
For an upgrade from v0.27, use a release that includes [the companion migration change, #8514](https://github.com/obot-platform/obot/pull/8514). The migration behavior described here requires that change; restoring legacy client URLs alone does not migrate provider configuration or model references.
:::

:::warning Back up and review access
Back up the database before upgrading and restore that backup to roll back. Before returning to service, review provider configuration, default models, Profiles, wildcard grants and custom pricing. Test both allowed and denied requests as ordinary users; an administrator's successful request does not verify other users' access.
:::

## What happens on upgrade {#what-happens-on-upgrade}

- **Provider configuration:** Legacy credentials and connection settings are imported once. Existing gateway configuration wins and is not merged with legacy credentials. Azure and Bedrock authentication variants become separate keys in their family's provider pool; review their allowed models, endpoints, accounts and regions under **LLM Gateway → Providers**.
- **Model references:** After discovery publishes gateway models, default model aliases and explicit model IDs in Profiles move to matching gateway records. Undated model names can match the latest dated version within the same provider. Profile identities, subjects, budgets and guardrails remain unchanged.
- **Manual review:** Model settings, including disabled state, custom aliases and cost overrides, are not copied. Wildcard patterns remain unchanged; check their matches against gateway model names and adjust grants as needed. See [Profiles](../functionality/model-access-policies.md).
- **Recovery and retries:** Legacy models, providers and credentials remain stored. Unmatched references stay unchanged for manual review and retry after later model discovery. Failed provider imports retry on the next leader startup or process restart; deleting a successfully imported provider does not cause it to be imported again.

## Client routes {#client-routes}

Use `/api/llm-proxy/v1` and provider-qualified model names for new setups. Legacy provider URLs retain supported operations but are deprecated. They select their corresponding gateway provider and prefer an enabled imported key that supports the requested model, otherwise using the provider's normal key pool. Routing rules and fallbacks still apply.

See [Deprecated provider endpoints](./compatibility.md#deprecated-provider-endpoints) for provider mappings, supported paths and limitations, and [Connect clients and applications](./connect-clients.md) for current client configuration.

## Other changes {#other-changes}

- **Model access policies are Profiles.** Review their users, groups, models, budgets and applied guardrails under **LLM Gateway → Profiles**.
- **Daily token limits become budgets.** On initial gateway startup, `--daily-user-input-token-limit` and `--daily-user-output-token-limit` seed a *Daily token limit* budget on each existing profile, or the *Default Policy* when there are none. Defaults are 10,000,000 input and 100,000 output tokens per user per day; negative flag values omit that limit. Administrators are exempt and personal daily limits override the budget's limits. Users in multiple profiles receive each profile's allowance. Later flag changes do not change these budgets, deleted budgets are not recreated, and later profiles do not get one automatically. Edit budgets on each profile. The `/api/users/{id}/remaining-token-usage` endpoint is removed.
- **Model-provider configuration APIs are removed.** Configure, deconfigure, reveal and validate operations under `/api/model-providers` no longer exist. Listing and getting providers still return the gateway and retained legacy records. Configure providers through [LLM Gateway → Providers](../configuration/model-providers.md).
- **Model pricing configuration changes.** `--model-info-source-url` / `OBOT_SERVER_MODEL_INFO_SOURCE_URL` is removed. The gateway uses [`OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL`](./audit.md#model-pricing) instead.
- **Message policies become LLM Judge guardrails.** Converted guardrail policies start disabled for review. Policies scoped to specific users or groups get their own profile. `OBOT_SERVER_ENABLE_MESSAGE_POLICIES` is removed; past violations remain stored but are no longer exposed through the UI or API. See [AI Judge Policies](../functionality/ai-judge-policies.md).
