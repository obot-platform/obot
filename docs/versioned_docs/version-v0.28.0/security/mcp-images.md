---
title: "MCP image maintenance"
description: "How Obot maintains catalog MCP images for repeatable deployments and security updates."
---

Obot builds selected servers from the [MCP catalog](https://github.com/obot-platform/mcp-catalog) in the [MCP images repository](https://github.com/obot-platform/mcp-images) and publishes them under `ghcr.io/obot-platform/mcp-images/`. These images package npm or Python applications, upstream binaries, source builds, or existing containers. The [repackage manifest](https://github.com/obot-platform/mcp-images/blob/main/repackaging/images.yaml) and [repository-image manifest](https://github.com/obot-platform/mcp-images/blob/main/repository-images.yaml) list the configured builds and upstream versions covered here.

## Why use a packaged image?

Installing a server with npm or uv can resolve different transitive dependencies even when the server's own version is pinned. Lockfiles used by [npm ci](https://docs.npmjs.com/cli/v11/commands/npm-ci/) and [uv](https://docs.astral.sh/uv/concepts/projects/sync/) control dependency selection, but the language runtime and OS libraries still need to be managed separately.

An image packages the installed server, dependencies, and runtime together. Deployments reuse that result without downloading or installing the packaged dependencies at startup. You can evaluate an image once and deploy the same contents across environments.

[Pin by digest](https://docs.docker.com/build/building/best-practices/#pin-base-image-versions) to select the same artifact for a given platform; tags can move. Rebuilding the Dockerfile later may incorporate newer dependencies, so repeatable deployment does not imply byte-identical builds.

## How updates reach the catalog

Changes to configured upstream versions, build recipes, or shared bases trigger [builds of the affected images](https://github.com/obot-platform/mcp-images/blob/main/.github/workflows/publish-images.yml). Maintainers can also request rebuilds. Native AMD64 and ARM64 builds must both succeed before they are published under one image reference.

Versioned server images use `{version}-obotN` tags. For example, rebuilding upstream version `1.2.3` with patched dependencies could advance `1.2.3-obot1` to `1.2.3-obot2`. Each build receives a new revision, and the workflow refuses to replace an existing revision with different contents. Some catalog entries still use older tag formats or `latest`.

After publication, signing, and scanning, automation requests a [catalog-update pull request](https://github.com/obot-platform/mcp-catalog/blob/main/.github/workflows/update-image-tag.yml). Merging it changes the catalog's image reference. Existing Obot-managed vMCPs keep their saved definitions until an owner or administrator applies the update; Git-managed vMCPs refresh through catalog sync. See [Snapshots and updates](../mcp-gateway/publish.md#virtual-mcps-snapshots-and-updates).

## Security improvements from repackaging

Repackaging lets Obot update OS packages and runtime dependencies without waiting for a new upstream server release. Recipes support explicit dependency constraints and overrides for targeted fixes.

The shared Node and Python images use Wolfi bases, upgrade OS packages during assembly, and run servers as a non-root user. Other images have individual recipes: [Grafana](https://github.com/obot-platform/mcp-images/blob/main/repackaging/Dockerfile.grafana) refreshes its Alpine packages, while [Tableau](https://github.com/obot-platform/mcp-images/blob/main/mcp-servers/Dockerfile.tableau) installs production dependencies and removes npm and Yarn from the final image. Removing unnecessary tools reduces the software that must be patched.

The [publication workflow](https://github.com/obot-platform/mcp-images/blob/main/.github/workflows/publish-image.yml) signs each image digest with Cosign so consumers can verify the publisher. It also produces a software bill of materials (SBOM) in SPDX JSON format, listing detected components as a build artifact.

Trivy reports known vulnerabilities to GitHub code scanning and can rescan published images without rebuilding them. The workflow does not require zero findings before publication.
