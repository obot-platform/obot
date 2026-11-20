# 2026-11-20: Read-Only Admin as an add-on role

- **Status:** Accepted
- **Date:** 2026-11-20

## Related issues

- [Obot #8581](https://github.com/obot-platform/obot/issues/8581)
- [Obot #8099](https://github.com/obot-platform/obot/issues/8099)

## Related ODPs

None.

## Context

Security reviewers need to inspect administrative configuration and audit metadata. Giving them Admin would also grant writes. Giving them Auditor would expose sensitive audit data by default.

## Decision

Read-Only Admin is an independent role bit (256), represented by the `admin-readonly` authorization group. Admins and Owners can grant it directly or through an identity-provider group mapping. Effective role resolution keeps the highest base role and unions the add-on bits from all sources. An add-on-only mapping supplies no base role. Standard is the fallback when no source grants a base role.

The add-on grants administrative inspection through an explicit allowlist of safe reads. Inspection respects secret masking and avoids starting stopped workloads. Audit views expose metadata, and existing permissions govern personal MCP payload access. Auditor grants sensitive audit access and exports independently of this add-on.

## Rationale

A base-role design would replace users' existing privileges and could trigger workspace cleanup for Power Users. An independent bit gives reviewers the access they need while preserving base-role capabilities and allowing sensitive access to be granted separately.

## Consequences

Existing roles and mappings require no migration. Administrative endpoints need explicit classification as safe reads or excluded operations. The authorization tests check both routers for unclassified administrative reads.

## References

- [Earlier implementation in PR #8417](https://github.com/obot-platform/obot/pull/8417)
