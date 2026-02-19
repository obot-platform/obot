import type { LoadContext, Plugin } from "@docusaurus/types";
import * as fs from "fs/promises";
import * as path from "path";
import versions from "../versions.json";
import { escapeRegExp, PATH_REDIRECTS } from "./utils";

const VERBOSE = process.env.CANONICAL_URLS_VERBOSE === "true";

/**
 * Older versions that should have their canonical URLs point to the latest version.
 * Dynamically read from versions.json - first entry is latest, rest are older.
 */
const OLDER_VERSIONS = versions.slice(1);

/**
 * Plugin to handle SEO for versioned docs:
 * 1. Rewrites canonical URLs on older version pages to point to the latest version
 * 2. Adds "noindex,follow" robots meta tag to older versions and /next/ pages
 * 3. Rewrites canonical URLs on /next/ pages to point to the latest version
 * 4. Validates that rewritten canonical URLs actually resolve to existing pages
 * 5. Follows client-side redirects to resolve to the final destination URL
 *
 * Note: Sitemap filtering is handled via the sitemap plugin's ignorePatterns
 * option in docusaurus.config.ts.
 */
export default function canonicalUrlsPlugin(_context: LoadContext): Plugin {
  return {
    name: "canonical-urls-plugin",

    async postBuild({ outDir, siteConfig }) {
      const siteUrl = siteConfig.url.replace(/\/+$/, "");

      // Build a set of all valid root-level paths (latest version pages)
      // by scanning the build output, excluding versioned and /next/ directories.
      const validPaths = await collectValidPaths(outDir);

      // Build a map of paths that are client-side redirects to their targets.
      const redirectMap = await collectRedirects(outDir, validPaths);

      const context: ProcessingContext = { siteUrl, outDir, validPaths, redirectMap };

      // Process older versions
      await Promise.all(
        OLDER_VERSIONS.map(async (version) => {
          const versionDir = path.join(outDir, version);
          try {
            await fs.access(versionDir);
          } catch {
            if (VERBOSE) console.log(`[canonical-urls] Skipping ${version} — directory not found`);
            return;
          }
          await processDirectory(versionDir, version, context);
        })
      );

      // Process /next/ directory (unreleased docs should not be indexed)
      const nextDir = path.join(outDir, "next");
      try {
        await fs.access(nextDir);
        await processDirectory(nextDir, "next", context);
      } catch {
        if (VERBOSE) console.log("[canonical-urls] Skipping next — directory not found");
      }

      console.log("[canonical-urls] Finished updating canonical URLs and robots meta for versioned docs");
    },
  };
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ProcessingContext {
  siteUrl: string;
  outDir: string;
  /** Set of root-relative paths (without leading/trailing slashes) that exist as real pages. */
  validPaths: Set<string>;
  /** Map of root-relative paths that are redirect stubs → their resolved target paths. */
  redirectMap: Map<string, string>;
}

// ---------------------------------------------------------------------------
// Build-output scanning
// ---------------------------------------------------------------------------

/**
 * Collect all valid root-level page paths from the build output.
 * Excludes versioned directories and /next/.
 * Returns paths like "concepts/architecture", "installation/overview", etc.
 */
async function collectValidPaths(outDir: string): Promise<Set<string>> {
  const skipDirs = new Set([...OLDER_VERSIONS, "next"]);
  const paths = new Set<string>();

  async function walk(dir: string, rel: string): Promise<void> {
    const entries = await fs.readdir(dir, { withFileTypes: true });
    await Promise.all(
      entries.map(async (entry) => {
        const fullPath = path.join(dir, entry.name);
        const relPath = rel ? `${rel}/${entry.name}` : entry.name;

        if (entry.isDirectory()) {
          if (rel === "" && skipDirs.has(entry.name)) return;
          await walk(fullPath, relPath);
        } else if (entry.name === "index.html") {
          // The page path is the directory containing index.html
          paths.add(rel);
        }
      })
    );
  }

  await walk(outDir, "");
  return paths;
}

/**
 * Identify pages that are client-side redirect stubs (created by
 * @docusaurus/plugin-client-redirects) and map them to their target paths.
 * Uses a two-pass approach: first collects all raw redirects, then resolves
 * chains so that processing order doesn't matter.
 */
async function collectRedirects(
  outDir: string,
  validPaths: Set<string>
): Promise<Map<string, string>> {
  const rawRedirects = new Map<string, string>();
  const refreshRegex = /http-equiv="refresh"[^>]*content="0;\s*url=([^"]+)"/i;

  // Pass 1: Collect all raw redirect mappings without following chains.
  for (const pagePath of validPaths) {
    const htmlFile = pagePath
      ? path.join(outDir, pagePath, "index.html")
      : path.join(outDir, "index.html");

    let content: string;
    try {
      content = await fs.readFile(htmlFile, "utf-8");
    } catch {
      continue;
    }

    const match = content.match(refreshRegex);
    if (!match) continue;

    const target = match[1].replace(/^\//, "").replace(/\/$/, "");
    rawRedirects.set(pagePath, target);
  }

  // Pass 2: Resolve chains now that all redirect relationships are known.
  const redirectMap = new Map<string, string>();
  for (const [source, rawTarget] of rawRedirects) {
    let target = rawTarget;
    const visited = new Set<string>([source]);
    // Follow redirect chains (max 10 hops to prevent infinite loops)
    for (let i = 0; i < 10 && rawRedirects.has(target); i++) {
      if (visited.has(target)) break; // cycle detected
      visited.add(target);
      target = rawRedirects.get(target)!;
    }
    redirectMap.set(source, target);
  }

  return redirectMap;
}

// ---------------------------------------------------------------------------
// Directory walker
// ---------------------------------------------------------------------------

async function processDirectory(
  dir: string,
  version: string,
  context: ProcessingContext
): Promise<void> {
  const entries = await fs.readdir(dir, { withFileTypes: true });

  await Promise.all(
    entries.map(async (entry) => {
      const fullPath = path.join(dir, entry.name);

      if (entry.isDirectory()) {
        await processDirectory(fullPath, version, context);
      } else if (entry.isFile() && entry.name.endsWith(".html")) {
        await processHtmlFile(fullPath, version, context);
      }
    })
  );
}

// ---------------------------------------------------------------------------
// HTML processing
// ---------------------------------------------------------------------------

async function processHtmlFile(
  filePath: string,
  version: string,
  context: ProcessingContext
): Promise<void> {
  const { siteUrl, validPaths, redirectMap } = context;

  let content: string;
  try {
    content = await fs.readFile(filePath, "utf-8");
  } catch (error) {
    console.error(`[canonical-urls] Failed to read ${filePath}: ${error}`);
    throw error;
  }

  // 1. Rewrite canonical URL: strip the version prefix, then validate.
  //    The regex uses lookaheads to match <link> tags with rel="canonical"
  //    regardless of attribute order (rel before href, or href before rel).
  const canonicalTagRegex = /<link[^>]*?(?=\brel="canonical")(?=[^>]*\bhref=")[^>]*>/gi;
  const versionedHrefRegex = new RegExp(
    `(href=")${escapeRegExp(siteUrl)}/${escapeRegExp(version)}/([^"]*)(")`,
  );

  let updatedContent = content.replace(canonicalTagRegex, (tag) => {
    const hrefMatch = tag.match(versionedHrefRegex);
    if (!hrefMatch) return tag; // href doesn't contain the expected versioned URL

    const [, hrefPrefix, pagePath, hrefSuffix] = hrefMatch;

    // pagePath is everything after the version prefix, e.g. "concepts/architecture/"
    const normalizedPath = pagePath.replace(/\/$/, "");
    const resolvedPath = resolveCanonicalPath(normalizedPath, validPaths, redirectMap);

    // Build the final canonical URL with trailing slash
    const canonicalUrl = resolvedPath
      ? `${siteUrl}/${resolvedPath}/`
      : `${siteUrl}/`;

    if (VERBOSE && normalizedPath !== resolvedPath) {
      console.log(
        `[canonical-urls] Remapped: /${version}/${normalizedPath}/ → ${canonicalUrl}`
      );
    }

    return tag.replace(versionedHrefRegex, `${hrefPrefix}${canonicalUrl}${hrefSuffix}`);
  });

  // 2. Add robots meta tag with "noindex,follow" to prevent indexing
  const robotsMetaRegex = /<meta\s[^>]*name\s*=\s*["']robots["'][^>]*>/i;
  if (!robotsMetaRegex.test(updatedContent)) {
    const robotsMeta = '<meta name="robots" content="noindex,follow">';
    const headTagRegex = /<head[^>]*>/i;
    if (headTagRegex.test(updatedContent)) {
      updatedContent = updatedContent.replace(
        headTagRegex,
        (match) => `${match}\n${robotsMeta}`
      );
    } else {
      console.warn(
        `[canonical-urls] Could not insert robots meta tag into ${filePath} — <head> tag not found`
      );
    }
  }

  if (content !== updatedContent) {
    try {
      await fs.writeFile(filePath, updatedContent, "utf-8");
    } catch (error) {
      console.error(`[canonical-urls] Failed to write ${filePath}: ${error}`);
      throw error;
    }
  }
}

// ---------------------------------------------------------------------------
// Path resolution
// ---------------------------------------------------------------------------

/**
 * Resolve a root-relative page path to a valid canonical target.
 *
 * Resolution order:
 * 1. If the path exists directly in the build output and is NOT a redirect stub → use it.
 * 2. If the path exists but IS a redirect stub → follow the redirect to its target.
 * 3. If the path is in the explicit PATH_REDIRECTS map → use the mapped path (then validate).
 * 4. Otherwise → fall back to the site root ("").
 */
function resolveCanonicalPath(
  pagePath: string,
  validPaths: Set<string>,
  redirectMap: Map<string, string>
): string {
  // Case 1: Path exists and is a real (non-redirect) page
  if (validPaths.has(pagePath) && !redirectMap.has(pagePath)) {
    return pagePath;
  }

  // Case 2: Path exists but is a redirect stub — follow it
  if (redirectMap.has(pagePath)) {
    const target = redirectMap.get(pagePath)!;
    if (validPaths.has(target) && !redirectMap.has(target)) {
      return target;
    }
  }

  // Case 3: Explicit mapping for removed/renamed pages
  if (pagePath in PATH_REDIRECTS) {
    const mapped = PATH_REDIRECTS[pagePath];
    if (mapped === "") return "";  // explicit fallback to site root
    if (validPaths.has(mapped) && !redirectMap.has(mapped)) {
      return mapped;
    }
    // The mapped target might itself be a redirect
    if (redirectMap.has(mapped)) {
      const target = redirectMap.get(mapped)!;
      if (validPaths.has(target) && !redirectMap.has(target)) {
        return target;
      }
    }
  }

  // Case 4: Fallback to site root
  console.warn(
    `[canonical-urls] No valid canonical target for "/${pagePath}" — falling back to site root`
  );
  return "";
}
