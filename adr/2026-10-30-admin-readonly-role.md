# 2026-10-30: Admin Read-Only as a base role

- **Status:** Accepted
- **Date:** 2026-10-30
- **Supersedes:** None
- **Superseded by:** None

## Related ODPs

None. There is no ODP for this decision.

## Context

Customers want to let someone see Obot's configuration without being able to change it. Roles are bit flags (`apiclient/types/user.go`): one base role (Owner, Admin, Power User Plus, Power User or Basic) plus the orthogonal Auditor flag. A user's effective role is their own role ORed with the roles of their group role assignments, then normalized to the highest base role plus Auditor if any source had it (`normalizeToHighestRole` in `pkg/gateway/client/grouproleassignment.go`). Before this decision, a user therefore held exactly one base role.

The only way to get read-only access to the admin area was the Auditor flag on a non-admin base role. Auditor also grants sensitive data: MCP, LLM and egress audit log request and response payloads, auth provider reveal, and audit log exports with their storage credentials. Granting someone read-only access to configuration meant also granting them that data, and assigning Auditor is reserved to owners.

## Decision

Add **Admin Read-Only** (`RoleAdminReadonly`, group `admin-readonly`) as a new base role.

- **Ranking.** Owner > Admin > Admin Read-Only > Power User Plus > Power User > Basic. Normalization keeps exactly one highest base role, with Auditor if any source grants it.
- **Standard User capabilities.** Admin Read-Only includes Basic. Users can create personal vMCPs, configure their own connections and credentials, complete OAuth, use permitted MCP servers, and create or revoke their own agent identities. Administrative read access does not grant publishing or workspace-management privileges.
- **Role changes.** Assigning Admin Read-Only replaces the previous base role, just as assigning Standard User does. If the effective role loses Power User, existing workspace reconciliation deletes the power-user workspace and cascades cleanup of its resources. The role dialog warns about this consequence. A group grant of Admin Read-Only also replaces lower effective roles; an Admin or Owner grant still takes precedence. No special workspace retention is introduced.
- **Access.** Admin Read-Only can read admin configuration but cannot change administrative configuration. Its static authorization rules (`adminReadonlyRules` in `pkg/api/authz/authz.go`) are GET routes only. It is excluded from:
  - administrative credential-reveal endpoints (personal connection credentials remain accessible to their owner);
  - action routes that are reads in name only (tool previews, the MCP tester, guardrail and storage credential tests);
  - MDM installer downloads, which can enroll devices;
  - setup and debug endpoints;
  - the unmasked license key;
  - a pending auth provider switch, which is shown only to admins, who can act on it.

  Audit logs appear as they do for an admin: without request and response payloads.
- **Auditor is unchanged.** Its rules and meaning stay the same, and it can be combined with Admin Read-Only (`RoleAdminReadonly | RoleAuditor`) to add payloads and exports.
- **Two "can view" checks.** `Role.groups()` adds the `admin-readonly` group for Owner, Admin and Admin Read-Only, but not for Auditor, whose access stays exactly as it was.
  - Handlers and authorizers that granted privileged reads to "admin or auditor" now use one check that also admits Admin Read-Only: `UserCanViewAdmin()` on the request and `User.CanViewAdmin()` in the authorizer.
  - Handlers that showed everything only to admins, such as the list of all workspaces or all Claude sessions, now use `UserCanReadAsAdmin()`. It admits owners, admins and Admin Read-Only but not auditors.
  - In the UI these are `hasAdminAccess()` and `isAdminReadonly()`.
  - Write paths, audit log payloads, the license key, and filters that hide what only an admin could set up (catalog entries whose OAuth credentials are not configured) keep their admin or auditor checks.
- **Assignment.** Admins assign and remove Admin Read-Only like Admin: directly on a user or through a group role assignment. Unlike Auditor, it does not require an owner.
- **Not a default role.** The default role for new users remains Basic. The server rejects Admin Read-Only as the default role, not only the UI.
- **Role validation.** Changing a user's role now rejects values that are not one known base role, plus, optionally, Auditor; previously any integer was accepted. A role that is not changing is not checked, so users whose stored role predates the check can still update other fields. Group role assignments already validated their base role and now accept Admin Read-Only. The default role, previously unchecked, must be Basic, Power User, Power User Plus or Admin: the roles the UI offers.

## Rationale

- **A base role rather than an add-on flag.** A flag would let Admin Read-Only be assigned beside any role, so whether a user can change admin configuration would depend on the combination. A base role keeps the answer simple to assign and audit, matching what customers ask for.
- **Use existing downgrade behavior.** Admin Read-Only is a Standard User with administrative reads. It does not preserve PU/PUP privileges, directly or through groups. Workspace lifecycle remains the same as other role downgrades, rather than adding a separate retention policy for this role.
- **A new role rather than narrowing Auditor.** Narrowing Auditor to sensitive data only, and migrating existing Auditor users to Admin Read-Only plus Auditor, would make the model cleaner. However, it would be a breaking migration, and users with Power User or Power User Plus plus Auditor would lose their base role. Leaving Auditor alone means no existing user's access changes.
- **An explicit read-only rule list rather than deriving one from the admin rules.** Many admin rules are method-less prefixes, some reads are POSTs (reveals), and some GETs are sensitive. An explicit list, checked by a test against the admin rules, makes every exposure a deliberate choice.
- **Auditors keep their own rules rather than joining the new group.** The Auditor rules leave out some admin reads on purpose (for example MCP tunnels and the secrets that may be bound), and existing tests pin that. Giving auditors the new group would have widened their access without anyone choosing to.

## Consequences

- Every new admin GET route must be either added to `adminReadonlyRules` or listed as an intentional exclusion in `adminReadonlyExcludedReads`; `TestAdminReadonlyRulesCoverAdminReads` fails otherwise. `TestAdminReadonlyCannotMutate` fails if the role can make any change its baseline role can't.
- Admin pages must honour read-only mode for every mutating control. Pages that relied on only admins reaching them now need read-only handling, which also fixes the same gaps Auditors had.
- Admin Read-Only with Auditor can read slightly more than Basic with Auditor: the union of both rule lists.
- A stored or effective role has one base role, optionally with Auditor. Explicit combinations of Admin Read-Only and PU/PUP are rejected.
- A direct PU/PUP-to-Admin-Read-Only assignment follows the existing downgrade lifecycle, including workspace cleanup when the effective role loses Power User.
- `TestAdminReadonlyRulesCoverAdminReads` covers only static authorization rules. A GET handler that filters what a non-admin sees by `UserIsAdmin()` must be checked by hand when widening what Admin Read-Only reads.
- A new or reranked base role must be checked against the workspace reconcile, since normalization drops lower base roles.
- Role bit 256 is now used. Owner and Admin users get an extra group, `admin-readonly`, in their identity.
- A read-only admin can still start an OAuth flow for their own connection to a server through the `oauth-url` reads, as any user with access to it can. That changes only their own credentials, not configuration.

## References

- `apiclient/types/user.go`: roles, groups and role hierarchy
- `pkg/api/authz/authz.go`: static rules for admin, Admin Read-Only and Auditor
- `pkg/api/authz/adminreadonly_test.go`: drift and no-mutation tests
- `pkg/api/authz/user.go`, `pkg/api/authz/ui.go`: privileged read checks
- `pkg/api/request.go`: `UserCanViewAdmin()`
- `pkg/gateway/client/grouproleassignment.go`: effective role resolution and normalization
- `pkg/controller/handlers/poweruserworkspace/`: workspace reconcile on role changes
- `pkg/gateway/server/user.go`: role assignment rules
- `ui/user/src/lib/services/user/operations.ts`: profile role helpers
- `ui/user/src/lib/services/admin/constants.ts`, `ui/user/src/routes/identity-access/`: role pickers
