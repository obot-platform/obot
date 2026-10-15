---
title: "Editions and licensing"
---

# Editions and licensing {#obot-editions}

Every self-hosted edition of Obot runs from the same container image. A license key decides which capabilities and
usage limits apply, and you can add, change, or remove it from the **License** page in the admin UI without reinstalling.

## Editions

| | Obot Community | Obot Community, registered | Obot Enterprise | Obot Cloud |
|---|---|---|---|---|
| Hosting | Self-hosted | Self-hosted | Self-hosted | Hosted by Obot |
| Cost | Free | Free | [Contact us](https://obot.ai/contact-us/) | [Free trial](https://obot.ai/obot-cloud-trial/), then paid |
| License key | None | Community license, issued when you register | Enterprise license | Managed by Obot |
| Users | Up to 100 | Up to 100 | Unlimited, or the number in your contract | No limit |
| Devices (Obot Sentry) | Up to 100 | Up to 100 | Unlimited, or the number in your contract | — |
| Local, GitHub, and Google sign-in | ✓ | ✓ | ✓ | ✓ |
| Entra, Okta, JumpCloud, and Auth0 sign-in | — | ✓ | ✓ | ✓ |
| All [model providers](../configuration/model-providers.md) | ✓ | ✓ | ✓ | ✓ |
| Enterprise support | — | — | ✓ | ✓ |

### Obot Community

Obot Community is the free, open source edition, and it is what you get when no license key is configured. It
includes the full MCP gateway and registry, LLM gateway, skills registry, and Device Management, with Local, GitHub,
and Google [authentication](../configuration/auth-providers.md). It supports up to 100 users and 100 devices.

### Registering Obot Community

Registration is free and unlocks the enterprise [auth providers](../configuration/auth-providers.md): Entra, Okta,
JumpCloud, and Auth0. The 100 user and 100 device limits still apply.

To register, an administrator opens the **License** page, or the registration prompt shown when they configure one of
those auth providers, and enters a name, an email address, and optionally a company. Obot sends those details with the
installation ID to `https://upgrade-server.obot.ai`, receives a Community license key, and installs it. You don't need
to copy or keep the key.

### Obot Enterprise

Obot Enterprise includes everything in registered Obot Community, removes the user and device limits (unless your
contract sets specific limits), and adds enterprise support. To get an Enterprise license,
[contact us](https://obot.ai/contact-us/).

## Configuring a license key

Add an Enterprise license key in one of two ways:

- **In the admin UI.** Go to **License**, choose **Add License Key**, and paste the key. Obot validates the key before
  saving it, and stores it in the database.
- **With configuration.** Set [`OBOT_SERVER_LICENSE_KEY`](../configuration/server-configuration.md). A key set this
  way takes precedence over one stored in the database. The **License** page shows it as locked: you can't change,
  delete, or register over it in the UI.

Removing the license key from the **License** page returns the installation to unregistered Obot Community.

## How Obot validates a license

Obot validates license keys with Obot's license service at `https://api.keygen.sh`. Obot needs outbound HTTPS access
to that host to validate a license. See [Network requirements](../installation/network-requirements.md) for the other connections Obot makes.

Obot checks the license:

- When the server starts
- Every 24 hours after that
- When an administrator adds or changes the key. Obot checks a new key before saving it.
- When an administrator selects **Recheck License** on the **License** page. The button appears for Enterprise keys.
  Manual checks are limited to one every five minutes, and a successful one also tells every other replica to check
  again.

The first successful check activates the installation against the license. An installation is identified by an ID
stored in the Obot database, so all replicas that share a database count as one installation. A new database is a new
installation and activates again.

Each check has one of three outcomes:

| Outcome | What Obot does |
|---|---|
| The license service accepts the license | Obot applies the license's entitlements and saves them in the database |
| The license service rejects the license | Obot removes the license's entitlements and the installation behaves as [unlicensed](#what-happens-without-a-license) |
| The license service can't be reached or doesn't give an answer | Obot keeps using the entitlements saved from the last successful check |

### When the license service can't be reached

A check that times out, fails on the network, or gets an error from the license service that isn't about the license
itself doesn't change the license. Obot keeps using the entitlements saved from the last successful check, and tries
again at the next scheduled check. There is no time limit on this fallback: the license keeps working through an
outage of any length, until the license service answers again.

This is the same whether the check happens at startup or while Obot is running. A failed check at startup doesn't
prevent Obot from starting.

The fallback needs a successful check to fall back on. If a key has never been validated, Obot has no saved
entitlements and treats the installation as unlicensed until a check succeeds. This happens when:

- `OBOT_SERVER_LICENSE_KEY` is set on a new installation, or on one with a new database, that can't reach
  `https://api.keygen.sh`. Air-gapped installations can't validate a license key. Contact
  [Obot](https://obot.ai/contact-us/) if you need to run Obot Enterprise without that access.
- An administrator adds a key in the admin UI while the license service can't be reached. Obot doesn't save the key
  and shows "couldn't reach the license server to check the license key, try again later". The previous key, if any,
  stays in effect.

### When the license is no longer valid

The license service rejects a license when the key is invalid, expired, suspended, or revoked, or when the license
has no installations left to activate. Obot acts on the rejection at the next check: within 24 hours, at the next
restart, or when an administrator selects **Recheck License**. There is no grace period after a rejection.

When an administrator adds a key in the admin UI that the license service rejects, Obot doesn't save it and shows
"license key is invalid". The previous key, if any, stays in effect.

### Checking the license state

The **License** page in the admin UI shows:

- **License Key**: the configured key, partially masked
- **License Status**: for an Enterprise key, **Active** when Obot has entitlements for it and **Invalid** when it
  doesn't. Without a key, or with a Community license, it shows **N/A (Open-Source)**.
- **Entitlements**: the capabilities the license grants
- **User Limits** and **Device Limits**: current usage against the limits that apply

The same information is available from the API at `GET /api/license`, and `POST /api/license` runs a manual check.
`GET /api/version` also reports `hasValidLicense`, `userLimit`, `deviceLimit`, and any `licenseEntitlementViolations`.

### Logs

Obot logs each license check in the server logs:

| Message | Level | Meaning |
|---|---|---|
| `license provider initialized` | Info | Obot started with a license, and lists its entitlements |
| `initial license refresh failed` | Warn | The check at startup failed. Obot started anyway. |
| `license update finished` | Info | A scheduled check completed, and lists the entitlements now in effect |
| `license update failed` | Warn | A scheduled check failed |
| `license check failed` | Error | The license service couldn't be reached. `usingSavedEntitlements=true` means the last accepted entitlements are still in effect. |
| `license rejected` | Warn | The license service rejected the license, and gives the reason |
| `license validation failed` | Warn | The license service found the license not valid, and gives a `code` and `detail` |

### Recovering

Once the license service is reachable again, or once the license has been renewed, select **Recheck License** on the
**License** page, or restart Obot. Otherwise Obot picks up the change at the next scheduled check, within 24 hours.
Nothing else needs to be done: the entitlements apply as soon as a check succeeds.

To replace a rejected key, add a new key on the **License** page, or update `OBOT_SERVER_LICENSE_KEY` and restart Obot.

## What happens without a license

An installation is unlicensed when it has no license key, the license service has rejected its license, or its key has
never been validated. It then behaves as unregistered Obot Community. Obot does not delete data, users, devices, or
configuration when this happens, and everything comes back once a license is valid again.

### User and device limits

The limits return to 100 users and 100 devices.

- Existing users can still sign in, and enrolled devices stay enrolled.
- When the installation is at or above the limit, Obot refuses to create new users, whether they sign in for the first
  time or are provisioned through SCIM. A new user sees "Unable to provision your account. Please contact your
  administrator."
- When the installation is at or above the device limit, Obot refuses to enroll new devices, and Obot Sentry reports
  "Unable to enroll your device. Please contact your administrator."
- Administrators see a banner when 10% or fewer of the user seats remain, and another when the limit is reached.

Deleted users and the bootstrap user don't count toward the user limit. Every enrolled device counts toward the
device limit.

### Enterprise auth providers

New Entra, Okta, JumpCloud, or Auth0 providers can't be configured. If one of them is already configured, it stays
configured and users can still sign in with it, but Obot considers the installation out of compliance and limits
functionality until an administrator resolves it. While that's the case, Obot returns `402 Payment Required` for:

- MCP server connections, including OAuth authorization for MCP servers
- The LLM gateway
- The skills API
- Device scan uploads from Obot Sentry

Users see "We're sorry, this system is currently operating with limited functionality. Please contact your
administrator." Administrators see a banner that the license is missing or invalid.

To restore full functionality, select **Resolve** in the banner and do one of the following:

- Register for a Community license, which covers the enterprise auth providers.
- Add a valid Enterprise license key, or contact [Obot](https://obot.ai/contact-us/) to renew one.
- Choose **Downgrade** to deconfigure the enterprise auth providers. Users who signed in with those providers must
  then sign in with a provider that is still configured.

### Other capabilities

The MCP server tester's built-in model, which Obot hosts, requires a valid license of any kind. Everything else,
including model providers, MCP hosting, registries, and Device Management, works the same without a license.

## Feature availability

Edition limits and runtime availability are separate. A license does not enable a disabled feature or supply its
infrastructure.

| Capability | Additional requirement or status |
|---|---|
| MCP and LLM gateways, registries, and skills | Configure resources, credentials, and access policies for the intended users |
| Device Management | Beta; install and enroll Obot Sentry on supported devices |
| Local tool-call enforcement | Experimental; only supported clients and identified tool calls are covered |
| Managed image pull secrets and Kubernetes secret bindings | Kubernetes MCP runtime required |
| Domain-based MCP egress control | Kubernetes and a configured supported policy provider required |

See [Device Management](../device-management/how-sentry-works.md) and [Server Configuration](../configuration/server-configuration.md) for the relevant constraints.
