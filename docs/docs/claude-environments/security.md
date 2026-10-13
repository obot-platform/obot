---
title: Network access, secrets, and auditing
description: Control session egress, inject destination-specific credentials, and review Claude Environment audit logs.
---

:::warning Experimental
These controls are experimental. Validate access restrictions and credential handling with your own tools before relying on them for sensitive workloads. Self-hosted sessions still communicate with Anthropic.
:::

## Allowed destinations

The internal Obot service and `api.anthropic.com` are always allowed for the integration to function. Additional hosts are configured on the environment and its profiles. Profiles add destinations; they cannot remove the environment's destinations. Matching profiles combine by union.

Enter hostnames rather than URLs. A wildcard such as `*.github.com` matches **one subdomain level**, such as `api.github.com`. It does not match `github.com` or `a.b.github.com`; add those separately when needed. The environment and profiles support up to 100 unique additional hosts in total. Remember to allow destinations needed for repository cloning, dependencies, and third-party tools.

**Allow all web destinations** permits HTTP on port 80 and intercepted HTTPS on port 443 to any hostname. The internal Obot HTTP endpoint retains its configured port. This option does not enable arbitrary TCP or UDP. Auditing and destination-specific credential rules still apply. Any matching profile can enable this broader policy. The saved hostname list becomes effective again when this option is disabled everywhere that applies to the user.

Policy changes also reach running sessions. Reconciliation and gateway caching normally take approximately 15 seconds; requests already in flight are not canceled. Test with a new request, not an existing connection or Claude's recollection of an earlier result.

## Obot vMCP credentials

The runner receives vMCP URLs and a placeholder header, not an Obot API key. The egress gateway obtains a short-lived, user-scoped key over mutual TLS and injects it only on canonical `/mcp-connect/…` requests to the internal Obot origin. It refreshes credentials without restarting the runner and rechecks live session access.

An environment allowlist does not bypass vMCP permissions. Obot applies the mapped user's access, or the dedicated Claude Tag user's access. Revocation and disabling a user prevent future credential retrieval. A new execution may be needed to load newly added tools into Claude's tool configuration.

## Injected HTTP secrets

Administrators can configure **Injected secrets** on environments and profiles. Provide:

- An exact lowercase destination hostname, such as `api.example.com`.
- An HTTP header name, such as `Authorization`.
- The complete value, such as `Bearer YOUR_TOKEN`.

Injection applies to **HTTPS port 443**. Secret destinations cannot be wildcards, although an egress wildcard can allow that exact host. Adding a secret does not grant network access. Internal Obot and routing headers cannot be overridden. At most 16 distinct secret header names may be configured for a destination across the environment and profiles.

Profiles override the environment on collisions of hostname and case-insensitive header name. Among matching profiles, the first in displayed order wins. Leave a saved value blank to retain it, supply a replacement to rotate it, or remove the entry to delete it.

Values are write-only and stored on the environment object using Obot's credential encryption mechanism. With encryption disabled, they are stored in plaintext. Actor configuration contains references rather than values, and the gateway fetches values through its authenticated credential-provider connection. Running sessions pick up updates through reconciliation and credential retrieval; do not expect instantaneous changes to in-flight requests.

Keeping a value out of the runner's configuration does not prevent the destination service from echoing it back or granting sensitive capabilities. Only inject credentials into destinations you trust.

## Managed settings

Actor **Advanced configuration → Managed settings JSON** accepts a JSON object up to 64 KiB. Obot checks the object shape and size, not whether individual keys are valid Claude settings. Administrators are responsible for the contents.

The runner writes the object to `/etc/claude-code/managed-settings.json` before starting Claude. Blank input uses the image default; `{}` supplies an empty object. Changes apply to new executions, including resumed sessions. Follow [Anthropic's managed settings and precedence documentation](https://code.claude.com/docs/en/settings); applicable server-managed policy takes precedence according to Claude's rules.

This is configuration delivery, not a tamper-proof filesystem boundary: the current runner operates as root. Do not put injected secrets in managed settings; use destination-specific injection instead.

## Audit access and capture

Open **Audit Logs → Claude Environments** as an administrator. Filter by user, Claude account, environment, session, destination, result, and time. **Details** opens the right-side panel. Membership in the **auditor** group is additionally required to view request/response bodies and headers; administrator or owner status alone does not grant payload access.

HTTP and HTTPS passing through the egress gateway are audited. HTTPS interception uses Substrate's certificate infrastructure and still verifies upstream certificates. Certificate-pinned clients or clients that ignore the provided trust configuration can fail.

Obot buffers streamed captures and persists them in batches after streams close. Graceful shutdown stops handlers and flushes partial captures. Retention is separate from session retention: `config.OBOT_SERVER_EGRESS_AUDIT_LOG_RETENTION_DAYS` defaults to **90 days**, and **0 disables audit cleanup**. Age is measured from the record's last update.

Capture is best effort:

- Bodies are limited to 1 MiB per direction; details report truncation and original byte counts.
- Header credentials and query values are redacted, including configured injected header names. Arbitrary bodies can still contain secrets.
- Abrupt termination can lose in-memory captures. Database failures or the bounded queue filling can also lose records; Obot logs drops.
- DNS is outside this audit stream. WebSocket capture covers the HTTP handshake, not individual messages.
- Long-lived requests may not appear until they close and the batch is flushed.

Enable Obot encryption for sensitive audit storage. A custom encryption configuration must include `egressauditlogs.obot.obot.ai`; see [encryption](../security/credentials.md). Review the broader [audit data policy](../security/audit-data.md) before granting auditor access.
