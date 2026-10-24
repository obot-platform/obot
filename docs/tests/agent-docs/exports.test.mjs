import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import {
  parseMarkdown,
  render,
  walk,
} from "../../plugins/agent-docs/markdown.mjs";
import {
  generate,
  markdownPath,
  assertCrawlable,
} from "../../plugins/agent-docs/index.mjs";

test("preserves fences, all tabs, explicit IDs, Mermaid, tables and admonitions", () => {
  const source = `---\ntitle: Test\n---\nimport Tabs from '@theme/Tabs';\nimport TabItem from '@theme/TabItem';\n\n# Heading {#stable}\n\nUseful prose explaining how to perform this task.\n\n<Tabs>\n<TabItem value="a" label="First" default>\n\n~~~python\nimport os\nprint("<Unknown />")\n~~~\n\n</TabItem>\n<TabItem value="b" label="Second">\n\nSecond alternative.\n\n</TabItem>\n</Tabs>\n\n:::warning Important\nDo not lose this warning.\n:::\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n\`\`\`mermaid\ngraph LR\nA-->B\n\`\`\`\n`;
  const parsed = parseMarkdown(source, "Test");
  const output = render(parsed.tree);
  assert.ok(parsed.anchors.has("stable"));
  for (const value of [
    "import os",
    "<Unknown />",
    "First",
    "Second",
    "WARNING",
    "Do not lose",
    "graph LR",
    "A-->B",
  ])
    assert.ok(output.includes(value), value);
  assert.ok(!output.includes("@theme"));
  assert.equal(
    parsed.summary,
    "Useful prose explaining how to perform this task.",
  );
  assert.throws(
    () => parseMarkdown("<Mystery />", "Test"),
    /Unsupported component/,
  );
  assert.throws(
    () => parseMarkdown('import X from "external";', "Test"),
    /Unsupported MDX/,
  );
});

test("URLs derive from resolved slugs, with explicit index convention", () => {
  assert.equal(
    markdownPath("v0.28.0", "/installation/"),
    "/v0.28.0/installation.md",
  );
  assert.equal(markdownPath("current", "/"), "/next/index.md");
  assert.equal(markdownPath("", "/"), "/index.md");
  assert.equal(
    markdownPath("v0.28.0", "/custom-slug/", "/docs/"),
    "/docs/v0.28.0/custom-slug.md",
  );
  assert.throws(() => markdownPath("v0.28.0", "/%2e%2e/evil"), /Unsafe/);
});

test("robots permits retrieval of noindex exports, including specific groups", () => {
  const urls = ["/v0.27.0/page.md", "/v0.27.0/llms-full.txt"];
  assertCrawlable("User-agent: *\n", urls, "https://docs.example.com");
  for (const rules of [
    "User-agent: *\nDisallow: /*.md",
    "User-agent: OAI-SearchBot\nDisallow: /v0.27.0/",
    "User-agent: CustomBot\nDisallow: /*/llms-full.txt",
  ])
    assert.throws(
      () => assertCrawlable(rules, urls, "https://docs.example.com"),
      /blocks/,
    );
});

async function fixture(t, names = ["current", "v0.27.0", "v0.26.0"]) {
  const siteDir = await fs.mkdtemp(path.join(os.tmpdir(), "agent-docs-"));
  t.after(() => fs.rm(siteDir, { recursive: true, force: true }));
  const outDir = path.join(siteDir, "build"),
    generatedFilesDir = path.join(siteDir, ".docusaurus");
  await fs.mkdir(outDir);
  await fs.mkdir(generatedFilesDir);
  await fs.writeFile(
    path.join(outDir, "_headers"),
    "/*.md\n  X-Robots-Tag: noindex\n",
  );
  await fs.writeFile(path.join(outDir, "robots.txt"), "User-agent: *\n");
  const versions = [];
  for (const versionName of names) {
    const dir = path.join(siteDir, versionName);
    await fs.mkdir(dir);
    const isLast = versionName === names[1];
    const prefix = isLast
      ? ""
      : versionName === "current"
        ? "/next"
        : `/${versionName}`;
    const docs = [];
    for (const [id, slug, body] of [
      [
        "overview",
        "/",
        "# Home\n\nA useful introduction for this release.\n\n[Guide](./01-guide.md#stable)",
      ],
      [
        "01-guide",
        "/custom/",
        "# Guide {#stable}\n\nSpecific guide for " +
          versionName +
          " with a useful explanation.\n\n[Home](./overview.md)",
      ],
    ]) {
      const source = `@site/${versionName}/${id}.md`;
      await fs.writeFile(path.join(dir, `${id}.md`), body);
      const permalink = `${prefix}${slug}`;
      await fs.mkdir(path.join(outDir, permalink), { recursive: true });
      await fs.writeFile(
        path.join(outDir, permalink, "index.html"),
        "<article>Unchanged</article>",
      );
      docs.push({
        id,
        slug,
        source,
        permalink,
        version: versionName,
        title: id,
      });
    }
    versions.push({
      versionName,
      isLast,
      docs,
      sidebars: {
        sidebar: [
          {
            type: "category",
            label: "Start",
            items: docs.map((d) => ({ type: "doc", id: d.id })),
          },
        ],
      },
    });
  }
  return {
    versions,
    outDir,
    siteDir,
    generatedFilesDir,
    siteConfig: { url: "https://docs.example.com", baseUrl: "/" },
  };
}

test("v0.28 release rollover keeps versioned links stable and rolls latest aliases", async (t) => {
  for (const names of [
    ["current", "v0.27.0", "v0.26.0"],
    ["current", "v0.28.0", "v0.27.0"],
  ]) {
    const f = await fixture(t, names);
    await generate(f);
    const latest = names[1];
    const root = await fs.readFile(path.join(f.outDir, "llms.txt"), "utf8");
    assert.ok(root.includes(`Use ${latest}`));
    const pinned = await fs.readFile(
      path.join(f.outDir, latest, "index.md"),
      "utf8",
    );
    assert.equal(
      await fs.readFile(path.join(f.outDir, "index.md"), "utf8"),
      pinned,
    );
    assert.ok(pinned.includes(`/${latest}/custom.md#obot-doc-stable`));
    assert.ok(pinned.includes('canonical: "https://docs.example.com/"'));
    const previous = await fs.readFile(
      path.join(f.outDir, names[2], "index.md"),
      "utf8",
    );
    assert.ok(
      previous.includes(`canonical: "https://docs.example.com/${names[2]}/"`),
    );
    assert.ok(!previous.includes(`/${latest}/custom.md`));
    assert.ok(
      (
        await fs.readFile(path.join(f.outDir, latest, "llms.txt"), "utf8")
      ).includes("bytes; approximately"),
    );
    assert.ok(
      (
        await fs.readFile(path.join(f.outDir, "next/index.md"), "utf8")
      ).includes("unreleased: true"),
    );
    assert.equal(
      await fs.readFile(path.join(f.outDir, "index.html"), "utf8"),
      "<article>Unchanged</article>",
    );
  }
});

test("missing links, fragments, cross-version links and static collisions fail", async (t) => {
  for (const [body, error] of [
    ["[bad](./missing.md)", /Missing document/],
    ["[bad](./01-guide.md#missing)", /Missing anchor/],
    ["[bad](/v0.26.0/custom/)", /Cross-version/],
  ]) {
    const f = await fixture(t);
    await fs.writeFile(path.join(f.siteDir, "v0.27.0/overview.md"), body);
    await assert.rejects(generate(f), error);
  }
  const f = await fixture(t);
  await fs.writeFile(path.join(f.outDir, "index.md"), "owned by static");
  await assert.rejects(generate(f), /overwrite/);
});

test("drafts and unlisted docs stay out of indexes and exports", async (t) => {
  const f = await fixture(t);
  f.versions[1].docs.push(
    { id: "draft", title: "Draft", draft: true, source: "@site/missing.md" },
    {
      id: "unlisted",
      title: "Unlisted",
      unlisted: true,
      source: "@site/missing.md",
    },
  );
  await generate(f);
  const index = await fs.readFile(
    path.join(f.outDir, "v0.27.0/llms.txt"),
    "utf8",
  );
  assert.ok(!index.includes("Draft"));
  assert.ok(!index.includes("Unlisted"));
  const report = JSON.parse(
    await fs.readFile(
      path.join(f.generatedFilesDir, "agent-docs-summaries.json"),
      "utf8",
    ),
  );
  assert.equal(report.length, 6);
});

test("queries, fragments, external URLs and assets survive link rewriting", async (t) => {
  const f = await fixture(t);
  await fs.mkdir(path.join(f.outDir, "img"));
  await fs.writeFile(path.join(f.outDir, "img/example.png"), "fixture");
  await fs.writeFile(
    path.join(f.siteDir, "v0.27.0/01-guide.md"),
    `# Guide {#stable}\n\nA useful guide with links for testing.\n\n[Here](#stable)\n[Home](./overview.md?mode=test)\n[External](https://example.org/path?q=x#y)\n![Diagram](/img/example.png)\n`,
  );
  await generate(f);
  const page = await fs.readFile(
    path.join(f.outDir, "v0.27.0/custom.md"),
    "utf8",
  );
  for (const url of [
    "https://docs.example.com/v0.27.0/custom.md#obot-doc-stable",
    "https://docs.example.com/v0.27.0/index.md?mode=test",
    "https://example.org/path?q=x#y",
    "https://docs.example.com/img/example.png",
  ])
    assert.ok(page.includes(url), url);
  const bundle = await fs.readFile(
    path.join(f.outDir, "v0.27.0/llms-full.txt"),
    "utf8",
  );
  assert.ok(
    bundle.includes(
      "https://docs.example.com/v0.27.0/custom.md#obot-doc-stable",
    ),
  );
});

test("fence-like content stays literal, including nested Markdown examples", () => {
  const code = "````not-a-close\n# Keep {#id}\n:::warning Literal title";
  for (const fence of ["```", "~~~"]) {
    const source = `${fence}markdown\n${code}\n${fence}\n\n# Real {#real}`;
    const parsed = parseMarkdown(source, "Test");
    const blocks = [];
    walk(parsed.tree, (node) => {
      if (node.type === "code") blocks.push(node.value);
    });
    assert.deepEqual(blocks, [code]);
    assert.ok(parsed.anchors.has("real"));
    assert.ok(!parsed.anchors.has("id"));
  }
});

test("inline HTML heading text resolves incoming links and duplicate anchors", async (t) => {
  const f = await fixture(t);
  await fs.writeFile(
    path.join(f.siteDir, "v0.27.0/01-guide.md"),
    "# Guide {#stable}\n\n## Heading <code>foo</code>\n\nFirst.\n\n## Heading <code>foo</code>\n\nSecond.",
  );
  await fs.writeFile(
    path.join(f.siteDir, "v0.27.0/overview.md"),
    "[First](./01-guide.md#heading-foo)\n\n[Second](./01-guide.md#heading-foo-1)",
  );
  await generate(f);
  const output = await fs.readFile(
    path.join(f.outDir, "v0.27.0/index.md"),
    "utf8",
  );
  assert.ok(output.includes("/v0.27.0/custom.md#obot-doc-heading-foo)"));
  assert.ok(output.includes("/v0.27.0/custom.md#obot-doc-heading-foo-1)"));
});

test("fragment destinations survive explicit IDs, synthetic titles and namespace collisions", async (t) => {
  for (const [body, fragment, destination] of [
    ["# Foo {#custom}\n\n## Foo\n\nSection.", "foo", "## Foo"],
    ["## Foo\n\nSection.", "foo", "## Foo"],
    ["# Obot <code>doc foo</code>\n\n## Foo\n\nSection.", "foo", "## Foo"],
    ['Intro.\n\n<a id="foo"></a>\n\nTarget.', "foo", "</a>"],
  ]) {
    const parsed = parseMarkdown(body, "Foo");
    const output = render(parsed.tree);
    const target = parsed.fragmentTargets.get(fragment);
    assert.ok(target);
    assert.ok(output.includes(`id="${target}"`));
    if (destination !== "</a>")
      assert.ok(output.includes(`<a id="${target}"></a>\n\n${destination}`));
    if (body.startsWith("# Obot")) assert.equal(target, "_obot-doc-foo");
    assert.equal(output.split(`id="${target}"`).length - 1, 1);
    const f = await fixture(t);
    f.versions[1].docs[1].title = "Foo";
    await fs.writeFile(path.join(f.siteDir, "v0.27.0/01-guide.md"), body);
    await fs.writeFile(
      path.join(f.siteDir, "v0.27.0/overview.md"),
      `[Section](./01-guide.md#${fragment})`,
    );
    await generate(f);
    assert.ok(
      (
        await fs.readFile(path.join(f.outDir, "v0.27.0/index.md"), "utf8")
      ).includes(`custom.md#${target}`),
    );
  }
});

test("exports are byte-identical across clean builds and missing assets fail", async (t) => {
  const first = await fixture(t),
    second = await fixture(t);
  await generate(first);
  await generate(second);
  async function files(root, prefix = "") {
    const entries = await fs.readdir(path.join(root, prefix), {
      withFileTypes: true,
    });
    return (
      await Promise.all(
        entries.map(async (entry) =>
          entry.isDirectory()
            ? files(root, path.join(prefix, entry.name))
            : path.join(prefix, entry.name),
        ),
      )
    )
      .flat()
      .sort();
  }
  const paths = await files(first.outDir);
  assert.deepEqual(paths, await files(second.outDir));
  for (const file of paths)
    assert.deepEqual(
      await fs.readFile(path.join(first.outDir, file)),
      await fs.readFile(path.join(second.outDir, file)),
      file,
    );
  const missing = await fixture(t);
  await fs.writeFile(
    path.join(missing.siteDir, "v0.27.0/overview.md"),
    "![Missing](/img/not-here.png)",
  );
  await assert.rejects(generate(missing), /Missing HTML\/asset target/);
});
