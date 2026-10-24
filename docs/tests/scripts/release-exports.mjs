// Exercises the actual Docusaurus release command in an isolated copy.
import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { spawnSync } from "node:child_process";
import assert from "node:assert/strict";
const site = process.cwd();
const temp = await fs.mkdtemp(path.join(os.tmpdir(), "obot-docs-release-"));
const run = (args) => {
  const result = spawnSync(
    process.execPath,
    [
      path.join(site, "node_modules/@docusaurus/core/bin/docusaurus.mjs"),
      ...args,
    ],
    {
      cwd: temp,
      stdio: "inherit",
      env: { ...process.env, DOCS_SITE_URL: "http://localhost:3001" },
    },
  );
  if (result.status !== 0)
    throw new Error(`Docusaurus ${args.join(" ")} failed`);
};
try {
  await fs.cp(site, temp, {
    recursive: true,
    filter: (src) =>
      ![
        "node_modules",
        "build",
        ".docusaurus",
        ".wrangler",
        "playwright-report",
        "test-results",
      ].includes(path.relative(site, src).split(path.sep)[0]),
  });
  await fs.symlink(
    path.join(site, "node_modules"),
    path.join(temp, "node_modules"),
    "dir",
  );
  const original = JSON.parse(
    await fs.readFile(path.join(temp, "versions.json"), "utf8"),
  );
  const next = "v0.28.0";
  assert.ok(!original.includes(next), "Use an unreleased fixture version");
  // Exercise real metadata/theme behavior for an HTML-only unlisted page.
  await fs.writeFile(
    path.join(temp, "docs", "export-unlisted-fixture.md"),
    "---\ntitle: Unlisted export fixture\nunlisted: true\n---\n\nThis HTML page must not advertise a Markdown export.\n",
  );
  run(["docs:version", next]);
  const dropped = original.at(-1);
  const versions = JSON.parse(
    await fs.readFile(path.join(temp, "versions.json"), "utf8"),
  ).filter((v) => v !== dropped);
  await fs.writeFile(
    path.join(temp, "versions.json"),
    JSON.stringify(versions),
  );
  await fs.rm(path.join(temp, "versioned_docs", `version-${dropped}`), {
    recursive: true,
  });
  await fs.rm(
    path.join(temp, "versioned_sidebars", `version-${dropped}-sidebars.json`),
  );
  // Seed stale files: a real production rebuild must remove these itself.
  await fs.mkdir(path.join(temp, "build", dropped), { recursive: true });
  await fs.writeFile(path.join(temp, "build", dropped, "llms.txt"), "stale");
  await fs.writeFile(path.join(temp, "build", "stale.md"), "stale");
  run(["build"]);
  const read = (p) => fs.readFile(path.join(temp, "build", p), "utf8");
  assert.match(await read("llms.txt"), /Use v0\.28\.0/);
  assert.equal(await read("index.md"), await read(`${next}/index.md`));
  assert.match(
    await read(`${next}/index.md`),
    /canonical: "http:\/\/localhost:3001\/"/,
  );
  assert.match(
    await read(`${original[0]}/index.md`),
    new RegExp(`canonical: "http://localhost:3001/${original[0]}/"`),
  );
  assert.ok(
    (await read(`${original[0]}/index.md`)).includes(
      `/${original[0]}/installation/overview.md`,
    ),
  );
  assert.ok(!(await read("llms.txt")).includes(dropped));
  await assert.rejects(
    fs.access(path.join(temp, "build", dropped, "llms.txt")),
  );
  await assert.rejects(fs.access(path.join(temp, "build", "stale.md")));
  // The new release has main's unified gateway; old release retains provider routes.
  assert.ok(
    (await read(`${next}/llm-gateway/connect-clients.md`)).includes(
      "/api/llm-proxy/v1",
    ),
  );
  assert.ok(
    (await read(`${original[0]}/llm-gateway/connect-clients.md`)).includes(
      "/azure-entra/v1/messages",
    ),
  );
  assert.match(await read("next/index.md"), /unreleased: true/);
  for (const prefix of ["", "next/"]) {
    const html = await read(`${prefix}export-unlisted-fixture/index.html`);
    assert.ok(html.includes("This HTML page must not advertise"));
    assert.ok(!html.includes("View as Markdown"));
    assert.ok(!html.includes('type="text/markdown"'));
  }
  for (const prefix of ["", "next/", `${next}/`])
    await assert.rejects(
      fs.access(
        path.join(temp, "build", `${prefix}export-unlisted-fixture.md`),
      ),
    );
  console.log(
    `PASS: ${next} snapshot, ${original[0]} rollover, ${dropped} removal, stale-file cleanup, and gateway upgrade isolation. Copy: ${temp}`,
  );
} finally {
  if (process.env.KEEP_RELEASE_FIXTURE !== "1")
    await fs.rm(temp, { recursive: true, force: true });
}
