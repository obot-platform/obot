---
title: "Editions and licensing"
---

# Editions and licensing {#obot-editions}

Obot is fully open source under the [MIT License](https://github.com/obot-platform/obot/blob/main/LICENSE), and that
includes every edition. There is no separate closed-source enterprise build. Obot Community, registered Community, and
Obot Enterprise all run the same open source code from the same container image.

The editions differ only in their license key. The key gates the user and device limits, the enterprise auth
providers, and support. You can add or change it from the **License** page in the admin UI without reinstalling.

## Editions

| | Obot Community | Obot Community, registered | Obot Enterprise | Obot Cloud |
|---|---|---|---|---|
| Hosting | Self-hosted | Self-hosted | Self-hosted | Hosted by Obot |
| Source code | Open source (MIT) | Open source (MIT) | Open source (MIT) | Open source (MIT) |
| Cost | Free | Free | [Contact us](https://obot.ai/contact-us/) | [Free trial](https://obot.ai/obot-cloud-trial/), then paid |
| Users | Up to 100 | Up to 100 | [Contact us](https://obot.ai/contact-us/) | Determined by plan |
| Devices (Obot Sentry) | Up to 100 | Up to 100 | [Contact us](https://obot.ai/contact-us/) | Determined by plan |
| [Enterprise auth providers](../configuration/auth-providers.md#available-auth-providers) | — | ✓ | ✓ | ✓ |
| Support | — | — | ✓ | ✓ |

### Obot Community

Obot Community is what you get when no license key is configured. It is free and
includes the full MCP gateway and registry, LLM gateway, skills registry, and Device Management, with Local, GitHub,
and Google [authentication](../configuration/auth-providers.md). It supports up to 100 users and 100 devices.

### Registering Obot Community

Registration is free and unlocks the enterprise [auth providers](../configuration/auth-providers.md): Entra, Okta,
OneLogin, JumpCloud, and Auth0. The 100 user and 100 device limits still apply.

To register, an administrator opens the **License** page, or the registration prompt shown when they configure one of
those auth providers, and enters a name, an email address, and optionally a company. Obot sends those details with the
installation ID to `https://upgrade-server.obot.ai`, receives a Community license key, and installs it. You don't need
to copy or keep the key.

### Obot Enterprise

Obot Enterprise is the same open source code as Obot Community, with an Enterprise license key. It includes everything in registered Obot Community, removes the user and device limits (unless your
contract sets specific limits), and adds enterprise support. To get an Enterprise license,
[contact us](https://obot.ai/contact-us/).

## Configuring a license key

Add an Enterprise license key in one of two ways:

- **In the admin UI.** Go to **License**, choose **Add License Key**, and paste the key. Obot validates the key before
  saving it, and stores it in the database.
- **With configuration.** Set [`OBOT_SERVER_LICENSE_KEY`](../configuration/server-configuration.md). A key set this
  way takes precedence over one stored in the database. The **License** page shows it as locked: you can't change,
  delete, or register over it in the UI.

Deleting an Enterprise license key from the **License** page returns the installation to unregistered Obot Community.
The page has no option to delete a registered Community key: replace it by adding an Enterprise key, or delete it with
`DELETE /api/license`.

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
| The license service rejects the license | Obot removes the license's entitlements and the installation falls back to unregistered Obot Community. See [License compliance](#license-compliance). |
| The license service can't be reached or doesn't give an answer | Obot keeps using the entitlements saved from the last successful check, until the license's last known expiration date |

### When the license service can't be reached

A check that times out, fails on the network, or gets an error from the license service that isn't about the license
itself doesn't change the license. Obot keeps using the entitlements saved from the last successful check, and tries
again at the next scheduled check.

Obot saves the license's expiration date with the entitlements, and the fallback lasts until that date, however
long the license service is unreachable. A license with no expiration date keeps working for as long as the outage
lasts. Changes made to the license while Obot can't reach the license service, such as a new expiration date, take
effect at the next check that gets through.

Obot compares the expiration date with the server's system clock at each check that fails, so an installation whose
license expires during an outage falls back to unregistered Obot Community at the first failed check after that date.
Once a check reaches the license service again, Obot applies its answer right away.

This is the same whether the check happens at startup or while Obot is running. A failed check at startup doesn't
prevent Obot from starting.

The fallback needs a successful check to fall back on. If a key has never been validated, Obot has no saved
entitlements and treats the installation as unregistered Obot Community until a check succeeds. This happens when:

- `OBOT_SERVER_LICENSE_KEY` is set on a new installation, or on one with a new database, that can't reach
  `https://api.keygen.sh`. Air-gapped installations can't validate a license key yet. Support for offline
  licensing is coming soon.
- The installation was upgraded from v0.26.3 or earlier, which didn't save entitlements, and the license service
  can't be reached at the first startup after the upgrade.
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
- **User Limits** and **Device Limits**: current usage against the limits, without an Enterprise license. With an
  Enterprise license the page shows **-**, even when the license sets limits.

From the API, `GET /api/license` returns the key, status, and entitlements, and `POST /api/license` runs a manual
check. Usage and limits come from `GET /api/version`, which reports `userCount`, `deviceCount`, `userLimit`,
`deviceLimit`, `hasValidLicense`, and any `licenseEntitlementViolations`. `userLimit` and `deviceLimit` are omitted when
the license has no limit.

### Recovering

Once the license service is reachable again, or once the license has been renewed, select **Recheck License** on the
**License** page, or restart Obot. Otherwise Obot picks up the change at the next scheduled check, within 24 hours.
Nothing else needs to be done: the entitlements apply as soon as a check succeeds.

To replace a rejected key, add a new key on the **License** page, or update `OBOT_SERVER_LICENSE_KEY` and restart Obot.

## License compliance

Obot compares what you've configured, and how many users and devices you have, against what your current license
allows. When you're over, Obot is out of compliance. Being out of compliance never deletes or disables anything you
already have: no data, users, devices, or configuration. What it blocks depends on what you're over.

| Out of compliance when | What happens | How to fix it |
|---|---|---|
| An enterprise auth provider (Entra, Okta, OneLogin, JumpCloud, or Auth0) is configured and the license doesn't include it | **Limited functionality.** Obot returns `402 Payment Required` for MCP server connections (including OAuth authorization for MCP servers), the LLM gateway, the skills API, and device scan uploads from Obot Sentry. Users can still sign in. | Register Obot Community, add an Enterprise license key, or deconfigure the provider |
| There are more users than the user limit | Existing users keep working. Obot refuses to create new users, whether they sign in for the first time or are provisioned through SCIM. | Add an Enterprise license key, or delete users |
| There are more enrolled devices than the device limit | Enrolled devices keep working. Obot refuses to enroll new devices. | Add an Enterprise license key, or remove devices |

Without an Enterprise license, the user and device limits are 100 each. Deleted users and the bootstrap user don't count
toward the user limit. Every enrolled device counts toward the device limit.

Obot also won't let you get into the first case through configuration: you can't configure a new enterprise auth
provider without a license that includes it.

### How installations end up out of compliance

- **An Enterprise license expires, or is revoked or suspended.** At the next check, the installation falls back to
  unregistered Obot Community. An installation with 250 users is then over the 100-user limit, and any configured
  enterprise auth provider is no longer covered.
- **An administrator deletes the license key.** The effect is the same as a license that's no longer valid.
- **The license expires while the license service is unreachable.** See
  [When the license service can't be reached](#when-the-license-service-cant-be-reached).
- **A license key was never validated**, for example on an air-gapped installation. See
  [When the license service can't be reached](#when-the-license-service-cant-be-reached).
- **The installation grows** past the user or device limit of its license.

:::note
A license service outage doesn't put an installation out of compliance. Obot keeps the entitlements from the last
successful check until the license's last known expiration date. See
[When the license service can't be reached](#when-the-license-service-cant-be-reached).
:::

### What administrators and users see

| Situation | Administrators | Users |
|---|---|---|
| An enterprise auth provider isn't covered by the license | A banner that the license is missing or invalid, with a **Resolve** button | "We're sorry, this system is currently operating with limited functionality. Please contact your administrator." |
| 10% or fewer user seats remain, or the user limit is exceeded, without an Enterprise license | A banner about the user limit, with a **Resolve** button | Nothing, until a new user tries to sign in |
| A new user can't be created | — | "Unable to provision your account. Please contact your administrator." |
| A new device can't be enrolled | Nothing. Without an Enterprise license, the **License** page shows device usage against the limit. | Obot Sentry reports "Unable to enroll your device. Please contact your administrator." |

With an Enterprise license that sets user or device limits, Obot enforces those limits but doesn't show a banner or
usage on the **License** page, so new users and devices are refused without warning once a limit is reached.

### Getting back into compliance

Select **Resolve** in the banner, or go to the **License** page, and do one of the following:

- **Register Obot Community.** This covers the enterprise auth providers, but not more than 100 users or devices.
- **Add a valid Enterprise license key**, or contact [Obot](https://obot.ai/contact-us/) to renew one.
- **Switch to an auth provider the license covers**, such as Local, GitHub, or Google. See
  [Switching between auth providers](../configuration/auth-providers.md#switching-between-auth-providers).
- **Choose Downgrade** to deconfigure the enterprise auth provider. Only one auth provider can be configured at a
  time, so no provider is configured afterwards: configure another one before users can sign in again.
- **Delete users or remove devices** until you're within the limits.

Nothing needs to be reconfigured afterwards. Obot applies a new or renewed license as soon as it validates the key,
and lifts the restrictions as soon as you're back within what the license allows.

The MCP server tester's built-in model, which Obot hosts, also requires a valid license of any kind. It isn't affected
by compliance.
