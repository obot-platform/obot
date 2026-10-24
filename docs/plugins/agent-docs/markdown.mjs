import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkMdx from "remark-mdx";
import remarkGfm from "remark-gfm";
import remarkDirective from "remark-directive";
import remarkStringify from "remark-stringify";
import matter from "gray-matter";
import GithubSlugger from "github-slugger";

const processor = unified()
  .use(remarkParse)
  .use(remarkMdx)
  .use(remarkGfm)
  .use(remarkDirective)
  .use(remarkStringify, { bullet: "-", fences: true });
const text = (value) => ({ type: "text", value });
export const plainText = (node) =>
  node.type === "image"
    ? node.alt || ""
    : node.children
      ? node.children.map(plainText).join("")
      : node.value || "";
export function walk(node, visit) {
  visit(node);
  for (const child of node.children || []) walk(child, visit);
}

// Explicit Docusaurus heading IDs are not MDX expressions. Protect code fences
// while recording/removing their suffix before parsing MDX.
export function prepare(source, explicitIds = new Map()) {
  const normalized = source.replace(/\r\n/g, "\n");
  const protectedLines = new Set();
  walk(unified().use(remarkParse).parse(normalized), (node) => {
    if (node.type === "code") {
      for (
        let line = node.position.start.line;
        line <= node.position.end.line;
        line++
      )
        protectedLines.add(line);
    }
  });
  return normalized
    .split("\n")
    .map((line, index) => {
      if (protectedLines.has(index + 1)) return line;
      line = line.replace(/^(\s*:{3,}[a-z]+)\s+(.+)$/, "$1[$2]");
      return line.replace(
        /^(#{1,6}\s+.*?)\s+\{#([\w-]+)\}\s*$/,
        (_, heading, id) => {
          explicitIds.set(index + 1, id);
          return heading;
        },
      );
    })
    .join("\n");
}

export function parseMarkdown(source, title) {
  const { content, data } = matter(source);
  const explicitIds = new Map();
  const prepared = prepare(content, explicitIds);
  const tree = processor.parse(prepared);
  // Capture semantic text before JSX becomes literal HTML serialization.
  const headingIds = new WeakMap();
  const headingText = new WeakMap();
  const sourceSlugger = new GithubSlugger();
  walk(tree, (node) => {
    if (node.type === "heading") {
      headingText.set(node, plainText(node));
      headingIds.set(
        node,
        explicitIds.get(node.position.start.line) ??
          sourceSlugger.slug(plainText(node)),
      );
    }
  });
  function normalize(parent) {
    parent.children = parent.children.flatMap((node) => {
      if (node.type === "mdxjsEsm") {
        if (
          !/^(?:import\s+\w+\s+from\s+['"]@theme\/(?:Tabs|TabItem)['"];?\s*)+$/.test(
            node.value,
          )
        )
          throw new Error(`Unsupported MDX import/export: ${node.value}`);
        return [];
      }
      if (node.type.includes("Expression"))
        throw new Error(`Unsupported MDX expression: ${node.value}`);
      if (node.children) normalize(node);
      if (node.type === "containerDirective") {
        if (
          ![
            "note",
            "tip",
            "info",
            "warning",
            "danger",
            "caution",
            "important",
          ].includes(node.name)
        )
          throw new Error(`Unsupported admonition: ${node.name}`);
        return [
          {
            type: "blockquote",
            children: [
              {
                type: "paragraph",
                children: [
                  { type: "strong", children: [text(node.name.toUpperCase())] },
                ],
              },
              ...node.children,
            ],
          },
        ];
      }
      if (node.type === "textDirective")
        return [
          text(
            prepared.slice(
              node.position.start.offset,
              node.position.end.offset,
            ),
          ),
        ];
      if (node.type.endsWith("Directive"))
        throw new Error(`Unsupported directive: ${node.name}`);
      if (
        node.type === "mdxJsxFlowElement" ||
        node.type === "mdxJsxTextElement"
      ) {
        const attrs = Object.fromEntries(
          node.attributes.map((a) => {
            if (
              a.type !== "mdxJsxAttribute" ||
              (a.value !== null && typeof a.value !== "string")
            )
              throw new Error(`Unsupported JSX attribute on ${node.name}`);
            return [a.name, a.value];
          }),
        );
        if (node.name === "Tabs") return node.children;
        if (node.name === "TabItem") {
          const label = attrs.label || attrs.value;
          if (!label) throw new Error("TabItem needs a label or value");
          return [
            {
              type: "paragraph",
              children: [{ type: "strong", children: [text(label)] }],
            },
            ...node.children,
          ];
        }
        if (
          ![
            "a",
            "br",
            "details",
            "summary",
            "img",
            "div",
            "span",
            "p",
            "strong",
            "em",
            "code",
            "kbd",
            "sup",
            "sub",
          ].includes(node.name)
        )
          throw new Error(`Unsupported component: ${node.name}`);
        if (
          Object.keys(attrs).some(
            (a) =>
              ![
                "id",
                "name",
                "href",
                "src",
                "alt",
                "title",
                "width",
                "height",
                "className",
                "style",
                "open",
              ].includes(a),
          )
        )
          throw new Error(`Unsupported HTML attributes on ${node.name}`);
        const attr = Object.entries(attrs)
          .map(
            ([k, v]) =>
              ` ${k === "className" ? "class" : k}="${String(v ?? "")
                .replaceAll("&", "&amp;")
                .replaceAll('"', "&quot;")}"`,
          )
          .join("");
        const open = { type: "html", value: `<${node.name}${attr}>` };
        if (["br", "img"].includes(node.name)) return [open];
        return [
          open,
          ...node.children,
          { type: "html", value: `</${node.name}>` },
        ];
      }
      return [node];
    });
  }
  normalize(tree);
  if (!tree.children.some((n) => n.type === "heading" && n.depth === 1))
    tree.children.unshift({
      type: "heading",
      depth: 1,
      children: [text(title)],
    });
  const anchors = new Set();
  const occupied = new Set();
  const renderedSlugger = new GithubSlugger();
  walk(tree, (node) => {
    if (node.type === "heading") {
      occupied.add(
        renderedSlugger.slug(headingText.get(node) ?? plainText(node)),
      );
      if (headingIds.has(node)) anchors.add(headingIds.get(node));
    }
    if (node.type === "html")
      for (const m of node.value.matchAll(/\bid="([^"]+)"/g)) {
        anchors.add(m[1]);
        occupied.add(m[1]);
      }
  });
  // Markdown renderers assign automatic IDs even to explicitly anchored headings.
  // Use a collision-free namespace for heading destinations and rewrite incoming
  // fragments to it; an injected title must not steal a source section's ID.
  let prefix = "obot-doc-";
  while ([...occupied].some((id) => id.startsWith(prefix)))
    prefix = `_${prefix}`;
  const fragmentTargets = new Map([...anchors].map((id) => [id, prefix + id]));
  walk(tree, (node) => {
    if (node.type === "html")
      node.value = node.value.replace(
        /\bid="([^"]+)"/g,
        (_, id) => `id="${fragmentTargets.get(id)}"`,
      );
  });
  function anchorHeadings(parent) {
    parent.children = parent.children.flatMap((node) => {
      if (node.children) anchorHeadings(node);
      if (!headingIds.has(node)) return [node];
      const id = headingIds.get(node);
      const target = prefix + id;
      fragmentTargets.set(id, target);
      return [
        {
          type: "html",
          value: `<a id="${target.replaceAll("&", "&amp;").replaceAll('"', "&quot;")}"></a>`,
        },
        node,
      ];
    });
  }
  anchorHeadings(tree);
  const paragraph = tree.children.find(
    (n) => n.type === "paragraph" && plainText(n).trim().length > 30,
  );
  const summary = String(
    data.description || (paragraph && plainText(paragraph)) || title,
  )
    .replace(/\s+/g, " ")
    .trim();
  return {
    tree,
    anchors,
    fragmentTargets,
    summary,
    extracted: !data.description,
  };
}
export function render(tree) {
  return processor.stringify(tree);
}
