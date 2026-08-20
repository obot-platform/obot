# 2026-08-24: Thin composites

- **Status:** Accepted
- **Date:** 2026-08-24
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#7028](https://github.com/obot-platform/obot/issues/7028) — make composites thin references to their components.
- [obot-platform/obot#7130](https://github.com/obot-platform/obot/issues/7130) — reject invalid composite component references.

## Related ODPs

- [2026-08-16: Thin composites](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-08-16-thin-composites/README.md)

## Context

Composite catalog entries and deployments embedded copies of their components' manifests. Those snapshots drifted from their upstream entries, required a separate refresh step, and prevented component deployments from using the normal update lifecycle.

## Decision

Composite manifests store component references and only composite-owned state: tool overrides, tool prefixes, source digests, and per-deployment disabled state. Upstream component manifests remain authoritative and are not persisted in the composite.

API responses resolve and include component details at read time. Composite deployment responses also aggregate component configuration state. This preserves composite-based access without granting users direct access to the referenced components.

The composite controller materializes catalog-entry references as child MCP servers and multi-user references as MCP server instances. It validates the resolved manifest when writing a child, records component failures without blocking healthy siblings, and updates deployed children only after an explicit update request. Materialized children use the ordinary MCP server drift, diff, and update lifecycle.

## Rationale

References remove duplicated configuration and make each upstream entry the source of truth. Controller-owned materialization keeps deployment state, validation, and retries at the point where resolved manifests are written, while response-only hydration keeps the persistence model small.

## Consequences

- Catalog entry details reflect current upstream manifests; running children change only through an explicit update.
- Missing or invalid components degrade independently and surface through status instead of invalidating the whole composite.
- Deployed child manifests remain the record of what is running, but composites no longer preserve upstream point-in-time snapshots.
- List and single-object reads resolve and include component details.
- Rolling back to code that expects embedded component manifests is not supported.

## References

None.
