# 2026-11-05: Allow explicit shared pools for legacy cloud providers

- **Status:** Accepted
- **Date:** 2026-11-05
- **Supersedes:** [Legacy model-provider migration](2026-11-03-legacy-model-provider-migration.md)
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8248

## Related ODPs

None. Calvin explicitly approved the shared-pool option in this implementation session.

## Context

Azure and Azure Entra, or Bedrock IAM and API-key configurations, may use different endpoints, accounts or regions. The engine stores these settings on individual keys and can serve both through one provider. Pooling changes routing: either key may serve the same model, including retries. Automatically combining configurations would change an installation's routing without consent; requiring only one configuration makes intentional pooling harder.

## Decision

Keep the existing single-configuration choices and add **Import both as a shared pool**, with no default selection. Explain that either configuration can serve requests, including through legacy routes and migrated model references. Both keys initially allow all models; administrators can restrict their model lists after migration.

Validate both configurations and create their keys, provider settings and provenance in one aggregate. Record the shared choice as `both`; identify its aggregate provenance by the provider family and that decision. Verify this provenance before migrating either source's model references. Environment-proxy mode evaluates proxy selection and `NO_PROXY` per destination; explicit administrator proxy settings take precedence.

Retain the previous decision's atomic imports, recovery and concurrency rules, source credential retention, model-discovery requirements and reference cleanup ordering. Existing gateway providers and completed imports are never replaced or extended by a new decision.

## Consequences

Pooling remains opt-in because keys can have different model availability, permissions, cost or regional behavior. Selecting both does not preserve source-specific key routing. Independently named cloud providers remain outside this change.
