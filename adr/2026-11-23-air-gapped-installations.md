# 2026-11-23: Air-gapped installations

- **Status:** Accepted
- **Date:** 2026-11-23
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/field-issues#72](https://github.com/obot-platform/field-issues/issues/72) — air-gapped
  installations.

## Context

An air-gapped installation can't reach Obot's services or GitHub. Before this change, an operator had to find and set
each outbound setting: turn off product analytics, update checks, and the MCP Tester model proxy, and point both
default catalogs at mirrors before first startup. A license key couldn't be used at all, since checking one calls
Obot's license service. The system MCP catalog is small, versioned with Obot, and needed for features such as
filters, but an air-gapped operator had to mirror its Git repository just to get it.

## Decision

**One switch.** `OBOT_SERVER_AIRGAP=true` marks an installation as air-gapped. At startup, before anything reads the
settings, [`applyAirgap`](../pkg/services/config.go) rewrites them:

- Obot never contacts Obot's services. Product analytics, update checks, and the MCP Tester model proxy are off,
  overriding `OBOT_SERVER_PRODUCT_ANALYTICS_MODE`, `OBOT_SERVER_DISABLE_UPDATE_CHECK`, and
  `OBOT_SERVER_MODEL_PROXY_URL`.
- Obot never contacts the license service. License keys and Community registration are refused, a stored license key
  isn't used, and only a [license file](2026-11-20-offline-license-files.md) licenses the installation. Obot doesn't
  start with a license key configured. An installation that isn't air-gapped never uses a license file.
- The default MCP catalog doesn't include Obot's catalog on GitHub. Admins bring their own, with
  `OBOT_SERVER_DEFAULT_MCPCATALOG_PATH` or in the UI.
- The default system MCP catalog is the copy the image ships at `/system-mcp-catalog`, unless
  `OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH` points somewhere other than Obot's repository on GitHub.

**For new installations.** Air-gap mode is set before first startup. The default catalogs are created once, then, so
the catalog changes apply to an installation that starts air-gapped.

**System catalog in the image.** The image build clones Obot's system MCP catalog into `/system-mcp-catalog`, and the
build fails if it can't. CI resolves the catalog's branch to one commit and builds every platform from it, and the
release's image list reads the same commit. Catalog sync already reads a catalog from a local directory, and the API
already accepts the configured default path as a source, so an air-gapped installation's system catalog is an ordinary
local source and needs no code of its own.

## Rationale

- **Rewriting settings at startup** keeps every downstream consumer unchanged: they see the same values an operator
  could have set by hand.
- **License files only.** A license key can't be checked without the license service, so accepting one would either
  fail or trust it unchecked. Air-gap mode is for new installations, which have no stored key to keep working.
- **Files in the image, not embedded in the binary.** A local directory is a source the catalog code already
  supports, and a fixed path in the image is the same on every replica and across upgrades. Embedding the catalog in
  the binary would cover release binaries too, but needs a build step to fetch it, a package to hold it, a source
  only the system catalog understands, and a release job to share one copy between builds.
- **Not shipping the MCP catalog** keeps a large, fast-moving catalog of mostly internet-dependent servers out of the
  release. An air-gapped operator chooses what to offer.

## Consequences

- An air-gapped installation's system catalog changes only with an Obot upgrade.
- A release binary run outside the image has no `/system-mcp-catalog`. An air-gapped installation that runs one
  points `OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH` at its own copy.
- Model price downloads, the skill repository, and MDM assets still have their
  own settings.

## References

- [Server configuration](../pkg/services/config.go), the [image build](../Dockerfile), and
  [catalog sync](../pkg/controller/handlers/mcpcatalog/mcpcatalog.go)
- [Network requirements](../docs/docs/installation/network-requirements.md#restricted-and-air-gapped-installations)
