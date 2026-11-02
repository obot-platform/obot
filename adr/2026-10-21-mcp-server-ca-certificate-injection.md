# 2026-10-21: Inject user-supplied CA certificates into MCP server containers

- **Status:** Accepted
- **Date:** 2026-10-21
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

Customers adding their own MCP server often need that server (or an upstream it calls) to trust a private, corporate, or otherwise non-public CA — for example an internal API behind a certificate issued by the customer's own CA. Obot had no first-class way to supply such a certificate. A power user could already hand-roll something close to it using a generic `config` entry with `usage: "file"` and a self-chosen env var name (`MCPConfig`/`ServerConfig.Files`, see `pkg/mcp/types.go`), but there was no dedicated UI affordance, no awareness of which env var names matter to common clients, and no attempt at installing the certificate into the container's OS trust store.

## Decision

Add `CACertificate` as a new `types.Usage` value (alongside `env`, `header`, `file`, `dynamicFile`, `interpolated`) that a `config` entry (`types.MCPConfig`) can carry, holding one or more concatenated PEM certificates as its `Value`. `ServerToServerConfig` (`pkg/mcp/types.go`) turns a `CACertificate`-usage entry into five `ServerConfig.Files` entries, one per env var that mainstream HTTP/TLS clients check for an additional trust anchor: `SSL_CERT_FILE`, `NODE_EXTRA_CA_CERTS`, `REQUESTS_CA_BUNDLE`, `CURL_CA_BUNDLE`, `GIT_SSL_CAINFO`. This reuses the existing file-mount pipeline in both the Kubernetes backend (`pkg/mcp/kubernetes.go`) and the Docker backend (`pkg/mcp/docker.go`) unchanged — no new volume or secret machinery was needed.

For the `containerized` runtime, when the manifest also supplies an explicit container command override, a `CACertificate` config entry can additionally opt in (`InstallCATrustStore`) to a best-effort attempt to install the certificate into the container's OS trust store: the real command is wrapped behind a `/bin/sh -c` preamble that copies the mounted certificate into the conventional Debian/Ubuntu (`/usr/local/share/ca-certificates`) and RHEL/Fedora (`/etc/pki/ca-trust/source/anchors`) locations, runs `update-ca-certificates` or `update-ca-trust extract` if present, and then `exec`s the original command regardless of whether that step succeeded. This is opt-in, not automatic, because unconditionally replacing the command of any image without a POSIX shell would break a previously-working deployment outright - there is no way to tell ahead of time whether an arbitrary image has one.

The value is validated once, after it's fully resolved (a literal manifest value, a bound credential, or a per-user-supplied value), and before any of it is used: an invalid certificate fails the deployment with an error rather than mounting broken files.

The UI (`CatalogConfigurationForm.svelte`) exposes this as an "Embed CA Certificate" choice in the same Usage dropdown used to add any other config entry, rendering a PEM textarea in place of the usual key/value fields for that row instead of a separate, dedicated field or section.

## Rationale

An early version of this change added a dedicated `caCertificate` manifest field and a standalone "Advanced Options" UI section, reasoning that a certificate is public, always-optional data that deserves real validation and shouldn't depend on a user picking the right ad-hoc key name and usage for five separate env vars by hand. Product feedback moved this into the existing Usage model instead: a certificate behaves exactly like the other usages already in that list (a value that gets injected into the container a particular way), so it reads more naturally as one more option in the Configuration section a user already knows, rather than as a separate concept bolted onto the manifest. The validation goal is preserved: `ValidateConfig` still rejects anything that isn't a well-formed `CERTIFICATE` PEM block for a `CACertificate`-usage entry, and the UI still auto-assigns a fixed reserved key (`__caCertificate`) so the user never has to name one.

Folding this into `Config []MCPConfig` also simplified the implementation versus the dedicated-field version: propagation through `ConvertToCatalogEntry`/`MapCatalogEntryToServer` and the save/load mapping in the UI needed no special-casing at all, because the existing generic `Config` handling already carries any entry's `Usage` and `Value` through unchanged.

OS trust-store installation is scoped narrowly and deliberately: it only runs for the `containerized` runtime, only when the user has given an explicit command to re-exec, and only when explicitly requested. Obot does not control the ENTRYPOINT of a user-supplied container image, so wrapping it safely requires already knowing the full command and arguments to hand back control to; without that, `sh -c` would have nothing certain to `exec`. Making it opt-in rather than automatic whenever both conditions hold avoids silently breaking a deployment that was working before a certificate was added, for an image that turns out to have no shell at all. We considered fetching the image's baked-in ENTRYPOINT from its registry manifest (e.g. via `go-containerregistry`) to lift the "explicit command" restriction, but rejected it for this change given the added dependency, registry-auth edge cases, and deploy-time latency it would introduce for comparatively small additional coverage; the env-var mechanism already covers the large majority of HTTP clients regardless. The hosted `npx`/`uvx` (`stdio-wrapper`) and `openapi` runtimes execute through separate obot-managed images (`ghcr.io/obot-platform/mcp-images/*`) whose own binaries spawn the underlying process directly rather than through a shell Obot constructs, so trust-store installation for those runtimes is out of scope here and would need to be built into those images' own repos.

Only `NODE_EXTRA_CA_CERTS` is additive by convention (Node merges it with its own bundled roots); `SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE`, `CURL_CA_BUNDLE`, and `GIT_SSL_CAINFO` replace a client's default trust store rather than extend it once set. Pointing all five at a file containing only the pasted certificate(s) can therefore cause a client using one of the latter four to stop trusting certificate authorities it trusted before - for example Python's `requests` library losing trust in public CAs the moment `REQUESTS_CA_BUNDLE` is set to anything. Obot has no reliable way to merge the pasted certificate with an arbitrary target image's own default bundle at deploy time (it doesn't run inside the image, and different base images keep that bundle in different places), so this is a known, documented limitation rather than something this change fixes: the UI explains it next to the field, and a user who still needs public-CA trust for an affected client must include those certificates in the same field.

A config entry otherwise destined for `Env`, `File`, or `DynamicFile` usage may not reuse one of the five reserved env var names; `ValidateConfig` rejects it; both the Kubernetes and Docker backends resolve name collisions in their own secret/env map differently (one overwrites, one appends), so the only way to keep the two deployment backends' behavior identical is to not allow the collision in the first place.

## Consequences

- Users can paste a CA certificate as a config entry instead of reconstructing five env-var/file entries by hand; validation rejects malformed input (non-certificate PEM blocks, oversized input, garbage before/between/after PEM blocks) before it reaches a deployment, whether the value is a literal manifest value or resolved later from a bound credential or per-user input.
- Every runtime gets the env-var/file mechanism uniformly; only `containerized` runtime with an explicit command override, and only when the user opts in via `InstallCATrustStore`, gets the additional best-effort trust-store install. This is a documented, intentional gap rather than a silent one — the UI explains the requirement next to the toggle.
- The trust-store install is best-effort by design: a missing `update-ca-certificates`/`update-ca-trust` binary, a read-only filesystem, or an unexpected trust-store layout does not fail the container; it only means that particular install step silently did nothing.
- Deeper OS trust-store support for the `npx`/`uvx` and `openapi` hosted runtimes, or for `containerized` images with no explicit command, remains a follow-up that would need changes in the separate `stdio-wrapper`/`openapi-mcp` image repos (or registry-introspection support) rather than in this repo.
- Exactly one `CACertificate`-usage entry is possible per server: the UI auto-assigns it a fixed reserved key, so `ValidateConfig`'s existing duplicate-key check rejects a second one.
- For the four env vars that replace rather than extend a client's default trust, adding a CA certificate can remove trust the deployment relied on before. This is documented in the UI and in this ADR rather than solved, since solving it would require knowledge of the target image Obot does not have.

## References

- `pkg/mcp/types.go` — `ServerToServerConfig`, `configureContainerizedRuntime`, `resolveCACertificate`.
- `apiclient/types/mcpserver.go` — `CACertificateEnvKeys`, `ValidateCACertificate`, the reserved-key check in `ValidateConfig`.
- `pkg/substrate/claude.go` — prior art for the same five-env-var convention, used for the Claude sandbox egress MITM CA.
