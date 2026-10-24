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

The generated site is written to `docs/build/`. Verification clears generated metadata and build output before running TypeScript
checking and the Docusaurus production build. This prevents deleted pages from
leaving stale metadata in local corpus checks. Broken internal links, Markdown
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

## Agent-readable documentation

The local `plugins/agent-docs` plugin exports source Markdown using the docs
plugin's resolved version and route metadata. It runs during production builds;
there is no runtime service or additional release step.

- `/llms.txt` identifies the latest release and links to retained version indexes.
- `/vX.Y.Z/llms.txt` and `/next/llms.txt` organize that version's Markdown by sidebar
  topic. `/next/` is explicitly unreleased.
- `/vX.Y.Z/<route>.md` pins a document to a release; a version homepage uses
  `index.md`. `/installation/` maps to `installation.md`, not
  `installation/index.md`. Root Markdown aliases serve the latest release's exact
  exported bytes, including version-pinned links. Heading and explicit-anchor fragments
  are rewritten to collision-free export IDs so duplicate headings and generated titles
  cannot change a link destination.
- Each page includes YAML `title`, `version`, `canonical` (that version's HTML
  URL), and `unreleased`. This source URL is independent of the HTML SEO canonical.
- Each version index links to `llms-full.txt`, with its byte size and an approximate
  token count (bytes divided by four, not a tokenizer guarantee). Individual pages
  are the default retrieval path.

Pages receive an alternate Markdown head link and a “View as Markdown” link that
follows client navigation. The footer links to the root index. Cloudflare Pages
headers give Markdown exports `text/markdown` and `noindex`; full bundles and
older/unreleased indexes are also excluded from indexing. Keep exports crawlable
in `robots.txt` so crawlers can read those headers. Existing HTML indexing rules
are unchanged. Missing Markdown URLs remain 404s.

The normalizer supports Markdown, GFM tables, explicit heading anchors, Mermaid,
Docusaurus admonitions, Tabs/TabItem, and the ordinary HTML elements allowlisted
in `markdown.mjs`. It scans all retained versions, preserves fenced code, and
fails on unsupported MDX components/expressions/imports. Add a focused fixture
before extending it. Draft/unlisted documents, partials, generated category
pages, and HTML redirect stubs are not exported. Local document links and anchors,
asset targets, output collisions, and effective robots rules are validated.

Review `.docusaurus/agent-docs-summaries.json` after content changes. It records
all summaries and whether they were extracted from prose. Add authored frontmatter
descriptions to current docs, or use a small entry in
`plugins/agent-docs/summaries.json` for an index-only correction to frozen release
content. Version-specific overrides take precedence over shared (`*`) overrides.
The build abbreviates long summaries to keep indexes useful.

Run `npm run test:exports` for the Node test suite (also part of `verify` and CI).
After a build, `npm run test:exports:content` compares every exported page's code
blocks and table text with its unmodified source using independent test helpers,
and checks the frontmatter schema and version/HTML URL metadata (also part of
`verify` and CI). This does not establish complete prose or formatting fidelity.
Unit tests also compare clean-build output bytes and reject missing assets.
Run `npm run test:exports:release` for an isolated v0.28.0 snapshot, previous-latest
rollover, oldest-version removal, and clean rebuild. This invokes Docusaurus's
actual version command without modifying this checkout. Update the fixture version
once v0.28.0 is released. `KEEP_RELEASE_FIXTURE=1` retains the temporary copy for
inspection. Browser tests verify headers, raw server-rendered discovery links, and Markdown
links after in-app navigation.

For a preview whose generated absolute links stay local, build with
`DOCS_SITE_URL=http://localhost:3000 npm run build`, then run `npm run preview:pages`.
Production builds default to `https://docs.obot.ai`. No Docker or Obot application
instance is needed for documentation tests. A preview cannot verify production
Cloudflare bot/WAF policies.

Community alternatives include `docusaurus-plugin-llms` (which supports multiple
versions) and `@signalwire/docusaurus-plugin-llms-txt`. The local implementation
keeps Obot's exact pinned/alias URL contract, source-preserving conversion,
fail-on-unknown behavior, and release regression checks together. The former's
configurable content cleaning and the latter's rendered-HTML conversion do not
provide this exact validation contract out of the box. Reassess them if maintaining
this small adapter becomes more costly. Publishing these files does not guarantee
automatic agent discovery or improved search rankings; evaluate retrieval and
homepage-only discovery separately.

### Verification boundaries and acceptance evidence

Docusaurus 3.10.2 is the existing pinned version, not an upgrade made by this
change. Run verification, browser tests, and the isolated release test when
upgrading it; current results do not certify future versions.

`tests/agent-docs/acceptance-results.json` is a historical summary. Its original
answers and exact model identifier were not retained, so it cannot be independently
rescored. Do not rewrite historical counts to match today's suite. For new runs,
retain the exact prompt, entry URL, model identifier (or explicitly unknown),
complete answer with citations, fetched URLs, expected criteria written before
execution, and a criterion-by-criterion assessment. Keep unsupported discovery
or source-content claims separate from exporter defects. Never reconstruct a
missing transcript from an assessment summary.

The exporter processes trusted repository sources; it is not an untrusted-Markdown
sanitizer. Local tests cannot establish production Cloudflare bot/WAF access.
After deployment, check the public indexes, Markdown MIME/noindex headers, 404s,
and provider access/logs; record the date and client rather than claiming universal
agent discovery. Dependency audits are point-in-time checks, not a security guarantee.

The 2026-10-24 QA audit found one newly affected dependency in the exporter’s
`gray-matter` dependency; its scoped `js-yaml` override was patched to 3.15.2.
The existing docs stack still has 26 npm audit findings (20 low, 3 moderate,
2 high, 1 critical), also present on the base branch. Advisory roots include
`body-parser`, `dompurify`, `fast-uri`, `serialize-javascript`, and `undici`.
These need separate dependency maintenance; the exporter review does not establish
that they are harmless. Avoid `npm audit fix --force`, whose suggested changes
include downgrading the pinned Docusaurus/Wrangler stack.
