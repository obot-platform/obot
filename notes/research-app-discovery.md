# Research: Vanta-style app discovery to recommend MCP servers

Idea 4 in `feature-ideas.md`. Researched 2026-11-08; nothing is built yet.

## How Vanta does it

Vanta reads the identity provider (IdP). The integration is read-only and never writes to the
IdP.

- **Google Workspace.** A super admin connects Vanta with two permissions: view users, and
  *view user OAuth token data*. With vendor discovery on, Vanta reads the OAuth tokens users in
  the domain have granted, and lists those third-party apps on a Discovery tab. There is no
  manual inventory. Declining the token permission turns discovery off.
- **Okta.** An Okta API app with OAuth and a private key (an API token also works). Scopes are
  read-only and include `okta.appGrants.read`. Vanta collects each app's display and internal
  names and timestamps.
- **Workflow.** Discovered vendors can be made Active, moved to procurement, ignored or
  rejected. A vendor reappears when more people sign in to it through the IdP.

## What Obot could read

| Source | API | Scope | What it tells us |
|---|---|---|---|
| Okta | `GET /api/v1/apps`; `GET /api/v1/apps/{id}/users` | `okta.apps.read` | Apps connected to Okta, and who is assigned to each, individually or through a group |
| Okta | `GET /api/v1/logs?filter=eventType eq "user.authentication.sso" and target.id eq "<app>"` | `okta.logs.read` | Who actually uses each app, and when they last did |
| Google Workspace | Admin SDK Directory `tokens.list` per user | `admin.directory.user.security` | Third-party OAuth apps users have authorized, including ones not behind SSO |
| Entra ID | Microsoft Graph `servicePrincipals`, `oauth2PermissionGrants`, sign-in logs | `Application.Read.All`, `AuditLog.Read.All` | Enterprise apps, consented grants, sign-ins |

None of these find apps used with personal accounts. Obot's own device scans could cover
those: they already see local AI clients and the MCP servers configured in them.

## Sketch for Obot

1. **Connect.** Reuse the IdP Obot already authenticates against. Obot has Okta and Entra auth
   providers, which already read groups, so the admin grants the extra read-only scopes. Google
   Workspace needs its own super-admin consent.
2. **Inventory.** On a schedule, store discovered apps with their usage: assigned users, active
   users over 30 days, and the IdP groups that grant them. Store no content.
3. **Match.** Map each app to MCP servers. First by exact name or domain against catalog
   entries and the MCP Registry API that Obot serves, then by the Supervisor's model for the
   long tail ("Atlassian Jira Cloud" is the same server as Atlassian's remote MCP).
4. **Recommend.** For example: "42 people in the `eng` Okta group use Linear. The catalog has
   Linear's MCP server. Create a Linear vMCP granted to `eng`?" One click creates the vMCP with
   a profile for that IdP group, using the same code as approving a server request into a new
   vMCP. Recommendations surface in three places:
   - a **What I noticed** insight on the Supervisor dashboard
   - Obot AI's admin tools (a `recommend_mcp_servers` tool on the admin MCP server)
   - the Server Requests page, as suggestions alongside user requests.
5. **Close the loop.** Users of the recommended app who later request the same server get it
   without waiting for an admin. Adoption of the new vMCP feeds `adoption_summary`.

### Open questions

- Should Obot hold IdP read credentials for this, or ingest an export the admin uploads?
- Do we want shadow-app discovery from Google OAuth grants? It is useful, but it is more
  sensitive data than the rest.
- Where do recommendations for apps without an MCP server go? Possibly the Registry, to see
  whether one exists upstream.

## Sources

- [Vanta: Google Workspace integration guide](https://help.vanta.com/en/articles/14977969-google-workspace-integration-guide)
- [Vanta: Okta integration guide](https://help.vanta.com/en/articles/15082003-okta-integration-guide)
- [Vanta: Integrating multiple identity providers](https://help.vanta.com/en/articles/11345659-integrating-multiple-identity-providers)
- [Okta: Applications API](https://developer.okta.com/docs/api/resources/apps/)
- [Okta: Application Users API](https://developer.okta.com/docs/api/openapi/okta-management/management/tags/applicationusers)
- [Okta: System Log query](https://developer.okta.com/docs/reference/system-log-query/)
