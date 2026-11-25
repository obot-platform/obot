# 2026-11-23: Mirrored Obot images

- **Status:** Accepted
- **Date:** 2026-11-23
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/field-issues#72](https://github.com/obot-platform/field-issues/issues/72) — air-gapped
  installations.

## Context

Obot pulls its own images from `ghcr.io/obot-platform`: the MCP base, HTTP webhook, and OpenAPI images, the Claude
runner and Substrate worker images, and the images of system catalog entries. An installation that can't reach
`ghcr.io` had to override each image setting by hand, couldn't redirect system catalog images at all, and had no
published list of what to mirror.

## Decision

**Registry rewrite.** `OBOT_SERVER_IMAGE_REGISTRY` names a registry, with an optional path, that mirrors Obot's images.
[`imageregistry.Rewrite`](../pkg/imageregistry/imageregistry.go) replaces `ghcr.io` in any image under
`ghcr.io/obot-platform/` with it, keeping the repository path and tag or digest:
`ghcr.io/obot-platform/x:v1` becomes `registry.example.com/mirror/obot-platform/x:v1`. Obot applies it at startup to
every image setting, and on each sync to the containerized images of system catalog entries. Images outside
`ghcr.io/obot-platform/`, and every image in admins' own MCP catalogs and servers, are used as written.

**Release image list.** [`tools/list-images`](../tools/list-images/main.go) runs in the release build and lists the
Obot server images for the release tag, the default of every server setting named `*Image`, the images in the system
MCP catalog commit the image ships, and the images the Helm chart deploys with Substrate on, which it
finds by rendering the chart and scanning for image keys, including the worker image the chart passes to Obot. The
release publishes the list as `images.txt`, for scripts that mirror Obot's images to
`<registry>/<path without ghcr.io/>`.

## Rationale

- **Rewriting only the images Obot itself chooses** keeps the setting predictable. MCP catalogs are the admin's: an
  air-gapped installation starts without Obot's catalog, and the images in a catalog an admin adds are theirs to
  place.
- **Keeping the path** matches how `crane copy` and `skopeo copy` scripts mirror a list, and keeps digests valid.
- **Scanning at build time** instead of a CLI command keeps the list out of the product surface, and reading settings
  by name makes a new image setting appear in the list without a code change.

## Consequences

- A new image setting must end in `Image`. The rewrite and the list both find settings by that name.
- The Obot server image itself is still set by the deployment: `image.repository` in Helm, or the `docker run` image.
- Images that MCP servers pull for themselves, and `npx` and `uvx` packages, aren't covered.
- The Helm chart, not Obot, deploys Substrate's own components, so the rewrite doesn't reach them. An air-gapped
  Substrate install also sets the chart's `substrate.image.repository` and `substrate.resourceInstallerImage` to the
  mirror.
- `images.txt` also lists images that aren't Obot's: kubectl, Envoy, Postgres, and the Kubernetes pause image. The
  Substrate chart hard-codes the last three, so the cluster's container runtime has to be configured to pull them
  from a mirror. Making them chart values is follow-up work.

## References

- [Image registry rewrite](../pkg/imageregistry/imageregistry.go) and [image list](../tools/list-images/main.go)
- [Network requirements](../docs/docs/installation/network-requirements.md#restricted-and-air-gapped-installations)
