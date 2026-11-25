# 2026-11-20: Offline licensing with Keygen license files

- **Status:** Accepted
- **Date:** 2026-11-20
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/field-issues#72](https://github.com/obot-platform/field-issues/issues/72) — air-gapped
  installations, including fully offline licensing.

## Context

Obot licenses an installation with a Keygen license key, which it validates with `https://license.obot.ai` at startup,
daily, and when a key is added. An installation that can't reach that host can't be licensed, and a license key can't
carry its entitlements with it: Keygen only returns them from the validation call.

Keygen can check a license out as a license file: the license, its entitlements, and an expiry, signed with the
account's Ed25519 key.

## Decision

**A license file licenses an air-gapped installation without any network call.** An administrator drops the file on the
License page, sends it to `PUT /api/license` as `licenseFile`, or configures its contents with
`OBOT_SERVER_LICENSE_FILE`. The file grants the entitlements it carries.

**Only air-gapped installations use license files.** With
[`OBOT_SERVER_AIRGAP`](2026-11-23-air-gapped-installations.md), only a license file licenses the installation and
license keys are refused. Without it, an uploaded file is refused, a configured one refuses startup, and a stored one
is ignored, so licensing works exactly as it does with keys alone.

**Verification.** [`verifyLicenseFile`](../pkg/license/licensefile.go) verifies and decodes the file with keygen-go,
the Keygen library Obot already uses for license keys, against the public key in
[`keygen.pem`](../pkg/license/keygen.pem). That is the public key of Obot's Keygen account, embedded in the binary at
build time. It then requires Obot's Keygen product, the same one license keys are validated against.

A file expires at the earlier of the license's expiry and the file's own, and a file with neither doesn't expire. The
expiry is checked against the clock on every read, so a file expires without a restart.

**Precedence and storage.** In an air-gapped installation, the license in effect is the configured license file, else
the stored one. An uploaded file is stored in its own property, `obot-license-file`. Deleting it leaves the installation
unlicensed.

**No network.** While a file is in effect, Obot makes no Keygen calls, and `LicenseKey` returns nothing, so nothing can
forward a key.

**A bad file never stops the server.** A file that can't be verified, or that has expired, leaves the
installation unlicensed, logs the reason, and shows it to administrators on the License page. Startup is refused only
for settings that contradict each other: a license file configured without `OBOT_SERVER_AIRGAP`, a license key
configured with it, or a license file in the license key setting.

## Rationale

- **Keygen license files rather than our own format.** Obot's licenses already live in Keygen, and check-out produces
  the signed file with no new issuing service.
- **The public key is embedded.** Someone who can change configuration or the database still can't make Obot trust
  their own key. The key is public, so it's committed with the source.
- **The product is checked, policies aren't.** The product id is already a constant used for license keys, so checking
  it adds no release-time configuration. Pinning policies would need a release for every new offline policy. Who may
  check a license out is controlled in Keygen, through license permissions.
- **No rules of Obot's own about what a file contains.** Obot accepts what Keygen issued, as it does for a license
  key: whether a file expires and which entitlements it carries are decided at check-out.
- **Files only with air-gap mode.** One switch decides how an installation is licensed, so a file and a key never
  compete. An installation that can reach the license service keeps the key's online checks, suspension, and
  revocation.
- **A separate stored property.** Releases without license file support never read a file as a license key.
- **Unlicensed rather than down.** An expired or stored file that fails verification already left the installation
  unlicensed. A configured file does the same, so a damaged copy or a rotated key can't take the server down.

## Consequences

- Expiry and entitlements change only with a new file.
- The system clock is trusted, so a clock set back extends a file's life.
- A file issued without an expiry can't be withdrawn: suspension and revocation in Keygen never reach an issued file.
- Moving an installation into or out of air-gap mode changes how it's licensed: a stored key and a stored file each
  stay in the database, and only the one the mode uses is in effect.
- A leaked file works on any installation until it expires. The file embeds the license key, so Obot treats it as a
  secret: it's stored like the key, the Helm chart only takes it under `secret`, and logs and error messages never
  include its contents.
- A license holder who can check their own license out of Keygen can run it offline, without machine activation, until
  it expires. Keygen license permissions decide who can.
- Rotating the signing key needs an Obot release, and files signed with the old key then grant nothing.
- keygen-go has three gaps that Obot covers itself. It panics on a file whose envelope is null or names no algorithm,
  so verification recovers and refuses the file. Its `Decode` doesn't check the signature, so it only runs on a file
  that verified. It doesn't expose the license's product, so Obot reads that from the verified dataset.
- keygen-go also compares the file with the host's clock. Obot ignores that result and checks the expiry itself, so
  the same rule applies to every read.
- Upload a file only once every replica runs a release with license file support, since older replicas don't see it.

## References

- [License provider](../pkg/license/provider.go), [license file verifier](../pkg/license/licensefile.go), and
  [license API](../pkg/api/handlers/license.go)
- [Offline license files](../docs/docs/installation/network-requirements.md#offline-license-files)
- [Keygen license files](https://keygen.sh/docs/api/cryptography/#cryptographic-lic-format) and
  [license check-out](https://keygen.sh/docs/api/licenses/#licenses-actions-check-out)
