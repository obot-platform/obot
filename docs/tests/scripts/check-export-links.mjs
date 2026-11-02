// Check every documentation URL published in llms.txt, llms-full.txt and the
// Markdown exports against the built site, so agents never receive a link that
// 404s. Independent expectations: do not import exporter transformations or URL
// helpers.
import fs from "node:fs/promises";
import path from "node:path";
import assert from "node:assert/strict";

const build = path.resolve("build");
// Match docusaurus.config.ts so local-origin preview builds verify too.
const origin = new URL(process.env.DOCS_SITE_URL || "https://docs.obot.ai")
  .origin;
const originPattern = origin.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const publishedUrl = new RegExp(`${originPattern}[^\\s)"'<>\\]]*`, "g");
const versionIndexUrl = new RegExp(
  `${originPattern}/([^/\\s)]+)/llms\\.txt`,
  "g",
);
const released = JSON.parse(await fs.readFile("versions.json", "utf8"));
const versions = [...released, "next"];
const latest = released[0] ?? "next";

async function isFile(file) {
  try {
    return (await fs.stat(file)).isFile();
  } catch {
    return false;
  }
}
async function files(dir) {
  const result = [];
  for (const entry of await fs.readdir(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) result.push(...(await files(file)));
    else result.push(file);
  }
  return result;
}
// Mirror static hosting: an exact file, a directory index, or an extensionless page.
async function resolves(pathname) {
  const file = path.join(build, decodeURIComponent(pathname));
  if (!file.startsWith(`${build}${path.sep}`)) return false;
  return (
    (await isFile(file)) ||
    (await isFile(path.join(file, "index.html"))) ||
    (await isFile(`${file.replace(/\/$/, "")}.html`))
  );
}
const versionOf = (pathname) =>
  versions.find((v) => pathname.startsWith(`/${v}/`));

const published = (await files(build)).filter(
  (f) => f.endsWith(".md") || /(^|\/)llms(-full)?\.txt$/.test(f),
);
const failures = [];
const listed = new Map(versions.map((v) => [v, new Set()]));
let checked = 0;
for (const file of published) {
  const relative = `/${path.relative(build, file).split(path.sep).join("/")}`;
  // Root Markdown aliases are copies of the latest release.
  const owner =
    relative === "/llms.txt" ? null : (versionOf(relative) ?? latest);
  const text = await fs.readFile(file, "utf8");
  for (const [raw] of text.matchAll(publishedUrl)) {
    checked++;
    const { pathname } = new URL(raw, origin);
    const problem = (reason) => failures.push(`${relative}: ${reason}: ${raw}`);
    if (/\.md\.md$|\.html\.md$|\.md\/$/.test(pathname))
      problem("malformed Markdown suffix");
    if (!(await resolves(pathname)))
      problem("does not resolve to a built file");
    const target = versionOf(pathname);
    if (owner && target && target !== owner) problem("links another version");
    if (owner && pathname.endsWith(".md") && target !== owner)
      problem(`Markdown link outside /${owner}/`);
    if (relative === `/${owner}/llms.txt` && pathname.endsWith(".md"))
      listed.get(owner).add(pathname);
  }
}

assert.ok(checked > 0, "No published documentation URLs found");
assert.deepEqual(
  failures,
  [],
  `Broken published links:\n${failures.join("\n")}`,
);

// Clients that only try the conventional root path get the latest release.
assert.equal(
  await fs.readFile(path.join(build, "llms-full.txt"), "utf8"),
  await fs.readFile(path.join(build, latest, "llms-full.txt"), "utf8"),
  `Root llms-full.txt must match ${latest}/llms-full.txt`,
);

// The root index must offer exactly the retained versions.
const root = await fs.readFile(path.join(build, "llms.txt"), "utf8");
assert.deepEqual(
  Array.from(root.matchAll(versionIndexUrl), (m) => m[1]).sort(),
  [...versions].sort(),
  "Root llms.txt must link every retained version's llms.txt",
);
for (const version of versions) {
  const index = await fs.readFile(
    path.join(build, version, "llms.txt"),
    "utf8",
  );
  assert.ok(
    index.includes(`${origin}/${version}/llms-full.txt`),
    `${version}/llms.txt must link its llms-full.txt`,
  );
  // Every exported page of a version must be discoverable from its index.
  const exported = (await files(path.join(build, version)))
    .filter((f) => f.endsWith(".md"))
    .map((f) => `/${path.relative(build, f).split(path.sep).join("/")}`)
    .sort();
  assert.deepEqual(
    [...listed.get(version)].sort(),
    exported,
    `${version}/llms.txt must list exactly the exported Markdown pages`,
  );
}

console.log(
  `Verified ${checked} published URLs in ${published.length} files across ${versions.length} versions.`,
);
