// Compare the built corpus with source code blocks and table cell text. This also
// detects generated-metadata layout changes when upgrading Docusaurus.
import fs from "node:fs/promises";
import assert from "node:assert/strict";
import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import remarkMdx from "remark-mdx";
import remarkDirective from "remark-directive";
import matter from "gray-matter";
import {
  prepare,
  walk,
  plainText,
} from "../../plugins/agent-docs/markdown.mjs";
import { markdownPath } from "../../plugins/agent-docs/index.mjs";

const sourceParser = unified()
  .use(remarkParse)
  .use(remarkMdx)
  .use(remarkGfm)
  .use(remarkDirective);
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
  const exported = matter(
    await fs.readFile(`build${markdownPath(doc.version, doc.slug)}`, "utf8"),
  ).content;
  const before = preservedContent(sourceParser.parse(prepare(source)));
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
