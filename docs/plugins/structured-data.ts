import type { LoadContext, Plugin } from "@docusaurus/types";
import * as fs from "fs/promises";
import * as path from "path";
import versions from "../versions.json";
import { escapeRegExp } from "./utils";

const VERBOSE = process.env.STRUCTURED_DATA_VERBOSE === "true";
const ORG_NAME = "Obot AI, Inc";
const LATEST_VERSION = versions[0] ?? "current";
const OLDER_VERSIONS = versions.slice(1);

/** Map URL path prefix to a human-readable article section name. */
const SECTION_MAP: Record<string, string> = {
  concepts: "Concepts",
  functionality: "Features",
  installation: "Installation",
  configuration: "Configuration and Operations",
  enterprise: "Enterprise",
  faq: "FAQ",
};

/** Values derived from siteConfig that are threaded through helpers. */
interface SiteInfo {
  siteUrl: string;
  siteName: string;
  baseUrl: string;
}

type LimitFn = <T>(fn: () => Promise<T>) => Promise<T>;

/** Simple concurrency limiter to avoid EMFILE on large builds. */
function createLimit(concurrency: number): LimitFn {
  let active = 0;
  const queue: (() => void)[] = [];

  return <T>(fn: () => Promise<T>): Promise<T> =>
    new Promise<T>((resolve, reject) => {
      const run = () => {
        active++;
        fn()
          .then(resolve, reject)
          .finally(() => {
            active--;
            if (queue.length > 0) queue.shift()!();
          });
      };
      if (active < concurrency) run();
      else queue.push(run);
    });
}

/**
 * Post-build plugin that injects a JSON-LD @graph block into every
 * latest-version HTML page.  The graph contains Organization, WebSite,
 * WebPage, BreadcrumbList (migrated from the standalone one Docusaurus
 * already emits), and TechArticle entities.
 *
 * Older versioned pages are skipped — they are already noindex.
 */
export default function structuredDataPlugin(_context: LoadContext): Plugin {
  return {
    name: "structured-data-plugin",

    async postBuild({ outDir, siteConfig }) {
      const site: SiteInfo = {
        siteUrl: siteConfig.url.replace(/\/+$/, ""),
        siteName: siteConfig.title,
        baseUrl: siteConfig.baseUrl,
      };
      const limit = createLimit(64);
      await processDirectory(outDir, outDir, site, limit);
      console.log(
        "[structured-data] Finished injecting JSON-LD structured data"
      );
    },
  };
}

// ---------------------------------------------------------------------------
// Directory walker
// ---------------------------------------------------------------------------

async function processDirectory(dir: string, outDir: string, site: SiteInfo, limit: LimitFn): Promise<void> {
  const entries = await fs.readdir(dir, { withFileTypes: true });

  await Promise.all(
    entries.map(async (entry) => {
      const fullPath = path.join(dir, entry.name);

      if (entry.isDirectory()) {
        // Skip older version directories at the top level of outDir
        if (
          dir === outDir &&
          OLDER_VERSIONS.includes(entry.name)
        ) {
          return;
        }
        await processDirectory(fullPath, outDir, site, limit);
      } else if (entry.isFile() && entry.name.endsWith(".html")) {
        await limit(() => processHtmlFile(fullPath, site));
      }
    })
  );
}

// ---------------------------------------------------------------------------
// HTML processing
// ---------------------------------------------------------------------------

async function processHtmlFile(filePath: string, site: SiteInfo): Promise<void> {
  let html: string;
  try {
    html = await fs.readFile(filePath, "utf-8");
  } catch (error) {
    console.error(`[structured-data] Failed to read ${filePath}: ${error}`);
    throw error;
  }

  const title = extractTitle(html, site.siteName);
  const description = extractDescription(html);
  const canonical = extractCanonical(html);

  // Skip pages that have no usable metadata (redirect stubs, etc.) or are non-content pages
  if (!title || !canonical) {
    if (VERBOSE) {
      const missing = [!title && "title", !canonical && "canonical URL"].filter(Boolean).join(" and ");
      console.log(`[structured-data] Skipping ${filePath}: missing ${missing}`);
    }
    return;
  }
  if (canonical.includes("/404.html")) {
    if (VERBOSE) {
      console.log(`[structured-data] Skipping ${filePath}: 404 page`);
    }
    return;
  }

  const { breadcrumbs, html: htmlAfterRemoval } =
    extractAndRemoveBreadcrumbList(html);
  const section = deriveSection(canonical, site);

  const graph = buildGraph({
    title,
    description,
    url: canonical,
    breadcrumbs,
    section,
    site,
  });

  // Escape characters that could break out of a <script> tag or trigger XSS.
  // JSON parsers treat the \uXXXX forms identically to the raw characters.
  const safeJson = JSON.stringify(graph)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026")
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
  const scriptTag = `<script type="application/ld+json">${safeJson}</script>`;

  let updated = htmlAfterRemoval;

  // Inject our @graph script tag right before the closing </head> tag.
  // Use a case-insensitive regex so we handle variants like </HEAD>, and
  // explicitly skip writing if no </head> is present to avoid only removing
  // the BreadcrumbList without adding our replacement.
  const headCloseRegex = /<\/head>/i;
  if (!headCloseRegex.test(updated)) {
    console.warn(
      `[structured-data] Skipping structured data injection for ${filePath}: no </head> tag found`
    );
    return;
  }
  updated = updated.replace(headCloseRegex, `${scriptTag}\n</head>`);

  if (updated !== html) {
    try {
      await fs.writeFile(filePath, updated, "utf-8");
    } catch (error) {
      console.error(`[structured-data] Failed to write ${filePath}: ${error}`);
      throw error;
    }
  }
}

// ---------------------------------------------------------------------------
// Metadata extraction helpers
// ---------------------------------------------------------------------------

function extractTitle(html: string, siteName: string): string | null {
  const match = html.match(/<title[^>]*>([^<]+)<\/title>/);
  if (!match) return null;
  // Strip the common " | <siteName>" suffix using the configured site name
  const escapedSiteName = escapeRegExp(siteName);
  const suffixRegex = new RegExp(`\\s*\\|\\s*${escapedSiteName}$`);
  return match[1].replace(suffixRegex, "").trim() || null;
}

function extractDescription(html: string): string | null {
  // Handle attributes in any order (e.g. data-rh="true" before name=)
  const match = html.match(
    /<meta[^>]*?(?=\bname="description")(?=[^>]*\bcontent="([^"]*)")[^>]*>/
  );
  return match?.[1] || null;
}

function extractCanonical(html: string): string | null {
  // Handle attributes in any order (e.g. data-rh="true" before rel=)
  const match = html.match(
    /<link[^>]*?(?=\brel="canonical")(?=[^>]*\bhref="([^"]*)")[^>]*>/
  );
  return match?.[1] || null;
}

/** Regex that matches any <script type="application/ld+json">…</script> tag. */
const LD_JSON_SCRIPT_RE =
  /<script[^>]*type="application\/ld\+json"[^>]*>([\s\S]*?)<\/script>/g;

/**
 * Find the standalone BreadcrumbList JSON-LD that Docusaurus auto-generates,
 * return its parsed content, and return the HTML with that specific tag removed.
 *
 * Identification is done by *parsing* each ld+json block and checking that the
 * top-level `@type` is exactly `"BreadcrumbList"` (with an optional schema.org
 * `@context`), so @graph blocks or other structured data that merely reference
 * BreadcrumbList in a nested position are left untouched.
 */
function extractAndRemoveBreadcrumbList(html: string): {
  breadcrumbs: Record<string, unknown> | null;
  html: string;
} {
  let breadcrumbs: Record<string, unknown> | null = null;
  let tagToRemove: string | null = null;

  for (const match of html.matchAll(LD_JSON_SCRIPT_RE)) {
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(match[1]) as Record<string, unknown>;
    } catch {
      continue;
    }

    if (parsed["@type"] !== "BreadcrumbList") continue;

    // Optionally verify the @context is schema.org (Docusaurus always sets it)
    const ctx = parsed["@context"];
    if (ctx !== undefined && ctx !== "https://schema.org") continue;

    breadcrumbs = parsed;
    tagToRemove = match[0];
    break;
  }

  return {
    breadcrumbs,
    html: tagToRemove ? html.replace(tagToRemove, "") : html,
  };
}

function deriveSection(url: string, site: SiteInfo): string {
  // url looks like "https://docs.obot.ai/concepts/mcp-hosting/"
  const pathname = url.replace(site.siteUrl, "").replace(new RegExp(`^${escapeRegExp(site.baseUrl)}`), "/");
  const firstSegment = pathname.split("/").filter(Boolean)[0];
  if (firstSegment && firstSegment in SECTION_MAP) {
    return SECTION_MAP[firstSegment];
  }
  return "Overview";
}

// ---------------------------------------------------------------------------
// JSON-LD graph builder
// ---------------------------------------------------------------------------

interface PageMeta {
  title: string;
  description: string | null;
  url: string;
  breadcrumbs: Record<string, unknown> | null;
  section: string;
  site: SiteInfo;
}

function buildGraph(meta: PageMeta): Record<string, unknown> {
  const { siteUrl, siteName } = meta.site;
  const orgId = `${siteUrl}/#organization`;
  const siteId = `${siteUrl}/#website`;
  const pageId = `${meta.url}#webpage`;
  const articleId = `${meta.url}#article`;

  const organization = {
    "@type": "Organization",
    "@id": orgId,
    name: ORG_NAME,
    url: siteUrl,
  };

  const website = {
    "@type": "WebSite",
    "@id": siteId,
    name: siteName,
    url: siteUrl,
    publisher: { "@id": orgId },
  };

  const webPage: Record<string, unknown> = {
    "@type": "WebPage",
    "@id": pageId,
    url: meta.url,
    name: meta.title,
    isPartOf: { "@id": siteId },
  };
  if (meta.description) {
    webPage.description = meta.description;
  }

  const breadcrumbList: Record<string, unknown> | null = meta.breadcrumbs
    ? (() => {
        // Remove the standalone @context — it's inherited from the top-level @graph
        const { "@context": _, ...rest } = meta.breadcrumbs;
        return { ...rest, "@id": `${meta.url}#breadcrumb` };
      })()
    : null;

  const techArticle: Record<string, unknown> = {
    "@type": "TechArticle",
    "@id": articleId,
    headline: meta.title,
    url: meta.url,
    mainEntityOfPage: { "@id": pageId },
    author: { "@id": orgId },
    publisher: { "@id": orgId },
    articleSection: meta.section,
    inLanguage: "en",
    version: LATEST_VERSION,
  };
  if (meta.description) {
    techArticle.description = meta.description;
  }

  const graph: unknown[] = [organization, website, webPage];
  if (breadcrumbList) {
    graph.push(breadcrumbList);
    // Link the webpage to its breadcrumb
    webPage.breadcrumb = { "@id": `${meta.url}#breadcrumb` };
  }
  graph.push(techArticle);

  return {
    "@context": "https://schema.org",
    "@graph": graph,
  };
}
