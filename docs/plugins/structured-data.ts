import type { LoadContext, Plugin } from "@docusaurus/types";
import * as fs from "fs/promises";
import * as path from "path";
import versions from "../versions.json";
import { escapeRegExp } from "./utils";

const VERBOSE = process.env.STRUCTURED_DATA_VERBOSE === "true";
const ORG_NAME = "Obot AI, Inc";
const ORG_DESCRIPTION =
  "Obot is an open-source platform for hosting, managing, and securing MCP servers in enterprise environments.";
const SITE_DESCRIPTION =
  "Official documentation for Obot — the open-source MCP platform for hosting, registry, gateway, and chat.";
const ORG_LOGO_URL = "https://docs.obot.ai/img/obot-logo-blue-black-text.svg";
const ORG_SAME_AS = [
  "https://github.com/obot-platform/obot",
  "https://discord.gg/9sSf4UyAMC",
];
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

/**
 * Map URL path prefixes to proficiency levels for TechArticle.
 * Values follow schema.org/proficiencyLevel expectations.
 */
const PROFICIENCY_MAP: Record<string, string> = {
  concepts: "Beginner",
  functionality: "Beginner",
  installation: "Intermediate",
  configuration: "Advanced",
  enterprise: "Advanced",
  faq: "Beginner",
};

/**
 * Preferred landing pages for each section, used in breadcrumb links.
 * These are validated against the build output at post-build time; if a
 * preferred path doesn't exist, the plugin falls back to the first page
 * discovered under that section's directory.
 */
const PREFERRED_SECTION_LANDING: Record<string, string> = {
  concepts: "concepts/mcp-hosting",
  functionality: "functionality/overview",
  installation: "installation/overview",
  configuration: "configuration/auth-providers",
  enterprise: "enterprise/overview",
  faq: "faq",
};

/** Values derived from siteConfig that are threaded through helpers. */
interface SiteInfo {
  siteUrl: string;
  siteName: string;
  baseUrl: string;
  /** Validated section → landing page path map, built from the actual build output. */
  sectionLandingPaths: Record<string, string>;
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
 * Older versioned pages and /next/ (unreleased) pages are skipped —
 * they are already noindex and should not carry structured data.
 */
export default function structuredDataPlugin(_context: LoadContext): Plugin {
  return {
    name: "structured-data-plugin",

    async postBuild({ outDir, siteConfig }) {
      const sectionLandingPaths = await resolveSectionLandingPaths(outDir);
      const site: SiteInfo = {
        siteUrl: siteConfig.url.replace(/\/+$/, ""),
        siteName: siteConfig.title,
        baseUrl: siteConfig.baseUrl,
        sectionLandingPaths,
      };
      const limit = createLimit(64);
      await processDirectory(outDir, outDir, site, limit);
      console.log(
        "[structured-data] Finished injecting JSON-LD structured data",
      );
    },
  };
}

// ---------------------------------------------------------------------------
// Section landing page resolution
// ---------------------------------------------------------------------------

/**
 * Validate PREFERRED_SECTION_LANDING against the build output and resolve
 * each section to an actual page path.
 *
 * For each section in SECTION_MAP:
 * 1. If the preferred path exists as a built page → use it.
 * 2. Otherwise, scan the section directory for the first page (sorted
 *    alphabetically) and use that as a fallback.
 * 3. If no pages exist under the section at all → warn and fall back to the section slug itself.
 */
async function resolveSectionLandingPaths(
  outDir: string,
): Promise<Record<string, string>> {
  const resolved: Record<string, string> = {};

  for (const section of Object.keys(SECTION_MAP)) {
    const preferred = PREFERRED_SECTION_LANDING[section];

    // Check if the preferred path exists as a built page
    if (preferred) {
      const preferredHtml = path.join(outDir, preferred, "index.html");
      try {
        await fs.access(preferredHtml);
        resolved[section] = preferred;
        continue;
      } catch {
        console.warn(
          `[structured-data] Preferred landing page for "${section}" not found: /${preferred}/. ` +
            "Attempting to discover a fallback from the build output.",
        );
      }
    }

    // Fallback: find the first page under the section directory
    const sectionDir = path.join(outDir, section);
    try {
      await fs.access(sectionDir);
    } catch {
      console.warn(
        `[structured-data] Section directory "${section}" not found in build output — ` +
          "breadcrumb links for this section will point to the section slug directly.",
      );
      resolved[section] = section;
      continue;
    }

    const fallback = await findFirstPage(sectionDir, section);
    if (fallback) {
      console.warn(
        `[structured-data] Using fallback landing page for "${section}": /${fallback}/`,
      );
      resolved[section] = fallback;
    } else {
      console.warn(
        `[structured-data] No pages found under "${section}" — ` +
          "breadcrumb links will point to the section slug directly.",
      );
      resolved[section] = section;
    }
  }

  return resolved;
}

/**
 * Find the first index.html page under a directory (breadth-first, sorted).
 * Returns a root-relative path like "concepts/mcp-hosting" or null.
 */
async function findFirstPage(
  dir: string,
  relPrefix: string,
): Promise<string | null> {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  entries.sort((a, b) => a.name.localeCompare(b.name));

  // Check for index.html directly in this directory first
  if (entries.some((e) => e.isFile() && e.name === "index.html")) {
    return relPrefix;
  }

  // Recurse into subdirectories (sorted) to find the first page
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const result = await findFirstPage(
      path.join(dir, entry.name),
      `${relPrefix}/${entry.name}`,
    );
    if (result) return result;
  }

  return null;
}

// ---------------------------------------------------------------------------
// Directory walker
// ---------------------------------------------------------------------------

async function processDirectory(
  dir: string,
  outDir: string,
  site: SiteInfo,
  limit: LimitFn,
): Promise<void> {
  const entries = await fs.readdir(dir, { withFileTypes: true });

  await Promise.all(
    entries.map(async (entry) => {
      const fullPath = path.join(dir, entry.name);

      if (entry.isDirectory()) {
        // Skip versioned directories at the top level of outDir —
        // older versions are already noindex and /next/ is unreleased.
        // Only latest-version (root-level) pages should get structured data.
        if (
          dir === outDir &&
          (OLDER_VERSIONS.includes(entry.name) || entry.name === "next")
        ) {
          return;
        }
        await processDirectory(fullPath, outDir, site, limit);
      } else if (entry.isFile() && entry.name.endsWith(".html")) {
        await limit(() => processHtmlFile(fullPath, site));
      }
    }),
  );
}

// ---------------------------------------------------------------------------
// HTML processing
// ---------------------------------------------------------------------------

async function processHtmlFile(
  filePath: string,
  site: SiteInfo,
): Promise<void> {
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
  const ogImage = extractOgImage(html);

  // Skip pages that have no usable metadata (redirect stubs, etc.) or are non-content pages
  if (!title || !canonical) {
    if (VERBOSE) {
      const missing = [!title && "title", !canonical && "canonical URL"]
        .filter(Boolean)
        .join(" and ");
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

  const { html: htmlAfterRemoval } = extractAndRemoveBreadcrumbList(html);
  const section = deriveSection(canonical, site);

  const graph = buildGraph({
    title,
    description,
    url: canonical,
    section,
    ogImage,
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
      `[structured-data] Skipping structured data injection for ${filePath}: no </head> tag found`,
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
    /<meta[^>]*?(?=\bname="description")(?=[^>]*\bcontent="([^"]*)")[^>]*>/,
  );
  return match?.[1] || null;
}

function extractCanonical(html: string): string | null {
  // Handle attributes in any order (e.g. data-rh="true" before rel=)
  const match = html.match(
    /<link[^>]*?(?=\brel="canonical")(?=[^>]*\bhref="([^"]*)")[^>]*>/,
  );
  return match?.[1] || null;
}

function extractOgImage(html: string): string | null {
  // Handle attributes in any order (e.g. data-rh="true" before property=)
  const match = html.match(
    /<meta[^>]*?(?=\bproperty="og:image")(?=[^>]*\bcontent="([^"]*)")[^>]*>/,
  );
  return match?.[1] || null;
}

/** Regex that matches any <script type="application/ld+json">…</script> tag. */
const LD_JSON_SCRIPT_RE =
  /<script[^>]*type="application\/ld\+json"[^>]*>([\s\S]*?)<\/script>/g;

/**
 * Find and remove the standalone BreadcrumbList JSON-LD that Docusaurus
 * auto-generates, returning the HTML with that script tag stripped.
 *
 * Our @graph block contains its own hierarchical BreadcrumbList built from
 * the URL path, so the single-item Docusaurus version is redundant.
 *
 * Identification is done by *parsing* each ld+json block and checking that
 * the top-level `@type` is exactly `"BreadcrumbList"` (with an optional
 * schema.org `@context`), so @graph blocks or other structured data that
 * merely reference BreadcrumbList in a nested position are left untouched.
 */
function extractAndRemoveBreadcrumbList(html: string): { html: string } {
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

    return { html: html.replace(match[0], "") };
  }

  return { html };
}

/** Extract the relative pathname from a full URL, stripped of siteUrl and baseUrl. */
function toPathname(url: string, site: SiteInfo): string {
  return url
    .replace(site.siteUrl, "")
    .replace(new RegExp(`^${escapeRegExp(site.baseUrl)}`), "/");
}

function deriveSection(url: string, site: SiteInfo): string {
  // url looks like "https://docs.obot.ai/concepts/mcp-hosting/"
  const pathname = toPathname(url, site);
  const firstSegment = pathname.split("/").filter(Boolean)[0];
  if (firstSegment && firstSegment in SECTION_MAP) {
    return SECTION_MAP[firstSegment];
  }
  return "Overview";
}

function deriveProficiency(url: string, site: SiteInfo): string {
  const pathname = toPathname(url, site);
  const firstSegment = pathname.split("/").filter(Boolean)[0];
  if (firstSegment && firstSegment in PROFICIENCY_MAP) {
    return PROFICIENCY_MAP[firstSegment];
  }
  return "Beginner";
}

/**
 * Two-letter technical acronyms that are meaningful as keywords.
 * URL path segments are always lowercase so we cannot rely on case detection
 * there; this allowlist ensures they are kept when found in URLs or titles.
 */
const SHORT_TERM_ALLOWLIST = new Set([
  "ai",
  "ci",
  "cd",
  "db",
  "dl",
  "ha",
  "id",
  "io",
  "ip",
  "ml",
  "os",
  "qa",
  "ui",
  "ux",
  "vm",
]);

/**
 * Derive keywords for a page from its title, section, and URL path segments.
 * Returns a deduplicated array of lowercase keywords.
 */
function deriveKeywords(
  title: string,
  section: string,
  url: string,
  site: SiteInfo,
): string[] {
  const pathname = toPathname(url, site);
  const segments = pathname.split("/").filter(Boolean);

  // Start with base keywords
  const keywords = new Set<string>(["obot", "mcp", section.toLowerCase()]);

  // Add meaningful words from URL path segments (skip generic ones)
  const skipWords = new Set(["overview", "index"]);
  for (const segment of segments) {
    if (skipWords.has(segment)) continue;
    // Convert kebab-case to individual words
    for (const word of segment.split("-")) {
      if (word.length > 2 || SHORT_TERM_ALLOWLIST.has(word)) {
        keywords.add(word.toLowerCase());
      }
    }
  }

  // Add meaningful words from the title (skip short/common words)
  const stopWords = new Set([
    "a",
    "an",
    "the",
    "and",
    "or",
    "of",
    "in",
    "on",
    "to",
    "for",
    "is",
    "it",
    "with",
    "by",
    "at",
    "from",
    "as",
    "how",
    "what",
    "why",
    "your",
    "you",
  ]);
  for (const word of title.split(/\s+/)) {
    const clean = word.toLowerCase().replace(/[^a-z0-9]/g, "");
    if (clean.length <= 1 || stopWords.has(clean)) continue;
    // Keep 2-letter words if they are all-uppercase in the original title
    // (e.g. "AI", "OS") or appear in the technical term allowlist.
    if (clean.length === 2) {
      const raw = word.replace(/[^a-zA-Z0-9]/g, "");
      if (raw === raw.toUpperCase() || SHORT_TERM_ALLOWLIST.has(clean)) {
        keywords.add(clean);
      }
      continue;
    }
    keywords.add(clean);
  }

  const MAX_KEYWORDS = 30;
  return Array.from(keywords).slice(0, MAX_KEYWORDS);
}

// ---------------------------------------------------------------------------
// JSON-LD graph builder
// ---------------------------------------------------------------------------

interface PageMeta {
  title: string;
  description: string | null;
  url: string;
  section: string;
  ogImage: string | null;
  site: SiteInfo;
}

function buildGraph(meta: PageMeta): Record<string, unknown> {
  const { siteUrl, siteName } = meta.site;
  const orgId = `${siteUrl}/#organization`;
  const siteId = `${siteUrl}/#website`;
  const pageId = `${meta.url}#webpage`;
  const articleId = `${meta.url}#article`;
  const breadcrumbId = `${meta.url}#breadcrumb`;

  // ── Organization ──────────────────────────────────────────────────────
  const organization: Record<string, unknown> = {
    "@type": "Organization",
    "@id": orgId,
    name: ORG_NAME,
    url: siteUrl,
    description: ORG_DESCRIPTION,
    logo: {
      "@type": "ImageObject",
      url: ORG_LOGO_URL,
      caption: ORG_NAME,
    },
    sameAs: ORG_SAME_AS,
  };

  // ── WebSite ───────────────────────────────────────────────────────────
  const website: Record<string, unknown> = {
    "@type": "WebSite",
    "@id": siteId,
    name: siteName,
    description: SITE_DESCRIPTION,
    url: siteUrl,
    publisher: { "@id": orgId },
    inLanguage: "en",
  };

  // ── BreadcrumbList ────────────────────────────────────────────────────
  // Build a full hierarchical breadcrumb from the URL path instead of using
  // the single-item list that Docusaurus generates by default.
  const breadcrumbList = buildBreadcrumbList(meta, breadcrumbId);

  // ── WebPage ───────────────────────────────────────────────────────────
  const webPage: Record<string, unknown> = {
    "@type": "WebPage",
    "@id": pageId,
    url: meta.url,
    name: meta.title,
    isPartOf: { "@id": siteId },
    about: { "@id": articleId },
    inLanguage: "en",
    breadcrumb: { "@id": breadcrumbId },
  };
  if (meta.description) {
    webPage.description = meta.description;
  }
  if (meta.ogImage) {
    webPage.primaryImageOfPage = {
      "@type": "ImageObject",
      url: meta.ogImage,
    };
  }

  // ── TechArticle ───────────────────────────────────────────────────────
  const keywords = deriveKeywords(
    meta.title,
    meta.section,
    meta.url,
    meta.site,
  );
  const proficiency = deriveProficiency(meta.url, meta.site);

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
    proficiencyLevel: proficiency,
    keywords: keywords,
  };
  if (meta.description) {
    techArticle.description = meta.description;
  }
  if (meta.ogImage) {
    techArticle.image = {
      "@type": "ImageObject",
      url: meta.ogImage,
    };
  }

  return {
    "@context": "https://schema.org",
    "@graph": [organization, website, webPage, breadcrumbList, techArticle],
  };
}

/**
 * Build a full hierarchical BreadcrumbList from the page URL.
 *
 * For a URL like `https://docs.obot.ai/installation/reference-architectures/gcp-gke/`
 * this produces:
 *   1. Home → https://docs.obot.ai/
 *   2. Installation → https://docs.obot.ai/installation/overview/
 *   3. Reference Architectures
 *   4. Google Cloud GKE (current page, no item URL)
 *
 * For the root page (no URL segments beyond the base), the list contains
 * only the "Home" item plus the page title.
 */
function buildBreadcrumbList(
  meta: PageMeta,
  breadcrumbId: string,
): Record<string, unknown> {
  const { siteUrl } = meta.site;
  const pathname = toPathname(meta.url, meta.site);
  const segments = pathname.split("/").filter(Boolean);

  const items: Record<string, unknown>[] = [];
  let position = 1;

  // 1. Home is always the first breadcrumb
  items.push({
    "@type": "ListItem",
    position: position++,
    name: "Home",
    item: `${siteUrl}/`,
  });

  if (segments.length > 0) {
    const firstSegment = segments[0];

    // 2. Section category (if the first segment maps to a known section)
    if (firstSegment in SECTION_MAP) {
      const sectionName = SECTION_MAP[firstSegment];
      // Link to the section's landing page if this isn't the only segment
      // (i.e., we're deeper than the section root)
      if (segments.length > 1) {
        const landingPath =
          meta.site.sectionLandingPaths[firstSegment] ?? firstSegment;
        items.push({
          "@type": "ListItem",
          position: position++,
          name: sectionName,
          item: `${siteUrl}/${landingPath}/`,
        });
      }

      // 3. Sub-category for deeply nested pages (e.g. reference-architectures, encryption-providers)
      if (segments.length > 2) {
        const subName = segments
          .slice(1, -1)
          .map((s) =>
            s
              .split("-")
              .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
              .join(" "),
          )
          .join(" – ");
        items.push({
          "@type": "ListItem",
          position: position++,
          name: subName,
        });
      }
    }

    // 4. Current page (last breadcrumb — no `item` URL per Google guidelines)
    items.push({
      "@type": "ListItem",
      position: position,
      name: meta.title,
    });
  }

  return {
    "@type": "BreadcrumbList",
    "@id": breadcrumbId,
    itemListElement: items,
  };
}
