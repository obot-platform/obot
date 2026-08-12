# Obot documentation

The Obot documentation site is built with Docusaurus 3 and published at [docs.obot.ai](https://docs.obot.ai).

## Layout

- `docs/` contains the current, unreleased documentation and is published under `/next/`.
- `versioned_docs/version-vX.Y.Z/` contains snapshots of released documentation.
- `versioned_sidebars/` contains the sidebar snapshot for each released version.
- `static/` contains images and downloads shared by every version.
- `versions.json` lists released versions. The first entry is the latest release and is published at the site root.

Make ordinary documentation changes in `docs/`. Treat files in `versioned_docs/` as release snapshots and only backport corrections that would otherwise mislead users of that release.

## Local development

Run all `make` commands from the root of the Obot repository, not from this `docs/` directory.

To install the documentation dependencies and start the development server:

```bash
make serve-docs
```

Most changes are reflected in the browser without restarting the server.

## Build

The documentation workflow uses npm. To run the same build used by CI:

```bash
cd docs
npm install
npm run build
```

The generated site is written to `docs/build/`. Broken documentation links fail the build.

## Links

Use relative links, including the `.md` extension, when linking to another documentation page:

```markdown
[MCP Servers](./mcp-servers.md)
```

Relative links resolve within the current documentation version. Do not use site-root paths such as `/functionality/mcp-servers/` for links between documentation pages.

Files under `static/` are shared by every version, so images and downloads should use absolute paths:

```markdown
![Add a server](/img/add-mcp-server-type-selector.png)
```

## Release versions

Run all version-management commands from the root of the Obot repository.

To snapshot the current documentation for a new release:

```bash
make gen-docs-release version=v0.26.0
```

Include the `v` prefix. This command updates `versions.json` and creates:

- `docs/versioned_docs/version-v0.26.0/`
- `docs/versioned_sidebars/version-v0.26.0-sidebars.json`

The Docusaurus configuration derives the latest release and version menu from `versions.json`; do not update `lastVersion` manually.

Keep the latest release and the three releases immediately preceding it. After creating a release snapshot, remove the oldest snapshot when necessary:

```bash
make remove-docs-version version=v0.22.0
```

The removal command requires `jq` on the host. Review the generated changes, run the documentation build, and then commit the updated snapshots, sidebar files, and `versions.json`.

The current documentation is available at `https://docs.obot.ai/next/`, the latest release at `https://docs.obot.ai/`, and older releases at paths such as `https://docs.obot.ai/v0.24.0/`.
