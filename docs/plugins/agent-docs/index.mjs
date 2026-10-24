import fs from "node:fs/promises";
import path from "node:path";
import robotsParser from "robots-parser";
import { parseMarkdown, render, walk } from "./markdown.mjs";

const PRODUCT =
  "Obot is an open-source AI governance platform with MCP and LLM gateways, MCP hosting, MCP and skills registries, and device management.";
const label = (value) =>
  String(value)
    .replace(/[\[\]\r\n]/g, " ")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
export function markdownPath(version, slug, baseUrl = "/") {
  const route = slug.replace(/^\/+|\/+$/g, "") || "index";
  const prefix = version === "current" ? "next" : version;
  const result = `${baseUrl}${prefix ? `${prefix}/` : ""}${route}.md`;
  if (
    decodeURIComponent(result)
      .split("/")
      .some((p) => p === ".." || p === ".") ||
    /[\\?#\0]/.test(decodeURIComponent(result))
  )
    throw new Error(`Unsafe Markdown path: ${result}`);
  return result;
}
export function assertCrawlable(robots, urls, origin) {
  const parser = robotsParser(`${origin}/robots.txt`, robots);
  const agents = new Set([
    "*",
    "Googlebot",
    "OAI-SearchBot",
    "ChatGPT-User",
    "Claude-SearchBot",
    "Claude-User",
    ...Array.from(
      robots.matchAll(/^\s*user-agent:\s*([^\s#]+)/gim),
      (m) => m[1],
    ),
  ]);
  for (const url of urls)
    for (const agent of agents)
      if (parser.isAllowed(new URL(url, origin).href, agent) === false)
        throw new Error(
          `robots.txt blocks ${url} for ${agent}; crawlers must see noindex headers`,
        );
}
function sidebarOrder(sidebars) {
  const result = new Map();
  function visit(items, group = "Documentation") {
    for (const item of items) {
      if (item.type === "category") {
        if (item.link?.type === "doc" && !result.has(item.link.id))
          result.set(item.link.id, { group: item.label, order: result.size });
        visit(item.items, group === "Documentation" ? item.label : group);
      } else if (["doc", "ref"].includes(item.type) && !result.has(item.id))
        result.set(item.id, { group, order: result.size });
    }
  }
  for (const items of Object.values(sidebars)) visit(items);
  return result;
}
export async function generate({
  versions,
  outDir,
  siteDir,
  generatedFilesDir,
  siteConfig,
}) {
  const origin = siteConfig.url;
  const base = siteConfig.baseUrl;
  const latest = versions.find((v) => v.isLast);
  if (!latest) throw new Error("Missing latest documentation version");
  const overrides = JSON.parse(
    await fs.readFile(new URL("./summaries.json", import.meta.url), "utf8"),
  );
  const pages = [];
  for (const version of versions) {
    const order = sidebarOrder(version.sidebars);
    for (const doc of version.docs) {
      if (doc.draft || doc.unlisted) continue;
      const source = path.resolve(siteDir, doc.source.replace(/^@site\//, ""));
      if (!source.startsWith(`${siteDir}${path.sep}`))
        throw new Error(`Unsafe source: ${doc.source}`);
      try {
        const parsed = parseMarkdown(
          await fs.readFile(source, "utf8"),
          doc.title,
        );
        const override =
          overrides[version.versionName]?.[doc.id] || overrides["*"]?.[doc.id];
        if (override) {
          parsed.summary = override;
          parsed.extracted = false;
        }
        if (parsed.summary.length > 320) {
          const first = parsed.summary
            .match(/^.{30,}?[.!?](?:\s|$)/)?.[0]
            ?.trim();
          parsed.summary =
            first && first.length <= 320
              ? first
              : parsed.summary.slice(0, 317).replace(/\s+\S*$/, "") + "…";
        }
        pages.push({
          ...doc,
          ...parsed,
          source,
          versionInfo: version,
          md: markdownPath(version.versionName, doc.slug, base),
          group: order.get(doc.id)?.group || "Other documentation",
          order: order.get(doc.id)?.order ?? Infinity,
        });
      } catch (error) {
        throw new Error(
          `${version.versionName}: ${doc.source}: ${error.message}`,
          { cause: error },
        );
      }
    }
  }
  const bySource = new Map(pages.map((p) => [p.source, p]));
  const byHtml = new Map(
    pages.map((p) => [p.permalink.replace(/\/$/, "") || "/", p]),
  );
  const byMarkdown = new Map(pages.map((p) => [p.md, p]));
  if (byMarkdown.size !== pages.length)
    throw new Error("Colliding Markdown routes");
  const outputs = new Map();
  function put(url, content) {
    if (outputs.has(url)) throw new Error(`Output collision: ${url}`);
    outputs.set(url, content);
  }
  const assetChecks = new Set();
  function rewrite(value, page) {
    if (/^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(value)) {
      const url = new URL(value, origin);
      if (url.origin !== new URL(origin).origin) return value;
      value = url.pathname + url.search + url.hash;
    }
    const [, rawPath, query = "", hash = ""] = value.match(
      /^([^?#]*)(\?[^#]*)?(#.*)?$/,
    );
    const decoded = decodeURIComponent(rawPath);
    let target;
    if (!rawPath) target = page;
    else if (/\.mdx?$/.test(decoded) && !rawPath.startsWith("/"))
      target = bySource.get(path.resolve(path.dirname(page.source), decoded));
    else {
      const url = new URL(rawPath, new URL(page.permalink, origin));
      target =
        byHtml.get(url.pathname.replace(/\/$/, "") || "/") ||
        byMarkdown.get(url.pathname);
    }
    if (target) {
      if (target.version !== page.version)
        throw new Error(`Cross-version link ${page.source}: ${value}`);
      if (hash && !target.anchors.has(decodeURIComponent(hash.slice(1))))
        throw new Error(`Missing anchor ${page.source}: ${value}`);
      const heading =
        hash && target.fragmentTargets.get(decodeURIComponent(hash.slice(1)));
      const fragment = heading ? `#${encodeURIComponent(heading)}` : hash;
      return new URL(target.md + query + fragment, origin).href;
    }
    if (/\.mdx?$/.test(decoded))
      throw new Error(`Missing document ${page.source}: ${value}`);
    const url = new URL(value, new URL(page.permalink, origin));
    assetChecks.add(url.pathname);
    return url.href;
  }
  for (const page of pages) {
    walk(page.tree, (node) => {
      if (["link", "image", "definition"].includes(node.type))
        node.url = rewrite(node.url, page);
      if (node.type === "html")
        node.value = node.value.replace(
          /\b(href|src)="([^"]*)"/g,
          (_, attr, url) =>
            `${attr}="${rewrite(url.replaceAll("&amp;", "&"), page).replaceAll("&", "&amp;").replaceAll('"', "&quot;")}"`,
        );
    });
    page.body = render(page.tree);
    const versionName =
      page.versionInfo.versionName === "current"
        ? "next"
        : page.versionInfo.versionName;
    const metadata = {
      title: page.title,
      version: versionName,
      canonical: new URL(page.permalink, origin).href,
      unreleased: versionName === "next",
    };
    page.output = `---\n${Object.entries(metadata)
      .map(([k, v]) => `${k}: ${JSON.stringify(v)}`)
      .join("\n")}\n---\n\n${page.body}`;
    put(page.md, page.output);
    if (page.versionInfo.isLast && page.versionInfo.versionName !== "current")
      put(markdownPath("", page.slug, base), page.output);
  }
  const report = [];
  let headers = await fs.readFile(path.join(outDir, "_headers"), "utf8");
  for (const version of versions) {
    const name =
      version.versionName === "current" ? "next" : version.versionName;
    const prefix = `${base}${name}`;
    const docs = pages
      .filter((p) => p.versionInfo === version)
      .sort((a, b) => a.order - b.order || a.id.localeCompare(b.id));
    const full =
      `# Obot ${name}${name === "next" ? " (unreleased)" : ""}\n\n` +
      docs
        .map(
          (p) =>
            `---\n\nDocument: ${p.title}\nVersion: ${name}\nMarkdown: ${new URL(p.md, origin).href}\nHTML: ${new URL(p.permalink, origin).href}\n\n${p.body}`,
        )
        .join("\n");
    put(`${prefix}/llms-full.txt`, full);
    const bytes = Buffer.byteLength(full);
    let index = `# Obot ${name}${name === "next" ? " (unreleased)" : ""}\n\n> ${PRODUCT}\n\nUse examples and configuration from this version together. ${name === "next" ? "This documentation describes unreleased development against main." : `This documentation describes release ${name}.`}\n\nPrefer individual pages below. Each has version metadata and its HTML source URL.\n\n## Complete version (optional)\n\n- [Full documentation](${new URL(`${prefix}/llms-full.txt`, origin).href}): ${bytes} bytes; approximately ${Math.ceil(bytes / 4)} tokens (bytes / 4 estimate; tokenizer-dependent).\n`;
    let group;
    for (const doc of docs) {
      if (group !== doc.group) {
        group = doc.group;
        index += `\n## ${label(group)}\n\n`;
      }
      index += `- [${label(doc.title)}](${new URL(doc.md, origin).href}): ${label(doc.summary)}\n`;
      report.push({
        version: name,
        source: path.relative(siteDir, doc.source),
        title: doc.title,
        extracted: doc.extracted,
        summary: doc.summary,
      });
    }
    put(`${prefix}/llms.txt`, index);
    if (!version.isLast)
      headers += `\n${prefix}/llms.txt\n  X-Robots-Tag: noindex\n`;
  }
  const latestName =
    latest.versionName === "current" ? "next" : latest.versionName;
  put(
    `${base}llms.txt`,
    `# Obot documentation\n\n> ${PRODUCT}\n\nUse ${latestName} unless the user specifies an installed version. Keep examples and configuration within the selected version. Use /next/ only for unreleased development. If the requested version is not listed, explain that its documentation is unavailable here; do not silently substitute another release.\n\n## Default documentation\n\n- [Obot ${latestName}](${new URL(`${base}${latestName}/llms.txt`, origin).href}): ${latestName === "next" ? "Unreleased; no release snapshots are available." : "Latest released documentation."}\n\n## Other versions\n\n${versions
      .filter((v) => !v.isLast)
      .map((v) => {
        const n = v.versionName === "current" ? "next" : v.versionName;
        return `- [Obot ${n}](${new URL(`${base}${n}/llms.txt`, origin).href}): ${n === "next" ? "Unreleased development." : "Retained release."}`;
      })
      .join("\n")}\n`,
  );
  assertCrawlable(
    await fs.readFile(path.join(outDir, "robots.txt"), "utf8"),
    [...outputs.keys()],
    origin,
  );
  for (const asset of assetChecks) {
    const relative = decodeURIComponent(asset.slice(base.length));
    if (relative.split("/").includes(".."))
      throw new Error(`Unsafe asset ${asset}`);
    const file = path.join(outDir, relative);
    try {
      await fs.access(file);
    } catch {
      throw new Error(`Missing HTML/asset target: ${asset}`);
    }
  }
  // Check all collisions before writing any exports. Docusaurus owns HTML and assets.
  for (const url of outputs.keys()) {
    const file = path.join(outDir, decodeURIComponent(url.slice(base.length)));
    try {
      await fs.access(file);
    } catch {
      continue;
    }
    throw new Error(`Export would overwrite existing file: ${url}`);
  }
  for (const [url, content] of outputs) {
    const file = path.join(outDir, decodeURIComponent(url.slice(base.length)));
    await fs.mkdir(path.dirname(file), { recursive: true });
    await fs.writeFile(file, content);
  }
  await fs.writeFile(path.join(outDir, "_headers"), headers);
  await fs.writeFile(
    path.join(generatedFilesDir, "agent-docs-summaries.json"),
    JSON.stringify(report, null, 2) + "\n",
  );
  console.log(
    `[agent-docs] Exported ${pages.length} pages; ${report.filter((r) => r.extracted).length} extracted summaries in .docusaurus/agent-docs-summaries.json`,
  );
}
export default function agentDocsPlugin(context) {
  let versions;
  return {
    name: "agent-docs",
    allContentLoaded({ allContent }) {
      versions =
        allContent["docusaurus-plugin-content-docs"]?.default?.loadedVersions;
      if (!versions)
        throw new Error(
          "agent-docs requires the default Docusaurus docs plugin",
        );
    },
    async postBuild({ outDir }) {
      await generate({ ...context, outDir, versions });
    },
  };
}
