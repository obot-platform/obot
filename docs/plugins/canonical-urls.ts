import type { LoadContext, Plugin } from "@docusaurus/types";
import * as fs from "fs/promises";
import * as path from "path";
import versions from "../versions.json";

/**
 * Older versions that should have their canonical URLs point to the latest version.
 * Dynamically read from versions.json - first entry is latest, rest are older.
 */
const OLDER_VERSIONS = versions.slice(1);

/**
 * Plugin to handle SEO for versioned docs:
 * 1. Rewrites canonical URLs to point to the latest version
 * 2. Adds "noindex,follow" robots meta tag to older versions
 *
 * Note: Sitemap filtering is handled via the sitemap plugin's ignorePatterns
 * option in docusaurus.config.ts.
 *
 * This helps avoid duplicate content issues in search engines by telling them
 * that the latest version is the authoritative source, while still allowing
 * crawlers to follow links on older version pages.
 *
 * The plugin runs after the build completes and modifies the generated HTML files.
 */
export default function canonicalUrlsPlugin(_context: LoadContext): Plugin {
  return {
    name: "canonical-urls-plugin",

    async postBuild({ outDir, siteConfig }) {
      const siteUrl = siteConfig.url;

      await Promise.all(
        OLDER_VERSIONS.map(async (version) => {
          const versionDir = path.join(outDir, version);

          try {
            await fs.access(versionDir);
          } catch {
            console.log(`[canonical-urls] Skipping ${version} - directory not found`);
            return;
          }

          await processDirectory(versionDir, version, siteUrl);
        })
      );

      console.log("[canonical-urls] Finished updating canonical URLs and robots meta for versioned docs");
    },
  };
}

/**
 * Recursively process all HTML files in a directory
 */
async function processDirectory(
  dir: string,
  version: string,
  siteUrl: string
): Promise<void> {
  const entries = await fs.readdir(dir, { withFileTypes: true });

  await Promise.all(
    entries.map(async (entry) => {
      const fullPath = path.join(dir, entry.name);

      if (entry.isDirectory()) {
        await processDirectory(fullPath, version, siteUrl);
      } else if (entry.isFile() && entry.name.endsWith(".html")) {
        await processHtmlFile(fullPath, version, siteUrl);
      }
    })
  );
}

/**
 * Update the canonical URL and add robots meta tag in an HTML file
 */
async function processHtmlFile(
  filePath: string,
  version: string,
  siteUrl: string
): Promise<void> {
  const content = await fs.readFile(filePath, "utf-8");

  // 1. Update canonical URL to point to the latest version
  // Example: <link rel="canonical" href="https://docs.obot.ai/v0.15.0/some/page/">
  // Should become: <link rel="canonical" href="https://docs.obot.ai/some/page/">
  const versionedUrlPattern = new RegExp(
    `(<link[^>]*rel="canonical"[^>]*href="${escapeRegExp(siteUrl)}/)${escapeRegExp(version)}/`,
    "g"
  );

  let updatedContent = content.replace(versionedUrlPattern, "$1");

  // 2. Add robots meta tag with "noindex,follow" to prevent indexing but allow link following
  // Insert after the opening <head> tag or after existing meta tags
  // Check specifically for a <meta> tag with name="robots" to avoid false positives
  // Handles: spaces around "=", single/double quotes, attributes in any order
  const robotsMetaRegex = /<meta\s[^>]*name\s*=\s*["']robots["'][^>]*>/i;
  if (!robotsMetaRegex.test(updatedContent)) {
    const robotsMeta = '<meta name="robots" content="noindex,follow">';

    // Try to insert after the <head> tag (case-insensitive, supports attributes)
    const headTagRegex = /<head[^>]*>/i;
    if (headTagRegex.test(updatedContent)) {
      updatedContent = updatedContent.replace(
        headTagRegex,
        (match) => `${match}\n${robotsMeta}`
      );
    } else {
      console.warn(
        `[canonical-urls] Could not insert robots meta tag into ${filePath} - <head> tag not found`
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

/**
 * Escape special regex characters in a string
 */
function escapeRegExp(string: string): string {
  return string.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
