# 2026-11-12: Restore legacy LLM gateway routes through the shared gateway

- **Status:** Accepted
- **Date:** 2026-11-12
- **Supersedes:** [Embedded LLM gateway](2026-09-26-embedded-llm-gateway.md) (legacy client-route policy only)
- **Superseded by:** None

## Related issues

- [#8495: Restore per-provider LLM proxy routes](https://github.com/obot-platform/obot/issues/8495)
- [#8248: Unified LLM gateway with budgets and guardrails](https://github.com/obot-platform/obot/issues/8248)

## Related ODPs

None.

## Context

The v0.28.0 gateway cutover removed several provider-specific URLs and kept the OpenAI and Anthropic URLs as aliases of the unified endpoint. Existing clients configured with the removed URLs could no longer connect. Restoring those URLs should reuse the gateway's policy and inference pipeline rather than revive separate provider backends.

## Decision

Keep `/api/llm-proxy/v1` as the endpoint for new clients. Restore supported operations under the legacy prefixes with these initial gateway providers:

| Prefix under `/api/llm-proxy/` | Gateway provider |
| --- | --- |
| `openai` | `openai` |
| `anthropic` | `anthropic` |
| `generic-responses` | `generic-responses` |
| `aws-bedrock`, `aws-bedrock-api-key` | `bedrock` |
| `azure`, `azure-entra` | `azure` |

Thin route adapters normalize legacy paths and pass provider scope through trusted request context to the shared authenticated proxy. Native names and aliases resolve within that provider; explicit model references must also match it initially. Listings are provider-scoped and access-filtered. In particular, `/openai` and `/anthropic` no longer accept other providers' models as unified aliases did in v0.28.0. Clients needing that behavior must use `/api/llm-proxy/v1` with provider-qualified model names.

Prefer the key named `imported-<legacy ModelProvider ID>` only when it is enabled and supports the resolved model. Otherwise use the provider's normal eligible key pool. Read this preference from the reconciled runtime snapshot, without loading configuration storage on each request. Azure and Bedrock authentication variants share a provider but retain distinct imported-key preferences.

Normal routing rules and fallbacks remain active. A rule changing the provider or model clears the initial key preference; an explicit rule-selected key takes precedence. Fallback attempts select their own credentials. Obot continues enforcing model access, budgets and guardrails and recording audit logs and usage through the existing gateway pipeline.

These adapters restore only supported gateway operations and the existing model-free Anthropic handlers, not every former provider-specific API. Both AWS prefixes select `bedrock`; they do not automatically switch to `bedrock_mantle`. Legacy URLs are deprecated and will be removed in a future release, with no removal version or date selected here.

## Rationale

Keeping the legacy URL's initial provider and credential family preserves its meaning while reusing the gateway's translation, routing and governance. The unified endpoint remains the interface for new and cross-provider client configurations. Credential and model-reference migration is a separate change; manually configured gateway providers can also serve the restored routes.

## Consequences

Existing clients can retain their base URL for supported operations. Clients that used OpenAI or Anthropic legacy URLs as cross-provider aliases must move to the unified endpoint. Administrators must review key eligibility and routing rules: pool fallback or rerouting can use another eligible credential. This record replaces only the earlier client-route decisions; the [core-gateway architecture](2026-11-02-core-llm-gateway.md) remains in effect.

## References

- [API compatibility and deprecated endpoints](../docs/docs/llm-gateway/compatibility.md)
- [Legacy route adapters](../pkg/gateway/server/llmproxy_legacy.go)
- [Legacy credential preference](../pkg/llmengine/legacyroutes.go)
- [Core routing rules](../pkg/llmcore/routing.go)
