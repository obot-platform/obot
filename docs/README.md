# Obot documentation

The Obot documentation site is built with Docusaurus 3 and published at [docs.obot.ai](https://docs.obot.ai).

## Layout

- `docs/` contains the current, unreleased documentation and is published under `/next/`.
- `versioned_docs/version-vX.Y.Z/` contains snapshots of released documentation.
- `versioned_sidebars/` contains the sidebar snapshot for each released version.
- `static/` contains images and downloads shared by every version.
- `versions.json` lists released versions. The first entry is the latest release and is published at the site root.

Make ordinary documentation changes in `docs/`. Treat files in `versioned_docs/` as release snapshots and only backport corrections that would otherwise mislead users of that release.

Keep unfinished guides in their normal locations under `docs/` with `draft: true` in their YAML frontmatter. Docusaurus shows drafts during local development and excludes them from production builds. Leave drafts out of `sidebars.ts` until they are ready to publish.

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
npm ci
npm run verify
```

The generated site is written to `docs/build/`. Verification runs TypeScript
checking and the Docusaurus production build. Broken internal links, Markdown
links, and heading anchors fail the build.

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

## Browser tests

After `npm run verify`, run `npm run test:docs` from this directory. The headed
Chromium suite checks sidebar navigation and direct loads, section links,
category controls, and the active released versions. It
starts a local Wrangler Pages preview on port 3000.

Install Chromium with `npx playwright install chromium`, or set `CHROMIUM_PATH`
to an installed executable. On a displayless Linux host, run
`xvfb-run --auto-servernum npm run test:docs`.

To check an existing preview, use:

```bash
DOCS_BASE_URL=https://<preview-origin> npm run test:docs
```

Reports, failure screenshots, and traces are generated in `playwright-report/`
and `test-results/`; CI uploads them as artifacts. They are not committed.

## Product smoke tests

Run commands from `docs/`:

1. Start a fresh Docker instance using [Local owner setup](./docs/installation/docker-deployment.md#preconfigure-a-local-owner),
   using the image you intend to verify. Activate the account.
2. Start `node tests/fixtures/mcp-server.mjs`. Register its `/mcp` endpoint on port
   8090 using an address reachable from the container. For a local private-network fixture, the test container may require
   `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP=false`. Limit that exception to this
   isolated test environment.
3. Follow the governance quickstart to create **Docs managed tools**, choose
   **Managed**, and enable only `echo`. Record the vMCP ID from its URL.
4. Use `npx playwright codegen --save-storage=/tmp/obot-docs-auth.json http://localhost:8080`
   to sign in with the Local owner and save browser state, then close the browser.
   Keep that file private and outside the repository.
5. Run the headed test (on a displayless CI host, prefix it with `xvfb-run --auto-servernum`):

```bash
OBOT_BASE_URL=http://localhost:8080 \
OBOT_STORAGE_STATE=/tmp/obot-docs-auth.json \
OBOT_TEST_VMCP_ID=<your-vmcp-id> \
npm run test:obot
```

Use `CHROMIUM_PATH=/usr/bin/chromium` when using the system browser; otherwise
install Playwright Chromium with `npx playwright install chromium`.
Product tests run separately from documentation CI and use an explicitly supplied
test instance. They do not store product traces or credentials in the repository.
The test browser uses UTC so audit timestamp assertions are consistent across
developer machines; this setting does not change the application's timezone.

These tests verify UI entry points, managed tool discovery, a successful tool
call, and its audit record. They do not validate Kubernetes deployments, cloud
services, hosted-agent execution, or live LLM compatibility.
