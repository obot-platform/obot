// Compare the built corpus with source code blocks and table cell text. This also
// detects generated-metadata layout changes when upgrading Docusaurus.
import fs from "node:fs/promises";
import assert from "node:assert/strict";
import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import matter from "gray-matter";
// Independent expectations: do not import exporter transformations or URL helpers.
function walk(node, visit) {
  visit(node);
  for (const child of node.children || []) walk(child, visit);
}
function plainText(node) {
  if (node.type === "image") return node.alt || "";
  return node.children
    ? node.children.map(plainText).join("")
    : node.value || "";
}

const sourceParser = unified().use(remarkParse).use(remarkGfm);
const exportParser = unified().use(remarkParse).use(remarkGfm);
function preservedContent(tree) {
  const result = { code: [], table: [] };
  walk(tree, (node) => {
    if (node.type === "code")
      result.code.push([node.lang, node.meta, node.value]);
    if (node.type === "table")
      result.table.push(
        node.children.map((row) =>
          // GFM pads omitted trailing cells; compare their semantic values.
          Array.from({ length: node.children[0].children.length }, (_, i) =>
            row.children[i] ? plainText(row.children[i]) : "",
          ),
        ),
      );
  });
  return result;
}
let pages = 0,
  code = 0,
  tables = 0;
const metadataDir = ".docusaurus/docusaurus-plugin-content-docs/default";
for (const name of await fs.readdir(metadataDir)) {
  if (!name.endsWith(".json")) continue;
  const doc = JSON.parse(await fs.readFile(`${metadataDir}/${name}`, "utf8"));
  if (!doc.source || !doc.slug || !doc.version || doc.draft || doc.unlisted)
    continue;
  const source = matter(
    await fs.readFile(doc.source.replace("@site/", ""), "utf8"),
  ).content;
  const version = doc.version === "current" ? "next" : doc.version;
  const route = doc.slug.replace(/^\/+|\/+$/g, "") || "index";
  const exportedFile = matter(
    await fs.readFile(`build/${version}/${route}.md`, "utf8"),
  );
  assert.deepEqual(Object.keys(exportedFile.data).sort(), [
    "canonical",
    "title",
    "unreleased",
    "version",
  ]);
  assert.equal(exportedFile.data.version, version);
  assert.equal(exportedFile.data.title, doc.title);
  assert.equal(exportedFile.data.unreleased, version === "next");
  assert.equal(new URL(exportedFile.data.canonical).pathname, doc.permalink);
  const exported = exportedFile.content;
  const before = preservedContent(sourceParser.parse(source));
  const after = preservedContent(exportParser.parse(exported));
  assert.deepEqual(after, before, doc.source);
  pages++;
  code += before.code.length;
  tables += before.table.length;
}
const report = JSON.parse(
  await fs.readFile(".docusaurus/agent-docs-summaries.json", "utf8"),
);
assert.ok(pages > 0);
assert.equal(
  pages,
  report.length,
  "Every exported source document must be checked",
);
console.log(
  `Verified ${pages} pages: ${code} fenced code blocks and ${tables} tables retained.`,
);
